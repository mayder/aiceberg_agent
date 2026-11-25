#!/usr/bin/env python3
"""Run the Agentless collector against an isolated TLS/SCRAM PostgreSQL."""

import json
import os
import pathlib
import socket
import subprocess
import tempfile
import time


def main():
    source = pathlib.Path(__file__).resolve().parent.parent
    bindir = pathlib.Path(subprocess.check_output(["pg_config", "--bindir"], text=True).strip())
    with tempfile.TemporaryDirectory(prefix="aiceberg-pg-") as temp:
        root = pathlib.Path(temp)
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            port = str(listener.getsockname()[1])
        provision(root, bindir)
        with (root / "postgres.log").open("w") as log:
            server = subprocess.Popen([
                bindir / "postgres", "-D", root / "data", "-c", "listen_addresses=127.0.0.1",
                "-c", f"port={port}", "-c", f"unix_socket_directories={root}", "-c", "ssl=on",
                "-c", f"ssl_cert_file={root / 'server.pem'}", "-c", f"ssl_key_file={root / 'server.key'}",
                "-c", "shared_buffers=16MB", "-c", "max_connections=10",
            ], stdout=log, stderr=log)
            try:
                wait_ready(bindir, root, port, server)
                run_probe(source, bindir, root, port)
            finally:
                if server.poll() is None:
                    try:
                        subprocess.run([bindir / "pg_ctl", "-D", root / "data", "-m", "fast", "-w", "stop"],
                                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=10)
                    except subprocess.TimeoutExpired:
                        server.terminate()
                if server.poll() is None:
                    server.terminate()
                try:
                    server.wait(5)
                except subprocess.TimeoutExpired:
                    server.kill()
                    server.wait()
    print("PostgreSQL fixture stopped and removed.", flush=True)


def provision(root, bindir):
    with (root / "provision.log").open("w") as log:
        subprocess.run([
            "openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1", "-sha256",
            "-subj", "/CN=localhost", "-addext", "subjectAltName=IP:127.0.0.1",
            "-keyout", root / "server.key", "-out", root / "server.pem",
        ], check=True, stdout=log, stderr=log)
        (root / "server.key").chmod(0o600)
        subprocess.run([
            bindir / "initdb", "-D", root / "data", "-U", "fixture_admin",
            "--auth-local=trust", "--auth-host=scram-sha-256", "--no-sync",
        ], check=True, stdout=log, stderr=log)


def wait_ready(bindir, root, port, server):
    for _ in range(75):
        ready = subprocess.run([
            bindir / "pg_isready", "-h", root, "-p", port, "-U", "fixture_admin", "-d", "postgres",
        ], capture_output=True)
        if ready.returncode == 0:
            return
        if server.poll() is not None:
            raise RuntimeError((root / "postgres.log").read_text()[-1500:])
        time.sleep(0.1)
    raise RuntimeError("PostgreSQL fixture unavailable")


def run_probe(source, bindir, root, port):
    psql = [bindir / "psql", "-X", "-h", root, "-p", port, "-U", "fixture_admin", "-d", "postgres", "-v", "ON_ERROR_STOP=1", "-q"]
    subprocess.run(psql + ["-c", "CREATE DATABASE fixture"], check=True)
    subprocess.run(psql, text=True, check=True, input=(
        "CREATE ROLE fixture_monitor LOGIN PASSWORD 'fixture-private';"
        "REVOKE ALL ON DATABASE fixture FROM PUBLIC;"
        "GRANT CONNECT ON DATABASE fixture TO fixture_monitor;"
    ))
    profile = {"reference": "postgres_profile:50", "client_id": 3, "asset_id": 52, "check_id": 72,
               "endpoint_id": 17, "host": "127.0.0.1", "port": int(port), "database": "fixture",
               "user": "fixture_monitor", "password": "fixture-private", "ca_pem": (root / "server.pem").read_text()}
    profile_path = root / "profiles.json"
    with os.fdopen(os.open(profile_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "w") as file:
        json.dump({"profiles": [profile]}, file)
    env = os.environ | {"AICEBERG_POSTGRES_PROFILES_FILE": str(profile_path),
                        "AICEBERG_POSTGRES_TEST_ADDRESS": f"127.0.0.1:{port}", "PGSERVICE": "",
                        "AICEBERG_SQL_ROTATION_FIXTURE": "1"}
    subprocess.run([
        "go", "test", "-race", "./internal/platform/agentless", "-run", "^TestPostgres", "-count=1", "-v",
    ], cwd=source, env=env, check=True, timeout=50)


if __name__ == "__main__":
    main()
