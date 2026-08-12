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
    ("/api/v1/operations/{id}", "get"),
    ("/api/v1/operations/{id}/cancel", "post"),
    ("/api/v1/exchange/immediate", "post"),
    ("/api/v1/admin/exchange/execute-monthly", "post"),
    ("/api/v1/admin/tasks/dead-letters", "get"),
    ("/api/v1/admin/tasks/dead-letters/{id}/replay", "post"),
    ("/ws", "get"),
    ("/readyz", "get"),
}
ASYNC_OPERATIONS = {
    "/api/v1/accounts/{id}/trigger",
    "/api/v1/tasks/trigger-all",
    "/api/v1/exchange/tasks/{id}/execute",
    "/api/v1/exchange/tasks/batch-execute",
    "/api/v1/exchange/immediate",
    "/api/v1/admin/exchange/execute-monthly",
}
TYPED_RESOURCE_OPERATIONS = {
    ("/api/v1/accounts", "get", "AccountList"),
    ("/api/v1/accounts", "post", "Account"),
    ("/api/v1/accounts/{id}", "get", "Account"),
    ("/api/v1/accounts/{id}", "put", "Account"),
    ("/api/v1/accounts/{id}/refresh", "post", "Account"),
    ("/api/v1/accounts/sms/send", "post", "SmsSendResult"),
    ("/api/v1/accounts/sms/status/{task_id}", "get", "SmsStatus"),
    ("/api/v1/accounts/sms/verify", "post", "Account"),
    ("/api/v1/tasks/logs", "get", "TaskLogList"),
    ("/api/v1/tasks/status", "get", "TaskStatus"),
    ("/api/v1/tasks/queue-status", "get", "QueueStatus"),
    ("/api/v1/admin/tasks/dead-letters", "get", "DeadLetterList"),
    ("/api/v1/exchange/rules", "get", "ExchangeRuleList"),
    ("/api/v1/exchange/rules", "post", "ExchangeRuleResponse"),
    ("/api/v1/exchange/accounts", "get", "ExchangeRuleList"),
    ("/api/v1/exchange/accounts", "post", "ExchangeRuleResponse"),
    ("/api/v1/exchange/tasks", "get", "ExchangeTaskList"),
    ("/api/v1/exchange/records", "get", "ExchangeRecordList"),
    ("/api/v1/products/search", "get", "ProductList"),
    ("/api/v1/products/categories", "get", "ProductCategoryList"),
    ("/api/v1/auth/register", "post", "AuthResponse"),
    ("/api/v1/auth/login", "post", "AuthResponse"),
    ("/api/v1/auth/refresh", "post", "RefreshTokenResponse"),
    ("/api/v1/auth/me", "get", "AuthUser"),
    ("/api/v1/stats/dashboard", "get", "DashboardData"),
    ("/api/v1/stats/cloud", "get", "CloudStatList"),
    ("/api/v1/stats/trend", "get", "TrendData"),
    ("/api/v1/stats/total-cloud", "get", "TotalCloudCount"),
    ("/api/v1/admin/users", "get", "AdminUserList"),
    ("/api/v1/admin/accounts/search", "get", "AdminAccountSearchList"),
    ("/api/v1/admin/accounts/summaries", "get", "AdminAccountSummaryList"),
    ("/api/v1/admin/dashboard", "get", "AdminDashboardData"),
    ("/api/v1/admin/stats/overview", "get", "AdminStatsOverview"),
    ("/api/v1/admin/task-configs", "get", "TaskConfigList"),
    ("/api/v1/exchange/config", "get", "ExchangeConfigPublic"),
    ("/api/v1/announcements", "get", "AnnouncementList"),
    ("/api/v1/announcements/popup", "get", "AnnouncementPopupList"),
    ("/api/v1/admin/exchange/config", "get", "ExchangeConfig"),
    ("/api/v1/admin/exchange/rules", "get", "ExchangeRuleList"),
    ("/api/v1/admin/exchange/tasks", "get", "ExchangeTaskList"),
    ("/api/v1/admin/announcements", "get", "AnnouncementList"),
    ("/api/v1/admin/announcements/{id}", "get", "AnnouncementResponse"),
}
PUBLIC_RESPONSE_SCHEMAS = {
    "Account", "AccountUser", "TaskLog", "TaskLogAccount", "Product",
    "ExchangeRule", "ExchangeTask", "ExchangeRecord", "AuthUser",
    "AdminUser", "AdminAccountSearch", "AdminAccountSummary", "AdminAccountRank",
    "DashboardData", "DashboardAccountRank", "CloudStat", "TaskConfig", "Announcement",
}
FORBIDDEN_PUBLIC_PROPERTIES = {"auth", "token", "jwt_token", "execution_token", "source_operation_id"}
TYPED_REQUEST_OPERATIONS = {
    ("/api/v1/accounts/sms/send", "post"): "SmsSendRequest",
    ("/api/v1/accounts/sms/verify", "post"): "SmsLoginRequest",
    ("/api/v1/exchange/tasks/batch-execute", "post"): "BatchExecuteExchangeTasksRequest",
    ("/api/v1/exchange/immediate", "post"): "ImmediateExchangeRequest",
}


def success_schema_ref(operation: dict) -> str | None:
    """Return the OpenAPI ref used by a canonical JSON success response, if any."""
    for status in ("200", "201", "202"):
        response = operation.get("responses", {}).get(status, {})
        direct = response.get("$ref")
        if direct:
            return direct
        ref = response.get("content", {}).get("application/json", {}).get("schema", {}).get("$ref")
        if ref:
            return ref
    return None

