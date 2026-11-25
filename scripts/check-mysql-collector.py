#!/usr/bin/env python3
"""Exercise the read-only collector against a temporary, TLS-only MySQL."""

import json
import os
import pathlib
import shutil
import socket
import subprocess
import tempfile
import time


def main():
    source = pathlib.Path(__file__).resolve().parent.parent
    bindir = os.environ.get("AICEBERG_MYSQL_PROBE_FIXTURE_BIN")
    if not bindir:
        mysqld = shutil.which("mysqld")
        if not mysqld:
            raise SystemExit("Set AICEBERG_MYSQL_PROBE_FIXTURE_BIN to the MySQL bin directory.")
        bindir = pathlib.Path(mysqld).parent
    binpath = pathlib.Path(bindir)
    with tempfile.TemporaryDirectory(prefix="aiceberg-mysql-probe-") as temp:
        root = pathlib.Path(temp).resolve()
        provision(root, binpath)
        with socket.socket() as reserved:
            reserved.bind(("127.0.0.1", 0))
            port = reserved.getsockname()[1]
        server = subprocess.Popen([
            binpath / "mysqld", "--no-defaults", f"--datadir={root / 'data'}",
            f"--socket={root / 'mysql.sock'}", f"--pid-file={root / 'mysql.pid'}",
            "--bind-address=127.0.0.1", f"--port={port}", "--skip-name-resolve",
            "--mysqlx=OFF", "--skip-log-bin", "--innodb-buffer-pool-size=64M",
            "--innodb-redo-log-capacity=16M", "--performance-schema=OFF",
            "--require-secure-transport=ON", f"--ssl-ca={root / 'server.pem'}",
            f"--ssl-cert={root / 'server.pem'}", f"--ssl-key={root / 'server.key'}",
            f"--init-file={root / 'init.sql'}", f"--log-error={root / 'server.log'}",
        ])
        try:
            wait_ready(root, binpath, server)
            probe(source, root, port)
        finally:
            if server.poll() is None:
                server.terminate()
            try:
                server.wait(10)
            except subprocess.TimeoutExpired:
                server.kill()
                server.wait()
    print("MySQL fixture stopped and removed.", flush=True)


def provision(root, binpath):
    subprocess.run([
        "openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1",
        "-subj", "/CN=localhost", "-addext", "subjectAltName=IP:127.0.0.1",
        "-keyout", root / "server.key", "-out", root / "server.pem",
    ], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True)
    (root / "server.key").chmod(0o600)
    (root / "init.sql").write_text(
        "ALTER USER 'root'@'localhost' IDENTIFIED BY 'fixture-root-password';\n"
        "CREATE USER 'fixture_mysql_probe'@'127.0.0.1' IDENTIFIED WITH caching_sha2_password "
        "BY 'fixture-probe-password' REQUIRE SSL;\n"
    )
    (root / "init.sql").chmod(0o600)
    subprocess.run([
        binpath / "mysqld", "--no-defaults", "--initialize-insecure", f"--datadir={root / 'data'}",
        f"--log-error={root / 'server.log'}",
    ], check=True, timeout=20)


def wait_ready(root, binpath, server):
    for _ in range(60):
        ready = subprocess.run([
            binpath / "mysqladmin", "--no-defaults", "--protocol=socket",
            f"--socket={root / 'mysql.sock'}", "-uroot", "ping",
        ], capture_output=True, timeout=2)
        if ready.returncode == 0:
            return
        if server.poll() is not None:
            raise RuntimeError((root / "server.log").read_text()[-1500:])
        time.sleep(0.1)
    raise RuntimeError("MySQL fixture unavailable")


def probe(source, root, port):
    profile_path = root / "profiles.json"
    profile = {
        "reference": "mysql_probe_profile:23", "client_id": 3, "asset_id": 52,
        "check_id": 72, "endpoint_id": 17, "host": "127.0.0.1", "port": port,
        "user": "fixture_mysql_probe", "password": "fixture-probe-password",
        "ca_pem": (root / "server.pem").read_text(),
    }
    with os.fdopen(os.open(profile_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "w") as file:
        json.dump({"profiles": [profile]}, file)
    env = os.environ | {
        "AICEBERG_MYSQL_PROBE_PROFILES_FILE": str(profile_path),
        "AICEBERG_MYSQL_PROBE_FIXTURE_ADDRESS": f"127.0.0.1:{port}",
        "AICEBERG_SQL_ROTATION_FIXTURE": "1",
    }
    subprocess.run([
        "go", "test", "-race", "./internal/platform/agentless", "-run", "^TestMySQLProbe", "-count=1", "-v",
    ], cwd=source, env=env, check=True, timeout=50)


if __name__ == "__main__":
    main()
