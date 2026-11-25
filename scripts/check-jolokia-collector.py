#!/usr/bin/env python3
"""Exercise the Agent's authenticated HTTPS probe against a loopback Jolokia JVM."""

import base64
import json
import os
from pathlib import Path
import socket
import ssl
import subprocess
import sys
import tempfile
import time
from urllib.error import HTTPError
from urllib.request import Request, urlopen


USERNAME = "fixture-observer"
PASSWORD = "fixture-jolokia-password"
REFERENCE = "http_probe_profile:51"


def configure_fixture(root, port):
    certificate = root / "server.pem"
    key = root / "server.key"
    subprocess.run([
        "openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1",
        "-subj", "/CN=localhost", "-addext", "subjectAltName=IP:127.0.0.1",
        "-out", certificate, "-keyout", key,
    ], capture_output=True, check=True, timeout=15)
    key.chmod(0o600)
    policy = root / "policy.xml"
    policy.write_text("""<restrict>
  <remote><host>127.0.0.1</host></remote>
  <http><method>post</method></http>
  <commands><command>read</command></commands>
</restrict>
""")
    config = root / "jolokia.properties"
    config.write_text(
        f"host=127.0.0.1\nport={port}\nprotocol=https\nauthMode=basic\n"
        f"user={USERNAME}\npassword={PASSWORD}\nserverCert={certificate}\nserverKey={key}\n"
        f"policyLocation={policy.as_uri()}\ndiscoveryEnabled=false\n"
    )
    config.chmod(0o600)
    return config, certificate


def verify_probe(source, root, port, certificate):
    url = f"https://127.0.0.1:{port}/jolokia"
    verify_read_policy(url, certificate)
    profile = {"reference": REFERENCE, "client_id": 3, "asset_id": 52,
               "check_id": 72, "endpoint_id": 17, "protocol": "jolokia", "url": url,
               "method": "basic", "username": USERNAME, "password": PASSWORD,
               "ca_pem": certificate.read_text()}
    profiles_path = root / "profiles.json"
    profiles_path.write_text(json.dumps({"profiles": [profile]}))
    profiles_path.chmod(0o600)
    env = os.environ | {"AICEBERG_HTTP_PROBE_PROFILES_FILE": str(profiles_path),
                        "AICEBERG_JOLOKIA_TEST_URL": url,
                        "AICEBERG_JOLOKIA_TEST_HTTP_REFERENCE": REFERENCE}
    subprocess.run([
        "go", "test", "-race", "-count=1", "-v", "./internal/platform/agentless",
        "-run", "^TestJolokia",
    ], cwd=source, env=env, check=True, timeout=60)


def verify_read_policy(url, certificate):
    credential = base64.b64encode(f"{USERNAME}:{PASSWORD}".encode()).decode()
    request = Request(url, data=b'{"type":"version"}', headers={
        "Authorization": "Basic " + credential, "Content-Type": "application/json",
    })
    trust = ssl.create_default_context(cafile=str(certificate))
    try:
        with urlopen(request, context=trust, timeout=3) as response:
            denied = json.loads(response.read(4096)).get("status") == 403
    except HTTPError as error:
        denied = error.code == 403
        error.close()
    if not denied:
        raise RuntimeError("Jolokia fixture did not enforce its read-only policy")
    print("PASS: the local Jolokia policy refused a command outside the read allowlist.", flush=True)


def wait_ready(server, log):
    deadline = time.monotonic() + 15
    while server.poll() is None and time.monotonic() < deadline:
        if "fixture-ready" in log.read_text(errors="replace"):
            return
        time.sleep(0.1)
    raise RuntimeError("loopback Jolokia fixture did not start")


def main(jar):
    source = Path(__file__).resolve().parent.parent
    with tempfile.TemporaryDirectory(prefix="aiceberg-jolokia-") as temp:
        root = Path(temp).resolve()
        fixture = root / "JolokiaFixture.java"
        fixture.write_text("""public class JolokiaFixture {
    public static void main(String[] args) throws Exception {
        System.out.println("fixture-ready");
        Thread.sleep(120000);
    }
}
""")
        with socket.socket() as reservation:
            reservation.bind(("127.0.0.1", 0))
            port = reservation.getsockname()[1]
        config, certificate = configure_fixture(root, port)
        log = root / "jvm.log"
        with log.open("w") as stream:
            server = subprocess.Popen(["java", "-Xmx64m", f"-javaagent:{jar}=config={config}", fixture],
                                      stdout=stream, stderr=stream)
            try:
                wait_ready(server, log)
                verify_probe(source, root, port, certificate)
            finally:
                if server.poll() is None:
                    server.terminate()
                try:
                    server.wait(5)
                except subprocess.TimeoutExpired:
                    server.kill()
                    server.wait()
    print("Jolokia fixture stopped and removed.", flush=True)


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit("Usage: check-jolokia-collector.py <official Jolokia JVM javaagent JAR>")
    main(Path(sys.argv[1]).resolve(strict=True))
