#!/usr/bin/env python3
"""Deploy the current backend/web build context to ssh2 and rebuild resin."""

from __future__ import annotations

import argparse
import getpass
import os
import shlex
import shutil
import socket
import sys
import tarfile
import tempfile
import time
from pathlib import Path

import paramiko


DEFAULT_HOST = "107.148.164.86"
DEFAULT_PORT = 22
DEFAULT_USER = "root"
DEFAULT_REMOTE_PATH = "/opt/Resin"
DEFAULT_SERVICE = "resin"
DEFAULT_TIMEOUT = 15
DEFAULT_HEALTH_TIMEOUT = 180
DEFAULT_SYNC_PATHS = (
    ".dockerignore",
    "Dockerfile",
    "cmd",
    "docker",
    "go.mod",
    "go.sum",
    "internal",
    "webui",
)
EXCLUDED_PARTS = {
    ".git",
    ".pytest_cache",
    "__pycache__",
    "dist",
    "node_modules",
}
NORMALIZE_TEXT_SUFFIXES = {
    ".css",
    ".dockerignore",
    ".env",
    ".example",
    ".go",
    ".html",
    ".js",
    ".json",
    ".jsx",
    ".md",
    ".mod",
    ".ps1",
    ".sh",
    ".sum",
    ".ts",
    ".tsx",
    ".txt",
    ".yaml",
    ".yml",
}
NORMALIZE_TEXT_NAMES = {
    ".dockerignore",
    "Dockerfile",
}

if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")
if hasattr(sys.stderr, "reconfigure"):
    sys.stderr.reconfigure(encoding="utf-8", errors="replace")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Upload the current build context to ssh2 and rebuild docker compose service.",
    )
    parser.add_argument("--host", default=os.getenv("DEPLOY_SSH_HOST", DEFAULT_HOST))
    parser.add_argument(
        "--port",
        type=int,
        default=int(os.getenv("DEPLOY_SSH_PORT", str(DEFAULT_PORT))),
    )
    parser.add_argument("--user", default=os.getenv("DEPLOY_SSH_USER", DEFAULT_USER))
    parser.add_argument(
        "--password",
        default=os.getenv("DEPLOY_SSH_PASSWORD"),
    )
    parser.add_argument(
        "--remote-path",
        default=os.getenv("DEPLOY_REMOTE_PATH", DEFAULT_REMOTE_PATH),
    )
    parser.add_argument(
        "--service",
        default=os.getenv("DEPLOY_SERVICE", DEFAULT_SERVICE),
    )
    parser.add_argument("--timeout", type=int, default=DEFAULT_TIMEOUT)
    parser.add_argument(
        "--health-timeout",
        type=int,
        default=DEFAULT_HEALTH_TIMEOUT,
        help="Seconds to wait for the container to become healthy/running.",
    )
    parser.add_argument(
        "--keep-archive",
        action="store_true",
        help="Keep the generated local tar.gz archive for inspection.",
    )
    return parser.parse_args()


def repo_root() -> Path:
    return Path(__file__).resolve().parents[1]


def should_skip(tarinfo: tarfile.TarInfo) -> tarfile.TarInfo | None:
    parts = Path(tarinfo.name).parts
    if any(part in EXCLUDED_PARTS for part in parts):
        return None
    return tarinfo


def should_skip_path(path: Path) -> bool:
    return any(part in EXCLUDED_PARTS for part in path.parts)


def should_normalize_text(path: Path) -> bool:
    if path.name in NORMALIZE_TEXT_NAMES:
        return True
    return path.suffix.lower() in NORMALIZE_TEXT_SUFFIXES


def copy_with_normalized_text(source: Path, destination: Path) -> None:
    destination.parent.mkdir(parents=True, exist_ok=True)
    data = source.read_bytes()
    if should_normalize_text(source):
        data = data.replace(b"\r\n", b"\n")
    destination.write_bytes(data)
    shutil.copystat(source, destination)


def stage_sync_tree(root: Path, staging_root: Path) -> list[str]:
    included: list[str] = []
    for relative in DEFAULT_SYNC_PATHS:
        source = root / relative
        if not source.exists():
            continue
        included.append(relative)
        if source.is_file():
            copy_with_normalized_text(source, staging_root / relative)
            continue
        for child in source.rglob("*"):
            rel = child.relative_to(root)
            if should_skip_path(rel):
                continue
            if child.is_dir():
                (staging_root / rel).mkdir(parents=True, exist_ok=True)
                continue
            copy_with_normalized_text(child, staging_root / rel)
    return included


def build_archive(root: Path, archive_path: Path) -> list[str]:
    with tempfile.TemporaryDirectory(prefix="resin-ssh2-stage-") as temp_dir:
        staging_root = Path(temp_dir)
        included = stage_sync_tree(root, staging_root)
        with tarfile.open(archive_path, "w:gz") as tar:
            for child in staging_root.rglob("*"):
                arcname = child.relative_to(staging_root).as_posix()
                tar.add(child, arcname=arcname, recursive=False)
    return included


