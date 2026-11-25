#!/usr/bin/env python3
"""Exercise direct JMX against an isolated JDK management agent with TLS."""

from contextlib import ExitStack, contextmanager
import hashlib
import json
import os
from pathlib import Path
import select
import shutil
import socket
import struct
import subprocess
import tempfile


FIXTURE_SECRET = "fixture-jmx-password"
STORE_SECRET = "fixture-store-password"


def main():
    source = Path(__file__).resolve().parent.parent
    with tempfile.TemporaryDirectory(prefix="aiceberg-jmx-probe-") as directory:
        root = Path(directory).resolve()
        classes = root / "classes"
        classes.mkdir()
        files = sorted((source / "integrations/jmx/src").rglob("*.java"))
        fixture_source = root / "JmxFixture.java"
        fixture_source.write_text("""
public class JmxFixture {
    public static void main(String[] args) throws Exception {
        System.out.println("ready");
        Thread.sleep(120000);
    }
}
""")
        run("javac", "--release", "17", "-d", classes, *files, fixture_source)
        provision_certificates(root)
        (root / "jmxremote.access").write_text("fixture-observer readonly\n")
        password = root / "jmxremote.password"
        password.touch(mode=0o600)
        password.write_text(f"fixture-observer {FIXTURE_SECRET}\n")
        with managed_fixture(root, classes) as (registry_port, server_port):
            verify_fixture(classes, root, registry_port, server_port)
            verify_agent(source, root, classes, registry_port, server_port)
        with managed_fixture(root, classes, "wrong-host.p12") as (registry_port, server_port):
            status, result = request(classes, registry_port, server_port, FIXTURE_SECRET, root / "ca.pem")
            if status == 0 or result != {"status": "fail", "code": "tls_failed"}:
                raise RuntimeError(f"Wrong certificate hostname was accepted: {result}")
        with socket.socket() as silent:
            silent.bind(("127.0.0.1", 0))
            silent.listen(1)
            port = silent.getsockname()[1]
            status, result = request(classes, port, port, FIXTURE_SECRET, root / "ca.pem", budget=1000)
            if status == 0 or result != {"status": "fail", "code": "timeout"}:
                raise RuntimeError(f"Open socket exceeded the JMX probe budget: {result}")
    print("PASS: TLS hostname validation and bounded handshake.")
    print("JDK fixture stopped and removed.", flush=True)


def reserve_ports():
    with ExitStack() as sockets:
        listeners = [sockets.enter_context(socket.socket()) for _ in range(2)]
        for listener in listeners:
            listener.bind(("127.0.0.1", 0))
        return tuple(listener.getsockname()[1] for listener in listeners)


def provision_certificates(root):
    run("openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1",
        "-subj", "/CN=JMX fixture root", "-addext", "basicConstraints=critical,CA:TRUE",
        "-keyout", root / "ca.key", "-out", root / "ca.pem")
    run("openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1",
        "-subj", "/CN=Untrusted fixture root", "-addext", "basicConstraints=critical,CA:TRUE",
        "-keyout", root / "other-ca.key", "-out", root / "other-ca.pem")
    issue_certificate(root, "server", "127.0.0.1")
    issue_certificate(root, "wrong-host", "127.0.0.2")


def issue_certificate(root, name, ip):
    run("openssl", "req", "-new", "-newkey", "rsa:2048", "-nodes",
        "-subj", "/CN=" + ip, "-keyout", root / (name + ".key"), "-out", root / (name + ".csr"))
    extension = root / (name + ".ext")
    extension.write_text(f"basicConstraints=critical,CA:FALSE\nsubjectAltName=IP:{ip}\nextendedKeyUsage=serverAuth\n")
    run("openssl", "x509", "-req", "-days", "1", "-in", root / (name + ".csr"), "-extfile", extension,
        "-CA", root / "ca.pem", "-CAkey", root / "ca.key", "-CAcreateserial", "-out", root / (name + ".pem"))
    run("openssl", "pkcs12", "-export", "-in", root / (name + ".pem"), "-inkey", root / (name + ".key"),
        "-certfile", root / "ca.pem", "-passout", "pass:" + STORE_SECRET, "-out", root / (name + ".p12"))


@contextmanager
def managed_fixture(root, classes, keystore="server.p12"):
    registry, server = reserve_ports()
    with start_fixture(root, classes, registry, server, keystore) as fixture:
        try:
            wait_ready(fixture)
            yield registry, server
        finally:
            fixture.terminate()
            try:
                fixture.wait(5)
            except subprocess.TimeoutExpired:
                fixture.kill()
                fixture.wait()


