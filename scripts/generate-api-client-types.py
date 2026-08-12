#!/usr/bin/env python3
"""Generate local TypeScript API DTOs from local OpenAPI/AsyncAPI.

All generated artifacts remain ignored by Git. Frontend lifecycle scripts
regenerate this DTO file before typecheck, unit-test and production-build so a
fresh checkout cannot accidentally depend on a developer-local artifact.
"""
from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parent.parent
OPENAPI = ROOT / ".local" / "openapi" / "openapi.json"
ASYNCAPI = ROOT / ".local" / "openapi" / "asyncapi.json"
DEFAULT_OUTPUT = ROOT / "frontend" / "src" / "api" / "generated" / "operation-contract.ts"
EXPECTED_STATUSES = ["queued", "running", "succeeded", "failed", "canceled"]
CORE_OPENAPI_SCHEMAS = [
    "AccountUser",
    "Account",
    "AccountList",
    "TaskLogAccount",
    "TaskLog",
    "TaskLogList",
    "QueueBackendMeta",
    "QueueStatus",
    "DeadLetter",
    "DeadLetterList",
    "Product",
    "ProductList",
    "ExchangeRule",
    "ExchangeRuleList",
    "ExchangeTask",
    "ExchangeTaskList",
    "ExchangeTaskCreateResult",
    "ExchangeTaskCreateResponse",
    "ExchangeRecord",
    "RecordStats",
    "ExchangeRecordList",
    "CreateAccountRequest",
    "UpdateAccountRequest",
    "AddExchangeRuleRequest",
    "UpdateExchangeRuleRequest",
    "CreateExchangeTaskRequest",
    "UpdateExchangeTaskRequest",
    "UpdateProductsRequest",
    "SmsSendRequest",
    "SmsSendResult",
    "SmsStatus",
    "SmsLoginRequest",
    "TaskStatusItem",
    "TaskStatus",
    "ProductCategoryList",
    "ExchangeRuleResponse",
    "BatchExecuteExchangeTasksRequest",
    "ImmediateExchangeRequest",
    "ReplayDeadLetterRequest",
    "AuthUser",
    "AuthResponse",
    "RefreshTokenResponse",
    "RegisterRequest",
    "LoginRequest",
    "RefreshTokenRequest",
    "SendPasswordResetCodeRequest",
    "ResetPasswordRequest",
    "DashboardTrendPoint",
    "DashboardAccountRank",
    "DashboardData",
    "CloudStat",
    "CloudStatList",
    "TrendData",
    "TotalCloudCount",
    "AdminUser",
    "AdminUserList",
    "AdminAccountSearch",
    "AdminAccountSearchList",
    "AdminAccountSummary",
    "AdminAccountSummaryList",
    "AdminAccountRank",
    "AdminDashboardData",
    "AdminStatsOverview",
    "TaskConfig",
    "TaskConfigList",
    "UpdateUserRoleRequest",
    "ResetUserPasswordRequest",
    "UpdateAccountStatusRequest",
    "UpdateTaskConfigRequest",
    "ExchangeConfig",
    "ExchangeConfigPublic",
    "UpdateExchangeConfigRequest",
    "Announcement",
    "AnnouncementList",
    "AnnouncementResponse",
    "AnnouncementPopupList",
    "CreateAnnouncementRequest",
    "UpdateAnnouncementRequest",
]


def project_path(value: str) -> Path:
    path = Path(value)
    if not path.is_absolute():
        path = ROOT / path
    return path.resolve()


def ts_type(schema: dict[str, Any]) -> str:
    if ref := schema.get("$ref"):
        if not isinstance(ref, str) or not ref.startswith("#/components/schemas/"):
            raise ValueError(f"unsupported schema reference: {ref!r}")
        return ref.rsplit("/", 1)[-1]
    for union_key in ("oneOf", "anyOf"):
        options = schema.get(union_key)
        if isinstance(options, list) and options:
            return " | ".join(ts_type(option) for option in options if isinstance(option, dict))
    if "const" in schema:
        return json.dumps(schema["const"], ensure_ascii=False)
    enum = schema.get("enum")
    if isinstance(enum, list) and enum:
        return " | ".join(json.dumps(value, ensure_ascii=False) for value in enum)
    kind = schema.get("type")
    if isinstance(kind, list):
        return " | ".join("null" if value == "null" else ts_type({"type": value}) for value in kind)
    if kind == "string":
        return "string"
    if kind in {"integer", "number"}:
        return "number"
    if kind == "boolean":
        return "boolean"
    if kind == "array":
        return f"{ts_type(schema.get('items', {}))}[]"
    if kind == "object":
        return "Record<string, unknown>"
    return "unknown"


