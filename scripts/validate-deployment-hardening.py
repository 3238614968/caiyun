#!/usr/bin/env python3
"""Fail CI when release/process hardening invariants drift."""
from __future__ import annotations

from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent


def read(relative: str) -> str:
    return (ROOT / relative).read_text(encoding="utf-8")


def require(content: str, token: str, location: str) -> None:
    if token not in content:
        raise SystemExit(f"{location} is missing required token: {token}")


def main() -> int:
    nginx = read("nginx-server.conf")
    for token in (
        "(?:logs|backups|bin|migrations|release)",
        "caiyun(?:-(?:api|worker))?-linux",
        "root /www/wwwroot/caiyun/dist",
        ".(?:sql|bak|dump|tar|tgz|gz|zip|7z|rar|xz|bz2|zst|json|db|sqlite|pem|key|crt|ya?ml|conf|service|sh|ps1|md|sig)",
    ):
        require(nginx, token, "nginx static artifact deny rules")

    compose = read("docker-compose.yml")
    for token in ("backend-api:", "stop_grace_period: 60s", "backend-worker:", "stop_grace_period: 75s"):
        require(compose, token, "Compose graceful shutdown")
    require(compose, 'TASK_CONFIG_SYNC_ON_STARTUP: "false"', "Compose release write gate")
    require(compose, 'TASK_QUEUE_BACKEND: ${TASK_QUEUE_BACKEND:-streams}', "Compose Streams queue default")
    require(compose, 'FIELD_CRYPTO_ALLOW_LEGACY_NO_AAD: ${FIELD_CRYPTO_ALLOW_LEGACY_NO_AAD:-false}', "Compose AAD cutover default")

    for path, token in (
        ("deploy/systemd/caiyun-api.service", "TimeoutStopSec=60s"),
        ("deploy/systemd/caiyun-worker.service", "TimeoutStopSec=75s"),
    ):
        require(read(path), token, path)

    archive = read("scripts/archive-history.sh")
    if "INSERT IGNORE INTO" in archive:
        raise SystemExit("archive script must not silently ignore copy conflicts")
    for token in ("MYSQL_PWD=", "START TRANSACTION", "SELECT ROW_COUNT(); COMMIT;"):
        require(archive, token, "archive script")

    migrator = read("backend/internal/app/migrator/run.go")
    require(migrator, "shouldSyncTaskConfig(*validateOnly, *skipTaskConfigSync)", "migrator validate-only gate")

    archive_repo = read("backend/internal/repository/history_archive_repo.go")
    if "INSERT IGNORE INTO" in archive_repo:
        raise SystemExit("archive repository must not silently ignore copy conflicts")

    rules = read("deploy/monitoring/prometheus-rules.yml")
    require(rules, 'caiyun_worker_up{job="caiyun-worker"} == 1', "Worker heartbeat alert scope")
    require(rules, 'absent(caiyun_worker_up{job="caiyun-worker"})', "Worker absence alert")

    print("deployment hardening invariants valid")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
