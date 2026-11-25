#!/usr/bin/env python3
"""Exercise the inherited-socket worker against an independent Telnet server."""
import asyncio
from contextlib import asynccontextmanager
from importlib.metadata import version
import os
from pathlib import Path

import telnetlib3


def observe_session(banner, result):
    async def session(reader, writer):
        try:
            if banner:
                writer.write("independent-fixture-banner\r\n")
                await writer.drain()
            data = await asyncio.wait_for(reader.read(1), timeout=3)
            if not result.done():
                result.set_result(data)
        finally:
            writer.close()
    return session


async def main():
    loop = asyncio.get_running_loop()
    banner_data, quiet_data = loop.create_future(), loop.create_future()
    async with open_server(True, banner_data) as banner_port, open_server(False, quiet_data) as quiet_port:
        print(f"telnetlib3 {version('telnetlib3')} listening on loopback", flush=True)
        await run_agent_test(banner_port, quiet_port)
        for data in await asyncio.wait_for(asyncio.gather(banner_data, quiet_data), timeout=5):
            if data != "":
                raise RuntimeError("probe sent application data to the Telnet session")
        print("Independent server confirmed EOF without application input in both sessions.", flush=True)


@asynccontextmanager
async def open_server(banner, result):
    server = await telnetlib3.create_server(
        "127.0.0.1", 0, shell=observe_session(banner, result), connect_maxwait=0.1,
    )
    try:
        yield server.sockets[0].getsockname()[1]
    finally:
        server.close()
        await server.wait_closed()


async def run_agent_test(banner_port, quiet_port):
    root = Path(__file__).resolve().parent.parent
    env = os.environ | {"AICEBERG_TELNET_TEST_BANNER_PORT": str(banner_port),
                        "AICEBERG_TELNET_TEST_QUIET_PORT": str(quiet_port)}
    test = await asyncio.create_subprocess_exec(
        "go", "test", "-race", "-count=1", "-v", "./cmd/agent",
        "-run", "^TestAgentlessTelnetAgainstIndependentServer$", cwd=root, env=env,
    )
    try:
        status = await asyncio.wait_for(test.wait(), timeout=90)
    finally:
        if test.returncode is None:
            test.kill()
            await test.wait()
    if status:
        raise SystemExit(status)


if __name__ == "__main__":
    asyncio.run(main())
