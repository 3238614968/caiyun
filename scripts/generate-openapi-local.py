#!/usr/bin/env python3
"""Generate a lightweight local OpenAPI contract from Go @Router comments.

This script is intentionally dependency-free and writes to .local/openapi by default.
Generated OpenAPI files are local-only artifacts and must not be committed.
"""
from __future__ import annotations

import argparse
import json
import re
from pathlib import Path
from typing import Any

PROJECT_ROOT = Path(__file__).resolve().parent.parent
LOCAL_OPENAPI_DIR = (PROJECT_ROOT / ".local" / "openapi").resolve()

ROUTER_RE = re.compile(r"@Router\s+(\S+)\s+\[(\w+)\]")
SUMMARY_RE = re.compile(r"@Summary\s+(.+)")
TAGS_RE = re.compile(r"@Tags\s+(.+)")
SECURITY_RE = re.compile(r"@Security\s+(.+)")
ROUTE_CALL_RE = re.compile(r'\b(\w+)\.(GET|POST|PUT|PATCH|DELETE)\(\s*"([^"]+)"')

ROUTE_PREFIXES = {
    "r": [""],
    "public": ["/api/auth", "/api/v1/auth"],
    "protected": ["/api", "/api/v1"],
    "accounts": ["/api/accounts", "/api/v1/accounts"],
    "tasks": ["/api/tasks", "/api/v1/tasks"],
    "stats": ["/api/stats", "/api/v1/stats"],
    "exchange": ["/api/exchange", "/api/v1/exchange"],
    "products": ["/api/products", "/api/v1/products"],
    "admin": ["/api/admin", "/api/v1/admin"],
}


def versioned_path(path: str) -> str | None:
    if not path.startswith("/api/"):
        return None
    if path.startswith("/api/v1/"):
        return None
    return "/api/v1/" + path.removeprefix("/api/")


def collect_operations(handlers_dir: Path) -> dict[str, dict[str, dict[str, Any]]]:
    paths: dict[str, dict[str, dict[str, Any]]] = {}
    for file in sorted(handlers_dir.glob("*.go")):
        lines = file.read_text(encoding="utf-8", errors="ignore").splitlines()
        comment_block: list[str] = []
        for line in lines:
            stripped = line.strip()
            if stripped.startswith("//"):
                comment_block.append(stripped[2:].strip())
                continue
            if "func " in stripped and comment_block:
                operation = operation_from_comments(comment_block, file)
                if operation:
                    path, method, op = operation
                    add_operation(paths, path, method, op)
                    v1_path = versioned_path(path)
                    if v1_path:
                        v1_op = dict(op)
                        v1_op["description"] = (v1_op.get("description", "") + "\nVersioned /api/v1 endpoint.").strip()
                        add_operation(paths, v1_path, method, v1_op)
                comment_block = []
            elif stripped and not stripped.startswith("//"):
                comment_block = []
    return paths


def merge_registered_routes(paths: dict[str, dict[str, dict[str, Any]]], routes_file: Path) -> None:
    """Merge the actual Gin route inventory so undocumented handlers cannot disappear from the contract."""
    source = routes_file.read_text(encoding="utf-8", errors="ignore")
    for receiver, method, suffix in ROUTE_CALL_RE.findall(source):
        prefixes = ROUTE_PREFIXES.get(receiver)
        if not prefixes:
            continue
        for prefix in prefixes:
            path = prefix + suffix
            operation = {
                "summary": f"{method} {path}",
                "tags": [receiver],
                "responses": {
                    "200": {"description": "OK"},
                    "202": {"description": "Accepted"},
                    "400": {"description": "Bad Request"},
                    "401": {"description": "Unauthorized"},
                    "500": {"description": "Internal Server Error"},
                },
                "x-source-file": "backend/internal/app/api/routes.go",
            }
            public_auth = path.endswith("/auth/login") or path.endswith("/auth/register") or path.endswith("/auth/refresh")
            public_probe = path in {"/", "/livez", "/startupz", "/readyz", "/health"}
            if not public_auth and not public_probe:
                operation["security"] = [{"BearerAuth": []}]
            add_operation(paths, path, method.lower(), operation)

