#!/usr/bin/env python3
"""Validate the generated local-only OpenAPI route inventory."""
from __future__ import annotations
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
CONTRACT = ROOT / ".local" / "openapi" / "openapi.json"
REQUIRED = {
    ("/api/v1/auth/refresh", "post"),
    ("/api/v1/auth/logout", "post"),
    ("/api/v1/auth/logout-all", "post"),
    ("/api/v1/operations/:id", "get"),
    ("/api/v1/operations/:id/cancel", "post"),
    ("/api/v1/exchange/immediate", "post"),
    ("/api/v1/admin/exchange/execute-monthly", "post"),
    ("/ws", "get"),
    ("/readyz", "get"),
}

def main() -> int:
    doc = json.loads(CONTRACT.read_text(encoding="utf-8"))
    if doc.get("openapi") != "3.0.3":
        raise SystemExit("unexpected OpenAPI version")
    paths = doc.get("paths", {})
    missing = sorted(f"{method.upper()} {path}" for path, method in REQUIRED if method not in paths.get(path, {}))
    if missing:
        raise SystemExit("missing critical routes:\n" + "\n".join(missing))
    if len(paths) < 80:
        raise SystemExit(f"route inventory unexpectedly small: {len(paths)}")
    print(f"OpenAPI route inventory valid: {len(paths)} paths")
    return 0

if __name__ == "__main__":
    raise SystemExit(main())
