#!/usr/bin/env python3
"""Validate that deployments consume the versioned migration source directly.

Fresh Docker Compose databases mount ``backend/migrations/001_init.sql``. All
Kubernetes schemas are created by the same image's ``caiyun migrate`` Job.
The previous generated K8s ConfigMap and legacy SQL copies are forbidden so a
deployment cannot silently run a stale schema snapshot.
"""
from __future__ import annotations

import argparse
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CANONICAL_BASELINE = ROOT / "backend" / "migrations" / "001_init.sql"
FORBIDDEN_COPIES = (
    ROOT / "backend" / "migrations" / "init.sql",
    ROOT / "backend" / "scripts" / "init_caiyun_database.sql",
)
K8S_MANIFEST = ROOT / "k8s" / "caiyun.yaml"
COMPOSE_MANIFEST = ROOT / "docker-compose.yml"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="validate the migration distribution (default)")
    parser.add_argument("--write", action="store_true", help="retained for automation compatibility; performs validation only")
    return parser.parse_args()


def fail(message: str) -> None:
    print(message, file=sys.stderr)
    raise SystemExit(1)


def main() -> int:
    args = parse_args()
    if not CANONICAL_BASELINE.is_file() or CANONICAL_BASELINE.stat().st_size == 0:
        fail("missing canonical migration: backend/migrations/001_init.sql")
    for path in FORBIDDEN_COPIES:
        if path.exists():
            fail(f"stale non-versioned SQL copy exists: {path.relative_to(ROOT)}")

    compose = COMPOSE_MANIFEST.read_text(encoding="utf-8")
    if "./backend/migrations/001_init.sql:/docker-entrypoint-initdb.d/001_init.sql:ro" not in compose:
        fail("Docker Compose must mount backend/migrations/001_init.sql directly")
    if "init_caiyun_database.sql" in compose:
        fail("Docker Compose still references the removed generated init SQL")

    k8s = K8S_MANIFEST.read_text(encoding="utf-8")
    forbidden_k8s_tokens = ("caiyun-mysql-init-sql", "/docker-entrypoint-initdb.d/", "001_init_caiyun_database.sql")
    for token in forbidden_k8s_tokens:
        if token in k8s:
            fail(f"Kubernetes manifest still contains legacy schema snapshot token: {token}")
    for token in ("kind: Job", "name: caiyun-migrate", "args: [\"migrate\"]"):
        if token not in k8s:
            fail(f"Kubernetes manifest is missing migration Job token: {token}")

    if args.write:
        print("versioned migrations are consumed directly; no generated SQL copies to write")
    else:
        print("migration distribution is single-source and Kubernetes uses the migration Job")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
