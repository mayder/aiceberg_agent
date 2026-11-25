#!/usr/bin/env python3
"""Qualify the browser service's IP rules against local HTTP fixtures."""

import argparse
import ipaddress
import socket
import subprocess
import sys
import threading
from contextlib import contextmanager
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path


class FixturePage(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"browser-network-fixture")

    def log_message(self, *_args):
        pass


class IPv6Fixture(HTTPServer):
    address_family = socket.AF_INET6


@contextmanager
def fixture(host):
    server_type = IPv6Fixture if host.version == 6 else HTTPServer
    address = f"[{host}]" if host.version == 6 else str(host)
    with server_type((str(host), 0), FixturePage) as server:
        thread = threading.Thread(target=server.serve_forever)
        thread.start()
        try:
            yield f"http://{address}:{server.server_port}/"
        finally:
            server.shutdown()
            thread.join()


def browser_network_policy(unit_path):
    rules = dict(
        line.split("=", 1)
        for line in unit_path.read_text().splitlines()
        if line.startswith(("IPAddressAllow=", "IPAddressDeny="))
    )
    if rules != {"IPAddressDeny": "any", "IPAddressAllow": "localhost"}:
        raise RuntimeError("unexpected default IP policy in the browser unit")
    return rules


def probe(url, policy=None):
    curl = ["curl", "--noproxy", "*", "--max-time", "2", "--fail", "--silent", "--show-error", url]
    command = curl
    if policy is not None:
        command = [
            "systemd-run", "--quiet", "--wait", "--collect", "--pipe",
            "--property=DynamicUser=yes",
            *(f"--property={name}={value}" for name, value in policy.items()),
            *curl,
        ]
    return subprocess.run(command, check=False, capture_output=True, timeout=10)


def verify_policy(host, unit_path):
    policy = browser_network_policy(unit_path)
    local = ipaddress.ip_address("::1" if host.version == 6 else "127.0.0.1")
    with fixture(local) as loopback, fixture(host) as destination:
        if probe(destination).returncode != 0:
            raise RuntimeError("the non-loopback fixture is unreachable before applying the policy")
        if probe(loopback, policy).returncode != 0:
            raise RuntimeError("the default policy blocked the loopback fixture")
        blocked = probe(destination, policy)
        if blocked.returncode not in (1, 7, 28) or b"curl:" not in blocked.stderr:
            raise RuntimeError("the default policy did not block the non-loopback connection")
        # systemd-run parses CIDRs in lists, and handles "localhost" as a
        # shorthand only when it is the entire property value.
        allowed = {**policy, "IPAddressAllow": f"127.0.0.0/8 ::1/128 {host}/{host.max_prefixlen}"}
        for url in (destination, loopback):
            result = probe(url, allowed)
            if result.returncode != 0:
                raise RuntimeError("the amended policy blocked an approved address")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--fixture-ip", type=ipaddress.ip_address, required=True)
    parser.add_argument(
        "--service", type=Path,
        default=Path(__file__).parent / "linux/aiceberg-browser-probe.service",
    )
    args = parser.parse_args()
    host = args.fixture_ip
    if host.is_loopback or not host.is_private or host.is_link_local or host.is_unspecified:
        parser.error("fixture-ip must be assigned to the private Linux lab, outside loopback")
    verify_policy(host, args.service)
    print(f"Linux browser unit IPv{host.version}: loopback allowed, unlisted IP blocked, approved host allowed")


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, OSError, subprocess.SubprocessError) as error:
        print(error, file=sys.stderr)
        sys.exit(1)