def main() -> int:
    doc = json.loads(CONTRACT.read_text(encoding="utf-8"))
    if doc.get("openapi") != "3.1.0":
        raise SystemExit("unexpected OpenAPI version")
    components = doc.get("components", {})
    if set(components.get("securitySchemes", {})) != {"CookieAuth", "CSRFToken"}:
        raise SystemExit("OpenAPI must describe CookieAuth and CSRFToken security")
    paths = doc.get("paths", {})
    missing = sorted(f"{method.upper()} {path}" for path, method in REQUIRED if method not in paths.get(path, {}))
    if missing:
        raise SystemExit("missing critical routes:\n" + "\n".join(missing))
    if len(paths) < 80:
        raise SystemExit(f"route inventory unexpectedly small: {len(paths)}")
    if any(":" in path for path in paths):
        raise SystemExit("OpenAPI paths must use {parameter}, not Gin :parameter syntax")
    operation = paths["/api/v1/operations/{id}"]["get"]
    if operation.get("parameters") != [{"name": "id", "in": "path", "required": True, "schema": {"type": "string"}}]:
        raise SystemExit("operation path parameter schema is missing")

    schemas = components.get("schemas", {})
    for schema in ("Operation", "OperationAccepted", "OperationEnvelope"):
        if schema not in schemas:
            raise SystemExit(f"missing typed operation schema: {schema}")
    statuses = schemas["Operation"].get("properties", {}).get("status", {}).get("enum")
    if statuses != ["queued", "running", "succeeded", "failed", "canceled"]:
        raise SystemExit(f"unexpected operation statuses: {statuses!r}")
    accepted_data = schemas["OperationAccepted"].get("properties", {}).get("data", {})
    if accepted_data.get("$ref") != "#/components/schemas/Operation":
        raise SystemExit("accepted operation response must return the full Operation DTO for idempotency replay")
    get_responses = operation.get("responses", {})
    if get_responses.get("200", {}).get("$ref") != "#/components/responses/OperationResult":
        raise SystemExit("operation GET must return typed OperationResult")
    cancel = paths["/api/v1/operations/{id}/cancel"]["post"]
    if cancel.get("operationId") != "cancel_operation" or cancel.get("responses", {}).get("200", {}).get("$ref") != "#/components/responses/OperationResult":
        raise SystemExit("operation cancel must return the typed final Operation state")
    for path in ASYNC_OPERATIONS:
        post = paths.get(path, {}).get("post")
        if post is None:
            raise SystemExit(f"missing async operation route: POST {path}")
        parameters = post.get("parameters", [])
        if not any(parameter.get("in") == "header" and parameter.get("name") == "Idempotency-Key" and parameter.get("required") for parameter in parameters):
            raise SystemExit(f"async route missing required Idempotency-Key: POST {path}")
        if post.get("responses", {}).get("202", {}).get("$ref") != "#/components/responses/Accepted":
            raise SystemExit(f"async route missing typed 202 response: POST {path}")
        if post.get("x-operation-lifecycle") != "queued -> running -> succeeded|failed|canceled":
            raise SystemExit(f"async route missing operation lifecycle: POST {path}")
    for path, method, schema_name in TYPED_RESOURCE_OPERATIONS:
        operation = paths.get(path, {}).get(method)
        responses = operation.get("responses", {}) if operation else {}
        response = responses.get("201") if (path, method) == ("/api/v1/auth/register", "post") else responses.get("200")
        response_ref = response.get("content", {}).get("application/json", {}).get("schema", {}).get("$ref") if response else None
        if response_ref != f"#/components/schemas/{schema_name}Envelope":
            raise SystemExit(f"typed DTO response missing: {method.upper()} {path} -> {schema_name}")
    for (path, method), schema_name in TYPED_REQUEST_OPERATIONS.items():
        request_ref = paths.get(path, {}).get(method, {}).get("requestBody", {}).get("content", {}).get("application/json", {}).get("schema", {}).get("$ref")
        if request_ref != f"#/components/schemas/{schema_name}":
            raise SystemExit(f"typed request DTO missing: {method.upper()} {path} -> {schema_name}")
    export = paths.get("/api/v1/exchange/records/export", {}).get("get", {})
    export_content = export.get("responses", {}).get("200", {}).get("content", {})
    if export_content.get("application/json", {}).get("schema", {}).get("$ref") != "#/components/schemas/ExchangeRecordExport" or "text/csv" not in export_content:
        raise SystemExit("record export must document JSON and CSV response formats")
    for path, methods in paths.items():
        if not path.startswith("/api/v1/"):
            continue
        for method, typed_operation in methods.items():
            if method not in {"get", "post", "put", "patch", "delete"}:
                continue
            if success_schema_ref(typed_operation) == "#/components/responses/Success":
                raise SystemExit(f"canonical route has a generic success response: {method.upper()} {path}")
    replay = paths["/api/v1/admin/tasks/dead-letters/{id}/replay"]["post"]
    replay_request_ref = replay.get("requestBody", {}).get("content", {}).get("application/json", {}).get("schema", {}).get("$ref")
    if replay_request_ref != "#/components/schemas/ReplayDeadLetterRequest":
        raise SystemExit("dead-letter replay must require the approved replay request schema")
    for schema_name in PUBLIC_RESPONSE_SCHEMAS:
        properties = schemas.get(schema_name, {}).get("properties", {})
        exposed = sorted(FORBIDDEN_PUBLIC_PROPERTIES.intersection(properties))
        if exposed:
            raise SystemExit(f"public schema exposes sensitive fields: {schema_name}: {', '.join(exposed)}")
    print(f"OpenAPI route inventory valid: {len(paths)} paths")
    return 0

if __name__ == "__main__":
    raise SystemExit(main())