def start_fixture(root, classes, registry_port, server_port, keystore):
    return subprocess.Popen([
        "java", "-Xmx64m", "-cp", classes,
        f"-Dcom.sun.management.jmxremote.port={registry_port}",
        f"-Dcom.sun.management.jmxremote.rmi.port={server_port}",
        "-Dcom.sun.management.jmxremote.host=127.0.0.1",
        "-Djava.rmi.server.hostname=127.0.0.1",
        "-Dcom.sun.management.jmxremote.ssl=true",
        "-Dcom.sun.management.jmxremote.registry.ssl=true",
        "-Dcom.sun.management.jmxremote.authenticate=true",
        f"-Dcom.sun.management.jmxremote.access.file={root / 'jmxremote.access'}",
        f"-Dcom.sun.management.jmxremote.password.file={root / 'jmxremote.password'}",
        f"-Djavax.net.ssl.keyStore={root / keystore}",
        "-Djavax.net.ssl.keyStoreType=PKCS12", f"-Djavax.net.ssl.keyStorePassword={STORE_SECRET}",
        "JmxFixture",
    ], stdout=subprocess.PIPE, stderr=subprocess.PIPE)


def wait_ready(fixture):
    started, _, _ = select.select([fixture.stdout], [], [], 8)
    if not started or fixture.stdout.readline() != b"ready\n":
        details = fixture.stderr.read(3000).decode(errors="replace") if fixture.poll() is not None else ""
        raise RuntimeError("JDK fixture did not start: " + details)


def request(classes, registry, server, secret, ca, host="127.0.0.1", budget=5000):
    def text(value):
        encoded = value.encode()
        return struct.pack(">H", len(encoded)) + encoded

    data = (struct.pack(">I", 0x41494A31) + text(host) + text("127.0.0.1")
            + struct.pack(">HHI", registry, server, budget)
            + text("fixture-observer") + text(secret) + text(ca.read_text()))
    result = subprocess.run(
        ["java", "-Xmx32m", "-cp", classes, "com.aiceberg.jmxprobe.Main"],
        input=data, capture_output=True, timeout=8,
    )
    if result.stderr or FIXTURE_SECRET.encode() in result.stdout:
        raise RuntimeError("JMX runner leaked diagnostics or credentials")
    return result.returncode, json.loads(result.stdout)


def verify_fixture(classes, root, registry, server):
    ca = root / "ca.pem"
    status, result = request(classes, registry, server, FIXTURE_SECRET, ca)
    if status != 0 or result["status"] != "ok":
        raise RuntimeError(f"Could not read real JMX metrics: {result}")
    metrics = result["metrics"]
    expected = {"heap_used_bytes", "heap_max_bytes", "uptime_ms", "live_threads", "daemon_threads", "loaded_classes"}
    if set(metrics) != expected or not all(isinstance(value, int) and value >= 0 for value in metrics.values()):
        raise RuntimeError(f"Unexpected JMX metric set: {metrics}")
    for name, port, secret, trust, expected in [
        ("stub port", registry, FIXTURE_SECRET, ca, "endpoint_not_allowed"),
        ("untrusted CA", server, FIXTURE_SECRET, root / "other-ca.pem", "tls_failed"),
        ("password", server, "incorrect-fixture-password", ca, "response_rejected"),
    ]:
        status, result = request(classes, registry, port, secret, trust)
        if status == 0 or result != {"status": "fail", "code": expected}:
            raise RuntimeError(f"JMX {name} should be rejected: {result}")
    print("PASS: six JMX metrics; unapproved RMI port, unknown CA and wrong password refused.", flush=True)


def verify_agent(source, root, classes, registry, server):
    worker = root / "jmx-probe.jar"
    run("jar", "--create", "--file", worker, "--main-class", "com.aiceberg.jmxprobe.Main",
        "-C", classes, "com")
    worker.chmod(0o600)
    profiles = root / "profiles.json"
    profiles.touch(mode=0o600)
    profiles.write_text(json.dumps({"profiles": [{
        "reference": "jmx_probe_profile:22", "client_id": 3, "asset_id": 52, "check_id": 72, "endpoint_id": 17,
        "host": "127.0.0.1", "ip": "127.0.0.1", "registry_port": registry, "server_port": server,
        "user": "fixture-observer", "password": FIXTURE_SECRET, "ca_pem": (root / "ca.pem").read_text(),
    }]}))
    env = os.environ | {
        "AICEBERG_JMX_JAVA": shutil.which("java"),
        "AICEBERG_JMX_WORKER_JAR": str(worker),
        "AICEBERG_JMX_WORKER_SHA256": hashlib.sha256(worker.read_bytes()).hexdigest(),
        "AICEBERG_JMX_PROBE_PROFILES_FILE": str(profiles),
        "AICEBERG_JMX_PROBE_FIXTURE_ADDRESS": f"127.0.0.1:{registry}",
    }
    test_binary = env.get("AICEBERG_JMX_GO_TEST_BINARY")
    if test_binary:
        command = [str(Path(test_binary).resolve()), "-test.run=^TestJMXProbe", "-test.count=1", "-test.v"]
    else:
        command = ["go", "test", "-race", "./internal/platform/agentless", "-run", "^TestJMXProbe", "-count=1", "-v"]
    subprocess.run(command, cwd=source, env=env, check=True, timeout=45)


def run(*command):
    process = subprocess.run(command, capture_output=True, timeout=20)
    if process.returncode != 0:
        raise RuntimeError(f"{command[0]} failed: {process.stderr.decode(errors='replace')[-1200:]}")


if __name__ == "__main__":
    main()
