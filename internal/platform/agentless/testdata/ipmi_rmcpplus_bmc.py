"""Loopback IPMI sensor fixture for TestIPMIProbeAgainstLoopbackBMC."""

import socket
import struct

from pyghmi.ipmi.bmc import Bmc


def sensor(i, name, kind, unit):
    body = bytearray(43)
    body[:9] = bytes([0x20, 0, i, 7, 1, 0x67, 0, kind, 1])
    body[16] = unit
    body[19] = 1
    body[29] = 255
    body[42] = 0xc0 | len(name)
    return struct.pack("<HBBB", i, 0x51, 1, len(body) + len(name)) + body + name


class TestBmc(Bmc):
    records = {1: sensor(1, b"CPU Temp", 1, 1), 2: sensor(2, b"Fan 1", 4, 18)}

    def handle_raw_request(self, req, session):
        if (session.auth_algo, session.integrity_algo, session.conf_algo) != (3, 4, 1):
            return session.send_ipmi_response(code=0xc1)
        key = req["netfn"], req["command"]
        data = req["data"]
        if key == (6, 1):
            reply = bytes([0x20, 0x01, 0x01, 0, 0x02, 0x02, 0, 0, 0, 0, 0])
        elif key == (10, 0x20):
            reply = bytes([0x51, 2, 0, 0xff, 0xff] + [0] * 8 + [2])
        elif key == (10, 0x22):
            reply = b"\x01\x00"
        elif key == (10, 0x23):
            index = (data[2] | data[3] << 8) or 1
            offset, length = data[4:6]
            reply = struct.pack("<H", 2 if index == 1 else 0xffff) + self.records[index][offset:offset + length]
        elif key == (4, 0x2d):
            reply = bytes([31 if data[0] == 1 else 0, 0xc0, 0, 0])
        elif key == (4, 0x27):
            reply = bytes(7)
        else:
            return session.send_ipmi_response(code=0xc1)
        session.send_ipmi_response(data=reply)

def main():
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    bmc = TestBmc({"fixture": "fixture-secret"}, address="127.0.0.1", port=port)
    print(port, flush=True)
    bmc.listen()


if __name__ == "__main__":
    main()