def interface_lines(name: str, schema: dict[str, Any], *, status_type: bool = False, field_types: dict[str, str] | None = None) -> list[str]:
    properties = schema.get("properties")
    required = set(schema.get("required", []))
    if not isinstance(properties, dict):
        raise ValueError(f"{name} schema has no object properties")
    field_types = field_types or {}
    lines = [f"export interface {name} {{"]
    for field, field_schema in properties.items():
        if not isinstance(field_schema, dict):
            raise ValueError(f"{name}.{field} schema is invalid")
        optional = "" if field in required else "?"
        field_type = field_types.get(field)
        if field_type is None:
            field_type = "OperationStatus" if status_type and field == "status" and "enum" in field_schema else ts_type(field_schema)
        lines.append(f"  {field}{optional}: {field_type}")
    lines.append("}")
    return lines


def required_schema(doc: dict[str, Any], name: str) -> dict[str, Any]:
    schema = doc.get("components", {}).get("schemas", {}).get(name)
    if not isinstance(schema, dict):
        raise ValueError(f"OpenAPI missing schema {name}")
    return schema


def render(openapi: dict[str, Any], asyncapi: dict[str, Any]) -> str:
    operation = required_schema(openapi, "Operation")
    update = asyncapi.get("components", {}).get("schemas", {}).get("OperationUpdate")
    event = asyncapi.get("components", {}).get("messages", {}).get("OperationUpdated", {}).get("payload")
    if not isinstance(update, dict) or not isinstance(event, dict):
        raise ValueError("AsyncAPI missing OperationUpdate or OperationUpdated payload")

    statuses = operation.get("properties", {}).get("status", {}).get("enum")
    realtime_statuses = update.get("properties", {}).get("status", {}).get("enum")
    if statuses != EXPECTED_STATUSES or realtime_statuses != EXPECTED_STATUSES:
        raise ValueError("OpenAPI and AsyncAPI operation statuses must match the lifecycle")
    if event.get("properties", {}).get("type", {}).get("const") != "operation.updated":
        raise ValueError("OperationUpdated event must use operation.updated")

    lines = [
        "/* eslint-disable */",
        "// Code generated by scripts/generate-api-client-types.py; DO NOT EDIT.",
        "// Source schemas are local-only .local/openapi/openapi.json and asyncapi.json.",
        "",
        f"export const operationStatuses = {json.dumps(statuses, ensure_ascii=False)} as const",
        "export type OperationStatus = typeof operationStatuses[number]",
        "",
    ]
    lines.extend(interface_lines("OperationResponse", operation, status_type=True))
    lines.append("")
    for name in CORE_OPENAPI_SCHEMAS:
        lines.extend(interface_lines(name, required_schema(openapi, name)))
        lines.append("")
    lines.extend(interface_lines("OperationUpdate", update, status_type=True))
    lines.append("")
    lines.extend([
        "export interface OperationAcceptedResponse {",
        "  code: 0",
        "  business_code?: string",
        "  message: string",
        "  trace_id?: string",
        "  data: OperationResponse",
        "}",
        "",
    ])
    lines.extend(interface_lines("OperationUpdatedEvent", event, field_types={"data": "OperationUpdate"}))
    return "\n".join(lines) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out", default=str(DEFAULT_OUTPUT.relative_to(ROOT)))
    parser.add_argument("--check", action="store_true", help="fail when the committed generated source differs")
    args = parser.parse_args()
    if not OPENAPI.is_file() or not ASYNCAPI.is_file():
        raise SystemExit("generate local OpenAPI and AsyncAPI before generating TypeScript types")
    output = project_path(args.out)
    try:
        output.relative_to(ROOT / "frontend" / "src")
    except ValueError as exc:
        raise SystemExit("--out must stay below frontend/src") from exc

    generated = render(
        json.loads(OPENAPI.read_text(encoding="utf-8")),
        json.loads(ASYNCAPI.read_text(encoding="utf-8")),
    )
    current = output.read_text(encoding="utf-8") if output.is_file() else ""
    if args.check:
        if current != generated:
            raise SystemExit(f"generated TypeScript contract is stale: {output.relative_to(ROOT)}")
        print(f"TypeScript API contract valid: {output.relative_to(ROOT)}")
        return 0
    output.parent.mkdir(parents=True, exist_ok=True)
    if current != generated:
        output.write_text(generated, encoding="utf-8")
    print(f"TypeScript API contract written to {output.relative_to(ROOT)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