def tcp_probe(host: str, port: int, timeout: int) -> None:
    with socket.create_connection((host, port), timeout=timeout):
        return


def run_remote(
    client: paramiko.SSHClient,
    command: str,
    timeout: int,
) -> tuple[int, str, str]:
    stdin, stdout, stderr = client.exec_command(command, timeout=timeout)
    del stdin
    code = stdout.channel.recv_exit_status()
    return (
        code,
        stdout.read().decode("utf-8", "ignore"),
        stderr.read().decode("utf-8", "ignore"),
    )


def detect_app_root(client: paramiko.SSHClient, remote_path: str, timeout: int) -> str:
    candidates = [remote_path, f"{remote_path}/app"]
    for candidate in candidates:
        compose_file = shlex.quote(f"{candidate}/docker-compose.yml")
        code, _, _ = run_remote(client, f"test -f {compose_file}", timeout)
        if code == 0:
            return candidate
    raise RuntimeError(
        f"docker-compose.yml not found under {remote_path} or {remote_path}/app",
    )


def upload_archive(
    client: paramiko.SSHClient,
    local_archive: Path,
    remote_archive: str,
) -> None:
    sftp = client.open_sftp()
    try:
        sftp.put(str(local_archive), remote_archive)
    finally:
        sftp.close()


def build_remote_command(
    app_root: str,
    remote_archive: str,
    service: str,
    health_timeout: int,
) -> str:
    quoted_root = shlex.quote(app_root)
    quoted_archive = shlex.quote(remote_archive)
    quoted_service = shlex.quote(service)
    return f"""set -e
cd {quoted_root}
tar -xzf {quoted_archive} -C {quoted_root}
docker compose build {quoted_service}
docker compose up -d {quoted_service}
container_id="$(docker compose ps -q {quoted_service})"
if [ -z "$container_id" ]; then
  echo "container id for service {service} not found" >&2
  exit 1
fi
deadline=$((SECONDS + {health_timeout}))
status=""
while [ "$SECONDS" -lt "$deadline" ]; do
  status="$(docker inspect -f '{{{{if .State.Health}}}}{{{{.State.Health.Status}}}}{{{{else}}}}{{{{.State.Status}}}}{{{{end}}}}' "$container_id" 2>/dev/null || echo unknown)"
  echo "HEALTH_STATUS=$status"
  if [ "$status" = "healthy" ] || [ "$status" = "running" ]; then
    break
  fi
  sleep 2
done
status="$(docker inspect -f '{{{{if .State.Health}}}}{{{{.State.Health.Status}}}}{{{{else}}}}{{{{.State.Status}}}}{{{{end}}}}' "$container_id" 2>/dev/null || echo unknown)"
if [ "$status" != "healthy" ] && [ "$status" != "running" ]; then
  echo "container did not become healthy/running in time: $status" >&2
  docker compose ps >&2 || true
  docker logs --tail 80 "$container_id" >&2 || true
  exit 1
fi
docker compose ps
rm -f {quoted_archive}
"""


def main() -> int:
    args = parse_args()
    password = args.password or getpass.getpass(
        f"SSH password for {args.user}@{args.host}: "
    )

    root = repo_root()
    with tempfile.NamedTemporaryFile(
        prefix="resin-ssh2-deploy-",
        suffix=".tar.gz",
        delete=False,
    ) as tmp:
        local_archive = Path(tmp.name)

    try:
        included = build_archive(root, local_archive)
        size_mb = local_archive.stat().st_size / (1024 * 1024)
        print(f"[1/6] Built archive {local_archive} ({size_mb:.2f} MiB)")
        print("Included paths:")
        for item in included:
            print(f"  - {item}")

        print(f"[2/6] TCP probing {args.host}:{args.port} ...")
        tcp_probe(args.host, args.port, args.timeout)
        print("TCP probe ok")

        print(f"[3/6] SSH connecting {args.user}@{args.host} ...")
        client = paramiko.SSHClient()
        client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
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
        try:
            print(f"[4/6] Detecting remote app root under {args.remote_path} ...")
            app_root = detect_app_root(client, args.remote_path, args.timeout)
            remote_archive = f"/tmp/resin-ssh2-deploy-{int(time.time())}.tar.gz"
            print(f"App root: {app_root}")

            print(f"[5/6] Uploading archive to {remote_archive} ...")
            upload_archive(client, local_archive, remote_archive)

            print("[6/6] Rebuilding and restarting remote service ...")
            command = build_remote_command(
                app_root=app_root,
                remote_archive=remote_archive,
                service=args.service,
                health_timeout=args.health_timeout,
            )
            code, stdout, stderr = run_remote(
                client,
                command,
                timeout=max(args.health_timeout + 300, 600),
            )
            if stdout.strip():
                print(stdout.strip())
            if stderr.strip():
                print(stderr.strip(), file=sys.stderr)
            if code != 0:
                print(f"Remote deploy failed with exit code {code}", file=sys.stderr)
                return code
        finally:
            client.close()
    finally:
        if not args.keep_archive and local_archive.exists():
            local_archive.unlink()

    print("Deploy completed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
