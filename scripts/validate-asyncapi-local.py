#!/usr/bin/env python3
"""Validate the generated local-only AsyncAPI event contract."""
from __future__ import annotations

import json
from pathlib import Path


CONTRACT = Path(__file__).resolve().parent.parent / ".local" / "openapi" / "asyncapi.json"
EXPECTED_STATUSES = ["queued", "running", "succeeded", "failed", "canceled"]


def main() -> int:
    doc = json.loads(CONTRACT.read_text(encoding="utf-8"))
    if doc.get("asyncapi") != "3.0.0":
        raise SystemExit("unexpected AsyncAPI version")
    channels = doc.get("channels", {})
    for channel in ("user-events", "user-websocket"):
        message = channels.get(channel, {}).get("messages", {}).get("operationUpdated", {})
        if message.get("$ref") != "#/components/messages/OperationUpdated":
            raise SystemExit(f"missing operation.updated message on {channel}")
    message = doc.get("components", {}).get("messages", {}).get("OperationUpdated", {})
    msg_type = message.get("payload", {}).get("properties", {}).get("type", {}).get("const")
    if msg_type != "operation.updated":
        raise SystemExit("operation event message type is missing")
    status_enum = doc.get("components", {}).get("schemas", {}).get("OperationUpdate", {}).get("properties", {}).get("status", {}).get("enum")
    if status_enum != EXPECTED_STATUSES:
        raise SystemExit(f"unexpected operation status enum: {status_enum!r}")
    print("AsyncAPI realtime contract valid")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