def operation_from_comments(comments: list[str], file: Path) -> tuple[str, str, dict[str, Any]] | None:
    router = None
    summary = None
    tags: list[str] = []
    security: list[dict[str, list[str]]] = []
    for comment in comments:
        if m := ROUTER_RE.search(comment):
            router = (m.group(1), m.group(2).lower())
        elif m := SUMMARY_RE.search(comment):
            summary = m.group(1).strip()
        elif m := TAGS_RE.search(comment):
            tags = [item.strip() for item in m.group(1).split(",") if item.strip()]
        elif m := SECURITY_RE.search(comment):
            security.append({m.group(1).strip(): []})
    if not router:
        return None
    path, method = router
    op: dict[str, Any] = {
        "summary": summary or f"{method.upper()} {path}",
        "tags": tags or [file.stem],
        "responses": {
            "200": {"description": "OK"},
            "400": {"description": "Bad Request"},
            "401": {"description": "Unauthorized"},
            "500": {"description": "Internal Server Error"},
        },
        "x-source-file": str(file.as_posix()),
    }
    if security:
        op["security"] = security
    elif not path.startswith("/api/auth/login") and not path.startswith("/api/auth/register"):
        op["security"] = [{"BearerAuth": []}]
    return path, method, op


def add_operation(paths: dict[str, dict[str, dict[str, Any]]], path: str, method: str, op: dict[str, Any]) -> None:
    paths.setdefault(path, {})[method] = op


def project_path(value: str) -> Path:
    """Resolve relative arguments from the repository root, not the caller's CWD."""
    path = Path(value)
    if not path.is_absolute():
        path = PROJECT_ROOT / path
    return path.resolve()


def local_openapi_output(value: str) -> Path:
    """Return a local-only output path, rejecting paths that Git could track."""
    output = project_path(value)
    try:
        output.relative_to(LOCAL_OPENAPI_DIR)
    except ValueError as exc:
        raise ValueError(
            f"--out must be inside {LOCAL_OPENAPI_DIR}; generated contracts are local-only"
        ) from exc
    return output


def main() -> int:
    parser = argparse.ArgumentParser(description="Generate local OpenAPI contract from Go @Router comments")
    parser.add_argument("--handlers-dir", default="backend/internal/handlers")
    parser.add_argument("--routes-file", default="backend/internal/app/api/routes.go")
    parser.add_argument(
        "--out",
        default=".local/openapi/openapi.json",
        help="local-only output below .local/openapi (default: %(default)s)",
    )
    parser.add_argument("--title", default="Caiyun API")
    parser.add_argument("--version", default="local")
    args = parser.parse_args()

    handlers_dir = project_path(args.handlers_dir)
    try:
        out = local_openapi_output(args.out)
    except ValueError as exc:
        parser.error(str(exc))
    if not handlers_dir.is_dir():
        parser.error(f"handlers directory does not exist: {handlers_dir}")
    paths = collect_operations(handlers_dir)
    routes_file = project_path(args.routes_file)
    if not routes_file.is_file():
        parser.error(f"routes file does not exist: {routes_file}")
    merge_registered_routes(paths, routes_file)
    doc = {
        "openapi": "3.0.3",
        "info": {
            "title": args.title,
            "version": args.version,
            "description": "Local-only generated contract. Do not commit generated output.",
        },
        "servers": [
            {"url": "http://localhost:8080", "description": "Local API"},
        ],
        "components": {
            "securitySchemes": {
                "BearerAuth": {"type": "http", "scheme": "bearer", "bearerFormat": "JWT"},
            }
        },
        "paths": dict(sorted(paths.items())),
    }
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(doc, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"OpenAPI contract written to {out.relative_to(PROJECT_ROOT)} ({len(paths)} paths)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
