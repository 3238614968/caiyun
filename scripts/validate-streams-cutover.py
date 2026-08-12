#!/usr/bin/env python3
"""Keep Redis List task queue compatibility code out of production call paths."""

from __future__ import annotations

import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
INTERNAL = ROOT / "backend" / "internal"
LIST_KEY_USE = re.compile(
    r"(?:LPush|BRPopLPush|LLen|LRange|LRem)\s*\(\s*queue\.Task(?:Queue|Processing|Delayed|DeadLetter)Key"
)


def main() -> int:
    failures: list[str] = []
    for path in INTERNAL.rglob("*.go"):
        relative = path.relative_to(INTERNAL).as_posix()
        if relative.startswith("queue/") or path.name.endswith("_test.go"):
            continue
        if LIST_KEY_USE.search(path.read_text(encoding="utf-8")):
            failures.append(f"{path.relative_to(ROOT)} directly accesses a legacy Redis List task key")

    backend = (INTERNAL / "queue" / "backend.go").read_text(encoding="utf-8")
    if 'TaskQueueBackendStreams' not in backend or 'TASK_QUEUE_BACKEND", TaskQueueBackendStreams' not in backend:
        failures.append("backend/internal/queue/backend.go must default TASK_QUEUE_BACKEND to streams")
    if 'APP_ENV", "")), "production")' not in backend or "仅支持 TASK_QUEUE_BACKEND=streams" not in backend:
        failures.append("backend/internal/queue/backend.go must reject Redis List in production")

    if failures:
        print("Streams cutover validation failed:", file=sys.stderr)
        print("\n".join(failures), file=sys.stderr)
        return 1
    print("Streams cutover validation passed: no production Redis List task-key access")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
