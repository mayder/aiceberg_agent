#!/usr/bin/env python3
"""Qualify the Agent against Node Exporter and Prometheus promhttp on loopback."""
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
from urllib.error import URLError
from urllib.request import Request, urlopen

USER = "fixture-observer"
PASSWORD = "fixture-exporter-password"
ACCEPT = "application/openmetrics-text;version=1.0.0,text/plain;version=0.0.4;q=0.9"
GO_SOURCE = """package main
import (
    "crypto/subtle"
    "log"
    "mime"
    "net/http"
    "os"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/collectors"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "github.com/prometheus/common/expfmt"
)
func main() {
    registry := prometheus.NewRegistry()
    registry.MustRegister(collectors.NewGoCollector())
    metrics := promhttp.HandlerFor(registry, promhttp.HandlerOpts{EnableOpenMetrics:true})
    mux := http.NewServeMux()
    mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
        user, password, ok := r.BasicAuth()
        if !ok || user != os.Getenv("EXPORTER_FIXTURE_USER") ||
            subtle.ConstantTimeCompare([]byte(password), []byte(os.Getenv("EXPORTER_FIXTURE_PASSWORD"))) != 1 {
            w.WriteHeader(http.StatusUnauthorized)
            return
        }
        format := expfmt.NegotiateIncludingOpenMetrics(r.Header)
        mediaType, parameters, err := mime.ParseMediaType(string(format))
        if err != nil || mediaType != "application/openmetrics-text" || parameters["version"] != "1.0.0" {
            w.WriteHeader(http.StatusNotAcceptable)
            return
        }
        metrics.ServeHTTP(w, r)
    })
    log.Fatal(http.ListenAndServeTLS(os.Args[1], os.Args[2], os.Args[3], mux))
}
"""


def configure(root):
    certificate, key = root / "server.pem", root / "server.key"
    subprocess.run([
        "openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1",
        "-subj", "/CN=localhost", "-addext", "subjectAltName=IP:127.0.0.1",
        "-out", certificate, "-keyout", key,
    ], capture_output=True, check=True, timeout=15)
    hashed = subprocess.run(["htpasswd", "-niB", USER], input=PASSWORD + "\n",
                            text=True, capture_output=True, check=True, timeout=10)
    config = root / "web.yml"
    config.write_text(
        f"tls_server_config:\n  cert_file: {json.dumps(str(certificate))}\n"
        f"  key_file: {json.dumps(str(key))}\nbasic_auth_users:\n"
        f"  {USER}: {json.dumps(hashed.stdout.strip().split(':', 1)[1])}\n"
    )
    config.chmod(0o600)
    key.chmod(0o600)
    return certificate, key, config


def build_promhttp(root):
    (root / "go.mod").write_text(
        "module openmetrics-fixture\n\ngo 1.25\n\n"
        "require (\n github.com/prometheus/client_golang v1.23.2\n github.com/prometheus/common v0.66.1\n)\n"
    )
    (root / "main.go").write_text(GO_SOURCE)
    binary = root / "promhttp-exporter"
    subprocess.run(["go", "build", "-mod=mod", "-o", binary, "."], cwd=root,
                   env=os.environ | {"GOWORK": "off"}, check=True, timeout=120)
    return binary


def wait_ready(server, url, certificate, expected):
    auth = base64.b64encode(f"{USER}:{PASSWORD}".encode()).decode()
    request = Request(url, headers={"Authorization": f"Basic {auth}", "Accept": ACCEPT})
    trust = ssl.create_default_context(cafile=str(certificate))
    deadline = time.monotonic() + 10
    while server.poll() is None and time.monotonic() < deadline:
        try:
            with urlopen(request, context=trust, timeout=2) as response:
                actual = (response.headers.get_content_type(), response.headers.get_param("version"))
                if actual != expected:
                    raise RuntimeError(f"unexpected exposition format: {actual}")
                print(f"Exporter negotiated {actual[0]} {actual[1]}", flush=True)
                return
        except URLError:
            time.sleep(0.1)
    raise RuntimeError("loopback exporter did not become ready")


def qualify(root, make_command, certificate, expected_format):
    with socket.socket() as reservation:
        reservation.bind(("127.0.0.1", 0))
        address = f"127.0.0.1:{reservation.getsockname()[1]}"
    url = f"https://{address}/metrics"
    profile = {"reference": "http_probe_profile:51", "client_id": 3, "asset_id": 52,
               "check_id": 72, "endpoint_id": 17, "protocol": "openmetrics", "url": url,
               "method": "basic", "username": USER, "password": PASSWORD,
               "ca_pem": certificate.read_text()}
    profiles = root / "profiles.json"
    profiles.write_text(json.dumps({"profiles": [profile]}))
    profiles.chmod(0o600)
    env = os.environ | {"EXPORTER_FIXTURE_USER": USER, "EXPORTER_FIXTURE_PASSWORD": PASSWORD}
    with (root / "exporter.log").open("w") as log:
        server = subprocess.Popen(make_command(address), stdout=log, stderr=log, env=env)
        try:
            wait_ready(server, url, certificate, expected_format)
            subprocess.run([
                "go", "test", "-race", "-count=1", "-v", "./internal/platform/agentless",
                "-run", "^TestOpenMetricsCollectorReadsRealExporter$",
            ], cwd=Path(__file__).resolve().parent.parent,
                env=os.environ | {"AICEBERG_OPENMETRICS_TEST_URL": url,
                                   "AICEBERG_HTTP_PROBE_PROFILES_FILE": str(profiles)},
                check=True, timeout=60)
        finally:
            if server.poll() is None:
                server.terminate()
            try:
                server.wait(timeout=5)
            except subprocess.TimeoutExpired:
                server.kill()
                server.wait(timeout=5)


def main(node_exporter):
    subprocess.run([node_exporter, "--version"], check=True, timeout=5)
    with tempfile.TemporaryDirectory(prefix="aiceberg-openmetrics-") as temp:
        root = Path(temp)
        certificate, key, config = configure(root)
        qualify(root, lambda address: [
            node_exporter, f"--web.listen-address={address}", f"--web.config.file={config}",
            "--collector.disable-defaults",
        ], certificate, ("text/plain", "0.0.4"))
        binary = build_promhttp(root)
        qualify(root, lambda address: [binary, address, certificate, key],
                certificate, ("application/openmetrics-text", "1.0.0"))


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit(f"Usage: {sys.argv[0]} /path/to/node_exporter")
    main(Path(sys.argv[1]).resolve(strict=True))
