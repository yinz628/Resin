#!/usr/bin/env python3
"""Smoke-check the VPS deployment layout for the generator Compose stack."""

from __future__ import annotations

import argparse
import getpass
import os
import shlex
import socket
import sys
from dataclasses import dataclass

import paramiko


DEFAULT_HOST = "45.192.197.184"
DEFAULT_PORT = 22
DEFAULT_USER = "root"
DEFAULT_REMOTE_PATH = "/opt/"


@dataclass
class CommandResult:
    code: int
    stdout: str
    stderr: str


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Validate the VPS Compose deployment layout and run the generator once.",
    )
    parser.add_argument("--host", default=os.getenv("SMOKE_SSH_HOST", DEFAULT_HOST))
    parser.add_argument(
        "--port",
        type=int,
        default=int(os.getenv("SMOKE_SSH_PORT", str(DEFAULT_PORT))),
    )
    parser.add_argument("--user", default=os.getenv("SMOKE_SSH_USER", DEFAULT_USER))
    parser.add_argument("--password", default=os.getenv("SMOKE_SSH_PASSWORD"))
    parser.add_argument(
        "--remote-path",
        default=os.getenv("SMOKE_REMOTE_PATH", DEFAULT_REMOTE_PATH),
    )
    parser.add_argument("--timeout", type=int, default=10)
    return parser.parse_args()


def tcp_probe(host: str, port: int, timeout: int) -> None:
    with socket.create_connection((host, port), timeout=timeout):
        return


def run_remote(client: paramiko.SSHClient, command: str, timeout: int) -> CommandResult:
    stdin, stdout, stderr = client.exec_command(command, timeout=timeout)
    del stdin
    return CommandResult(
        code=stdout.channel.recv_exit_status(),
        stdout=stdout.read().decode("utf-8", "ignore"),
        stderr=stderr.read().decode("utf-8", "ignore"),
    )


def detect_app_root(client: paramiko.SSHClient, remote_path: str, timeout: int) -> str:
    candidates = [remote_path, f"{remote_path}/app"]
    for candidate in candidates:
        compose_file = shlex.quote(f"{candidate}/docker-compose.yml")
        result = run_remote(client, f"test -f {compose_file}", timeout)
        if result.code == 0:
            return candidate
    raise RuntimeError(
        f"docker-compose.yml not found under {remote_path} or {remote_path}/app",
    )


def build_summary_command(app_root: str) -> str:
    quoted = shlex.quote(app_root)
    return f"""set -e
cd {quoted}
echo "APP_ROOT=$PWD"
echo "HAS_DOCKER_COMPOSE=$(test -f docker-compose.yml && echo 1 || echo 0)"
echo "HAS_MAPPING=$(test -f config/mapping.yaml && echo 1 || echo 0)"
echo "HAS_TEMPLATE=$(test -f config/config.json && echo 1 || echo 0)"
echo "HAS_STATE=$(test -f data/state/port_bindings.json && echo 1 || echo 0)"
echo "HAS_REPORT=$(test -f output/config.generated.report.json && echo 1 || echo 0)"
echo "RUN_GENERATOR_BEGIN"
docker compose run --rm generator 2>&1
echo "RUN_GENERATOR_END"
echo "DIRECTORY_LISTING_BEGIN"
ls -la | sed -n '1,20p'
echo "DIRECTORY_LISTING_END"
echo "REPORT_HEAD_BEGIN"
sed -n '1,80p' output/config.generated.report.json 2>/dev/null || true
echo "REPORT_HEAD_END"
echo "DOCKER_COMPOSE_PS_BEGIN"
docker compose ps 2>&1 || true
echo "DOCKER_COMPOSE_PS_END"
"""


def print_block(title: str, body: str) -> None:
    print(f"\n[{title}]")
    text = body.strip()
    print(text if text else "(empty)")


def main() -> int:
    args = parse_args()
    password = args.password or getpass.getpass(
        f"SSH password for {args.user}@{args.host}: "
    )

    print(f"[1/4] TCP probing {args.host}:{args.port} ...")
    try:
        tcp_probe(args.host, args.port, args.timeout)
    except OSError as exc:
        print(f"TCP probe failed: {exc}", file=sys.stderr)
        return 2
    print("TCP probe ok")

    print(f"[2/4] SSH connecting {args.user}@{args.host} ...")
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    try:
        client.connect(
            hostname=args.host,
            port=args.port,
            username=args.user,
            password=password,
            timeout=args.timeout,
            banner_timeout=args.timeout,
            auth_timeout=args.timeout,
            look_for_keys=False,
            allow_agent=False,
        )
    except Exception as exc:  # noqa: BLE001
        print(f"SSH login failed: {exc}", file=sys.stderr)
        return 3

    try:
        print(f"[3/4] Detecting app root under {args.remote_path} ...")
        app_root = detect_app_root(client, args.remote_path, args.timeout)
        print(f"App root: {app_root}")

        print("[4/4] Collecting remote summary ...")
        result = run_remote(client, build_summary_command(app_root), max(args.timeout, 30))
        if result.code != 0:
            if result.stdout.strip():
                print(result.stdout)
            if result.stderr.strip():
                print(result.stderr, file=sys.stderr)
            print(
                f"Remote smoke command failed with exit code {result.code}",
                file=sys.stderr,
            )
            return 4

        markers = {
            "RUN_GENERATOR": ("RUN_GENERATOR_BEGIN", "RUN_GENERATOR_END"),
            "DIRECTORY_LISTING": ("DIRECTORY_LISTING_BEGIN", "DIRECTORY_LISTING_END"),
            "REPORT_HEAD": ("REPORT_HEAD_BEGIN", "REPORT_HEAD_END"),
            "DOCKER_COMPOSE_PS": ("DOCKER_COMPOSE_PS_BEGIN", "DOCKER_COMPOSE_PS_END"),
        }
        begin_lookup = {begin: key for key, (begin, _) in markers.items()}
        end_lookup = {end: key for key, (_, end) in markers.items()}
        blocks: dict[str, list[str]] = {key: [] for key in markers}
        summary_lines: list[str] = []
        current_block: str | None = None

        for line in result.stdout.splitlines():
            if line in begin_lookup:
                current_block = begin_lookup[line]
                continue
            if line in end_lookup:
                current_block = None
                continue
            if current_block is None:
                summary_lines.append(line)
            else:
                blocks[current_block].append(line)

        print_block("SUMMARY", "\n".join(summary_lines))
        for key in ("RUN_GENERATOR", "DIRECTORY_LISTING", "REPORT_HEAD", "DOCKER_COMPOSE_PS"):
            print_block(key, "\n".join(blocks[key]))
        if result.stderr.strip():
            print_block("STDERR", result.stderr)
    finally:
        client.close()

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
