#!/usr/bin/env python3
"""Generate the local-only AsyncAPI contract for SSE and WebSocket events."""
from __future__ import annotations

import argparse
import json
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
OUTPUT_DIR = (ROOT / ".local" / "openapi").resolve()


def output_path(value: str) -> Path:
    candidate = Path(value)
    if not candidate.is_absolute():
        candidate = ROOT / candidate
    candidate = candidate.resolve()
    try:
        candidate.relative_to(OUTPUT_DIR)
    except ValueError as exc:
        raise ValueError(f"--out must be inside {OUTPUT_DIR}; generated contracts are local-only") from exc
    return candidate


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out", default=".local/openapi/asyncapi.json")
    parser.add_argument("--version", default="local")
    args = parser.parse_args()
    try:
        out = output_path(args.out)
    except ValueError as exc:
        parser.error(str(exc))

    operation_update = {
        "payload": {
            "type": "object",
            "required": ["message_id", "sequence", "type", "data"],
            "properties": {
                "message_id": {"type": "string", "format": "uuid"},
                "sequence": {"type": "integer", "minimum": 1},
                "type": {"type": "string", "const": "operation.updated"},
                "created_at": {"type": "integer", "description": "Unix milliseconds"},
                "expires_at": {"type": "integer", "description": "Unix milliseconds"},
                "data": {"$ref": "#/components/schemas/OperationUpdate"},
            },
        }
    }
    document = {
        "asyncapi": "3.0.0",
        "info": {
            "title": "Caiyun realtime events",
            "version": args.version,
            "description": "Local-only generated SSE/WebSocket contract. Do not commit generated output.",
        },
        "servers": {
            "local": {
                "host": "localhost:8080",
                "protocol": "http",
                "description": "SSE uses GET /events; WebSocket uses GET /ws.",
            }
        },
        "channels": {
            "user-events": {
                "address": "/events",
                "messages": {"operationUpdated": {"$ref": "#/components/messages/OperationUpdated"}},
                "description": "Cookie-authenticated user event stream. Clients resume SSE replay with Last-Event-ID.",
            },
            "user-websocket": {
                "address": "/ws",
                "messages": {"operationUpdated": {"$ref": "#/components/messages/OperationUpdated"}},
                "description": "Optional Cookie-authenticated WebSocket transport for the same event envelopes.",
            },
        },
        "operations": {
            "receiveUserEvents": {
                "action": "receive",
                "channel": {"$ref": "#/channels/user-events"},
                "messages": [{"$ref": "#/channels/user-events/messages/operationUpdated"}],
            },
            "receiveUserWebSocketEvents": {
                "action": "receive",
                "channel": {"$ref": "#/channels/user-websocket"},
                "messages": [{"$ref": "#/channels/user-websocket/messages/operationUpdated"}],
            },
        },
        "components": {
            "messages": {"OperationUpdated": operation_update},
            "schemas": {
                "OperationUpdate": {
                    "type": "object",
                    "required": ["operation_id", "type", "status", "attempt_count", "queued_at", "updated_at"],
                    "properties": {
                        "operation_id": {"type": "string", "format": "uuid"},
                        "type": {"type": "string"},
                        "status": {"type": "string", "enum": ["queued", "running", "succeeded", "failed", "canceled"]},
                        "account_id": {"type": "integer", "minimum": 1},
                        "resource_id": {"type": "integer", "minimum": 1},
                        "attempt_count": {"type": "integer", "minimum": 0},
                        "error_summary": {"type": "string"},
                        "queued_at": {"type": "string", "format": "date-time"},
                        "started_at": {"type": "string", "format": "date-time"},
                        "completed_at": {"type": "string", "format": "date-time"},
                        "updated_at": {"type": "string", "format": "date-time"},
                    },
                }
            },
        },
    }
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(document, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"AsyncAPI contract written to {out.relative_to(ROOT)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
