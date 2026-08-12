#!/usr/bin/env python3
"""Generate a local OpenAPI 3.1 contract from Go comments and route wiring.

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
ROUTE_CALL_RE = re.compile(r'\b(\w+)\.(GET|POST|PUT|PATCH|DELETE)\(\s*"([^"]*)"')
PATH_PARAMETER_RE = re.compile(r":([A-Za-z_][A-Za-z0-9_]*)")

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

ASYNC_OPERATION_PATHS = {
    "/api/v1/accounts/{id}/trigger",
    "/api/v1/tasks/trigger-all",
    "/api/v1/exchange/tasks/{id}/execute",
    "/api/v1/exchange/tasks/batch-execute",
    "/api/v1/exchange/immediate",
    "/api/v1/admin/exchange/execute-monthly",
}

OPERATION_STATUS_PATH = "/api/v1/operations/{id}"
OPERATION_CANCEL_PATH = "/api/v1/operations/{id}/cancel"


def versioned_path(path: str) -> str | None:
    if not path.startswith("/api/"):
        return None
    if path.startswith("/api/v1/"):
        return None
    return "/api/v1/" + path.removeprefix("/api/")


def normalize_path(path: str) -> str:
    """Convert Gin :parameters to OpenAPI {parameters}."""
    return PATH_PARAMETER_RE.sub(r"{\1}", path)


def request_security(path: str, method: str) -> list[dict[str, list[str]]]:
    public_auth = path.endswith("/auth/login") or path.endswith("/auth/register") or path.endswith("/auth/refresh")
    public_probe = path in {"/", "/livez", "/startupz", "/readyz", "/health"}
    if public_auth or public_probe:
        return []
    schemes: dict[str, list[str]] = {"CookieAuth": []}
    if method.lower() in {"post", "put", "patch", "delete"} and path.startswith("/api/"):
        schemes["CSRFToken"] = []
    return [schemes]


def default_responses() -> dict[str, dict[str, Any]]:
    return {
        "200": {"$ref": "#/components/responses/Success"},
        "202": {"$ref": "#/components/responses/Accepted"},
        "400": {"$ref": "#/components/responses/BadRequest"},
        "401": {"$ref": "#/components/responses/Unauthorized"},
        "403": {"$ref": "#/components/responses/Forbidden"},
        "409": {"$ref": "#/components/responses/Conflict"},
        "429": {"$ref": "#/components/responses/TooManyRequests"},
        "500": {"$ref": "#/components/responses/InternalError"},
    }


def parameters_for_path(path: str) -> list[dict[str, Any]]:
    return [
        {
            "name": name,
            "in": "path",
            "required": True,
            "schema": {"type": "string"},
        }
        for name in re.findall(r"\{([A-Za-z_][A-Za-z0-9_]*)\}", path)
    ]


def query_parameter(name: str, schema: dict[str, Any], description: str, *, required: bool = False) -> dict[str, Any]:
    """Build a documented query parameter for an explicitly typed route."""
    return {
        "name": name,
        "in": "query",
        "required": required,
        "description": description,
        "schema": schema,
    }


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
                "responses": default_responses(),
                "x-source-file": "backend/internal/app/api/routes.go",
            }
            security = request_security(path, method)
            if security:
                operation["security"] = security
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
        "responses": default_responses(),
        "x-source-file": str(file.as_posix()),
    }
    path = normalize_path(path)
    if security:
        # Handler comments historically named BearerAuth even though browsers
        # authenticate with HttpOnly cookies.  Keep comment parsing, but emit
        # the transport actually used by the runtime.
        op["security"] = request_security(path, method)
    else:
        generated_security = request_security(path, method)
        if generated_security:
            op["security"] = generated_security
    return path, method, op


def add_operation(paths: dict[str, dict[str, dict[str, Any]]], path: str, method: str, op: dict[str, Any]) -> None:
    path = normalize_path(path)
    parameters = parameters_for_path(path)
    if parameters:
        op = dict(op)
        op["parameters"] = parameters
    # Mark the legacy surface as compatibility-only in the local contract.
    if path.startswith("/api/") and not path.startswith("/api/v1/"):
        op = dict(op)
        op["deprecated"] = True
    paths.setdefault(path, {})[method] = op


def idempotency_parameter() -> dict[str, Any]:
    return {
        "name": "Idempotency-Key",
        "in": "header",
        "required": True,
        "description": "Unique client-generated key. Reusing it with a different command returns 409 IDEMPOTENCY_CONFLICT.",
        "schema": {"type": "string", "minLength": 1, "maxLength": 191},
    }


def typed_operation_responses() -> dict[str, dict[str, Any]]:
    return {
        "202": {"$ref": "#/components/responses/Accepted"},
        "400": {"$ref": "#/components/responses/BadRequest"},
        "401": {"$ref": "#/components/responses/Unauthorized"},
        "403": {"$ref": "#/components/responses/Forbidden"},
        "409": {"$ref": "#/components/responses/Conflict"},
        "429": {"$ref": "#/components/responses/TooManyRequests"},
        "500": {"$ref": "#/components/responses/InternalError"},
    }


def resource_schemas() -> dict[str, dict[str, Any]]:
    """Return versioned public DTOs for the most-used account/task/exchange APIs.

    The source models intentionally keep credentials behind ``json:\"-\"``.  Keep
    these schemas explicit rather than reflecting Go structs so a future model
    change cannot accidentally publish Auth, Token, JWTToken, or lease tokens.
    """
    time = {"type": "string", "format": "date-time"}
    integer = {"type": "integer", "minimum": 0}
    positive = {"type": "integer", "minimum": 1}
    schemas: dict[str, dict[str, Any]] = {
        "AccountUser": {
            "type": "object",
            "required": ["id", "username", "email", "role", "created_at", "updated_at"],
            "properties": {
                "id": positive, "username": {"type": "string"}, "email": {"type": "string", "format": "email"},
                "role": {"type": "string", "enum": ["user", "admin"]}, "created_at": time, "updated_at": time,
            },
        },
        "Account": {
            "type": "object",
            "required": ["id", "user_id", "phone", "platform", "expire_at", "cloud_count", "remark", "is_active", "jwt_error_count", "created_at", "updated_at"],
            "properties": {
                "id": positive, "user_id": positive, "phone": {"type": "string"}, "platform": {"type": "string"},
                "expire_at": {"type": "integer"}, "cloud_count": {"type": "integer"}, "remark": {"type": "string"},
                "is_active": {"type": "boolean"}, "jwt_error_count": integer, "created_at": time, "updated_at": time,
                "user": {"$ref": "#/components/schemas/AccountUser"},
            },
        },
        "AccountList": {
            "type": "object", "required": ["accounts", "total", "page", "page_size"],
            "properties": {"accounts": {"type": "array", "items": {"$ref": "#/components/schemas/Account"}}, "total": integer, "page": positive, "page_size": positive},
        },
        "TaskLogAccount": {
            "type": "object", "required": ["id", "phone", "remark"],
            "properties": {"id": positive, "phone": {"type": "string"}, "remark": {"type": "string"}},
        },
        "TaskLog": {
            "type": "object", "required": ["id", "user_id", "account_id", "task_type", "status", "message", "cloud_gained", "execution_time", "created_at"],
            "properties": {
                "id": positive, "user_id": positive, "account_id": positive, "task_type": {"type": "string"}, "status": {"type": "string"},
                "message": {"type": "string"}, "cloud_gained": {"type": "integer"}, "execution_time": integer, "created_at": time,
                "account": {"$ref": "#/components/schemas/TaskLogAccount"},
            },
        },
        "TaskLogList": {
            "type": "object", "required": ["task_logs", "total", "page", "page_size"],
            "properties": {"task_logs": {"type": "array", "items": {"$ref": "#/components/schemas/TaskLog"}}, "total": integer, "page": positive, "page_size": positive},
        },
        "QueueBackendMeta": {
            "type": "object", "properties": {
                "backend": {"type": "string"}, "pending_key": {"type": "string"}, "processing_key": {"type": "string"},
                "delayed_key": {"type": "string"}, "dead_letter_key": {"type": "string"}, "stream_key": {"type": "string"},
                "consumer_group": {"type": "string"}, "consumer_name": {"type": "string"}, "max_len_approx": {"oneOf": [{"type": "integer"}, {"type": "string"}]},
                "labels": {"type": "object", "additionalProperties": {"type": "string"}},
            },
        },
        "QueueStatus": {
            "type": "object", "required": ["queue_length", "active_workers", "pending_tasks", "completed_tasks", "successful_tasks", "failed_tasks"],
            "properties": {
                "backend": {"type": "string"}, "backend_meta": {"$ref": "#/components/schemas/QueueBackendMeta"}, "is_healthy": {"type": "boolean"},
                "errors": {"type": "array", "items": {"type": "string"}}, "queue_length": integer, "processing_count": integer,
                "delayed_count": integer, "dead_letter_count": integer, "active_workers": integer, "pending_tasks": integer,
                "completed_tasks": integer, "successful_tasks": integer, "failed_tasks": integer,
            },
        },
        "DeadLetter": {
            "type": "object", "required": ["id", "reason", "failed_at", "malformed"],
            "properties": {
                "id": {"type": "string"}, "reason": {"type": "string"}, "failed_at": time, "malformed": {"type": "boolean"},
                "task": {"type": "object", "properties": {"account_id": positive, "user_id": positive, "task_type": {"type": "string"}, "operation_id": {"type": "string"}, "operation_type": {"type": "string"}, "created_at": {"type": "integer"}, "retry_count": integer}},
            },
        },
        "DeadLetterList": {
            "type": "object", "required": ["dead_letters", "total"],
            "properties": {"dead_letters": {"type": "array", "items": {"$ref": "#/components/schemas/DeadLetter"}}, "total": integer},
        },
        "Product": {
            "type": "object",
            "required": ["id", "prize_id", "prize_name", "p_order", "category", "daily_remainder_count", "daily_limit_count", "daily_count", "image_url", "stock_status", "memo", "is_active", "is_deleted", "created_at", "updated_at"],
            "properties": {
                "id": positive, "prize_id": {"type": "string"}, "prize_name": {"type": "string"}, "p_order": {"type": "integer"}, "category": {"type": "string"},
                "daily_remainder_count": {"type": "integer"}, "daily_limit_count": {"type": "integer"}, "daily_count": {"type": "integer"},
                "image_url": {"type": "string"}, "stock_status": {"type": "string"}, "last_stock_check": time, "memo": {"type": "string"},
                "is_active": {"type": "boolean"}, "is_deleted": {"type": "boolean"}, "created_at": time, "updated_at": time,
            },
        },
        "ProductList": {
            "type": "object", "required": ["products", "total"],
            "properties": {"products": {"type": "array", "items": {"$ref": "#/components/schemas/Product"}}, "total": integer},
        },
        "ExchangeRule": {
            "type": "object",
            "required": ["id", "user_id", "account_id", "phone", "remark", "exchange_time_1", "exchange_time_2", "is_active", "created_at", "updated_at"],
            "properties": {
                "id": positive, "user_id": positive, "account_id": positive, "phone": {"type": "string"}, "remark": {"type": "string"},
                "exchange_time_1": {"type": "string", "format": "time"}, "exchange_time_2": {"type": "string", "format": "time"},
                "is_active": {"type": "boolean"}, "last_exchange_at": time, "created_at": time, "updated_at": time,
                "current_product": {"$ref": "#/components/schemas/Product"}, "account": {"$ref": "#/components/schemas/Account"},
            },
        },
        "ExchangeRuleList": {
            "type": "object", "required": ["accounts", "rules", "total"],
            "properties": {
                "accounts": {"type": "array", "items": {"$ref": "#/components/schemas/ExchangeRule"}},
                "rules": {"type": "array", "items": {"$ref": "#/components/schemas/ExchangeRule"}}, "total": integer,
            },
        },
        "ExchangeTask": {
            "type": "object",
            "required": ["id", "user_id", "exchange_account_id", "product_id", "prize_id", "prize_name", "task_type", "max_attempts", "attempted_count", "status", "last_result", "priority", "task_group", "timeout_seconds", "max_retries", "retry_count", "success_count", "fail_count", "created_at", "updated_at"],
            "properties": {
                "id": positive, "user_id": positive, "exchange_account_id": positive, "product_id": positive, "prize_id": {"type": "string"}, "prize_name": {"type": "string"},
                "task_type": {"type": "string", "enum": ["fixed", "long_term"]}, "max_attempts": positive, "scheduled_exchange_time": {"type": "string", "format": "time"},
                "restock_cycle": {"type": "string", "enum": ["daily", "weekly", "monthly", "once"]}, "restock_weekday": {"type": ["integer", "null"], "minimum": 0, "maximum": 6},
                "restock_day_of_month": {"type": ["integer", "null"], "minimum": 1, "maximum": 31}, "restock_times": {"type": "string"}, "custom_cron": {"type": "string"},
                "calendar_policy": {"type": "string", "enum": ["all", "workday", "holiday"]}, "holiday_dates": {"type": "string"}, "workday_dates": {"type": "string"},
                "skip_reason": {"type": "string"}, "attempted_count": integer, "status": {"type": "string", "enum": ["pending", "running", "completed", "failed"]},
                "last_attempt_at": time, "last_result": {"type": "string"}, "next_run_at": time, "priority": {"type": "integer"}, "task_group": {"type": "string"},
                "timeout_seconds": positive, "max_retries": integer, "retry_count": integer, "last_retry_at": time, "success_count": integer, "fail_count": integer,
                "created_at": time, "updated_at": time, "exchange_account": {"$ref": "#/components/schemas/ExchangeRule"}, "product": {"$ref": "#/components/schemas/Product"},
            },
        },
        "ExchangeTaskList": {
            "type": "object", "required": ["tasks", "total"],
            "properties": {"tasks": {"type": "array", "items": {"$ref": "#/components/schemas/ExchangeTask"}}, "total": integer},
        },
        "ExchangeTaskCreateResult": {
            "type": "object", "required": ["target_type", "target_id", "success", "message"],
            "properties": {"target_type": {"type": "string", "enum": ["cloud_account", "exchange_rule"]}, "target_id": positive, "success": {"type": "boolean"}, "message": {"type": "string"}, "task": {"$ref": "#/components/schemas/ExchangeTask"}},
        },
        "ExchangeTaskCreateResponse": {
            "type": "object", "required": ["task", "tasks", "created", "errors", "results"],
            "properties": {"task": {"$ref": "#/components/schemas/ExchangeTask"}, "tasks": {"type": "array", "items": {"$ref": "#/components/schemas/ExchangeTask"}}, "created": integer, "errors": {"type": "array", "items": {"type": "string"}}, "results": {"type": "array", "items": {"$ref": "#/components/schemas/ExchangeTaskCreateResult"}}},
        },
        "ExchangeRecord": {
            "type": "object", "required": ["id", "user_id", "exchange_account_id", "product_id", "prize_id", "prize_name", "status", "message", "execution_time_ms", "created_at"],
            "properties": {
                "id": positive, "user_id": positive, "exchange_account_id": positive, "exchange_task_id": positive, "product_id": positive, "prize_id": {"type": "string"},
                "prize_name": {"type": "string"}, "status": {"type": "string", "enum": ["success", "failed"]}, "message": {"type": "string"}, "execution_time_ms": integer,
                "created_at": time, "exchange_account": {"$ref": "#/components/schemas/ExchangeRule"}, "product": {"$ref": "#/components/schemas/Product"},
            },
        },
        "RecordStats": {"type": "object", "required": ["success", "failed"], "properties": {"success": integer, "failed": integer}},
        "ExchangeRecordList": {
            "type": "object", "required": ["records", "total", "stats"],
            "properties": {"records": {"type": "array", "items": {"$ref": "#/components/schemas/ExchangeRecord"}}, "total": integer, "stats": {"$ref": "#/components/schemas/RecordStats"}},
        },
        "CreateAccountRequest": {"type": "object", "required": ["phone", "auth"], "properties": {"phone": {"type": "string"}, "auth": {"type": "string", "writeOnly": True}, "remark": {"type": "string"}}},
        "UpdateAccountRequest": {"type": "object", "required": ["phone"], "properties": {"phone": {"type": "string"}, "auth": {"type": "string", "writeOnly": True}, "remark": {"type": "string"}}},
        "SmsSendRequest": {"type": "object", "required": ["phone"], "properties": {"phone": {"type": "string", "pattern": "^1[0-9]{10}$"}}},
        "SmsSendResult": {"type": "object", "required": ["phone", "task_id"], "properties": {"phone": {"type": "string"}, "task_id": {"type": "string", "minLength": 1, "maxLength": 128}}},
        "SmsStatus": {"type": "object", "required": ["phone", "task_id", "status", "retryable", "message"], "properties": {"phone": {"type": "string"}, "task_id": {"type": "string", "minLength": 1, "maxLength": 128}, "status": {"type": "string", "enum": ["pending", "completed", "failed", "timeout"]}, "retryable": {"type": "boolean"}, "message": {"type": "string"}}},
        "SmsLoginRequest": {"type": "object", "required": ["phone", "sms_code", "task_id"], "properties": {"phone": {"type": "string", "pattern": "^1[0-9]{10}$"}, "sms_code": {"type": "string", "minLength": 1}, "task_id": {"type": "string", "minLength": 1, "maxLength": 128}, "remark": {"type": "string"}}},
        "TaskStatusItem": {"type": "object", "required": ["account_id", "task_type", "status", "progress", "message"], "properties": {"account_id": positive, "task_type": {"type": "string"}, "status": {"type": "string"}, "progress": {"type": "number", "minimum": 0, "maximum": 1}, "message": {"type": "string"}, "start_time": {"type": "string"}, "end_time": {"type": "string"}}},
        "TaskStatus": {"type": "object", "required": ["tasks"], "properties": {"tasks": {"type": "array", "items": {"$ref": "#/components/schemas/TaskStatusItem"}}}},
        "ProductCategoryList": {"type": "object", "required": ["categories"], "properties": {"categories": {"type": "array", "items": {"type": "string"}}}},
        "ExchangeRuleResponse": {"type": "object", "required": ["account", "rule"], "properties": {"account": {"$ref": "#/components/schemas/ExchangeRule"}, "rule": {"$ref": "#/components/schemas/ExchangeRule"}}},
        "ExchangeRecordExport": {"type": "object", "required": ["records", "total", "exported_at"], "properties": {"records": {"type": "array", "items": {"$ref": "#/components/schemas/ExchangeRecord"}}, "total": integer, "exported_at": {"type": "string"}}},
        "BatchExecuteExchangeTasksRequest": {"type": "object", "required": ["task_ids"], "properties": {"task_ids": {"type": "array", "items": positive, "minItems": 1, "maxItems": 50, "uniqueItems": True}}},
        "ImmediateExchangeRequest": {"type": "object", "required": ["product_id"], "properties": {"exchange_rule_id": {"type": ["integer", "null"], "minimum": 1}, "exchange_account_id": {"type": ["integer", "null"], "minimum": 1}, "account_id": {"type": ["integer", "null"], "minimum": 1}, "product_id": positive}},
        "AddExchangeRuleRequest": {"type": "object", "required": ["account_id"], "properties": {"account_id": positive, "product_id": positive, "remark": {"type": "string"}, "exchange_time_1": {"type": "string", "format": "time"}, "exchange_time_2": {"type": "string", "format": "time"}}},
        "UpdateExchangeRuleRequest": {"type": "object", "properties": {"remark": {"type": "string"}, "exchange_time_1": {"type": "string", "format": "time"}, "exchange_time_2": {"type": "string", "format": "time"}, "is_active": {"type": "boolean"}, "product_id": positive}},
        "CreateExchangeTaskRequest": {"type": "object", "required": ["product_id"], "properties": {"exchange_rule_id": positive, "exchange_rule_ids": {"type": "array", "items": positive, "maxItems": 100}, "exchange_account_id": positive, "exchange_account_ids": {"type": "array", "items": positive, "maxItems": 100}, "account_id": positive, "account_ids": {"type": "array", "items": positive, "maxItems": 100}, "product_id": positive, "task_type": {"type": "string", "enum": ["fixed", "long_term"]}, "max_attempts": positive, "scheduled_exchange_time": {"type": "string", "format": "time"}, "restock_cycle": {"type": "string", "enum": ["daily", "weekly", "monthly", "once"]}, "restock_weekday": {"type": ["integer", "null"], "minimum": 0, "maximum": 6}, "restock_day_of_month": {"type": ["integer", "null"], "minimum": 1, "maximum": 31}, "restock_times": {"oneOf": [{"type": "string"}, {"type": "array", "items": {"type": "string"}}]}, "custom_cron": {"type": "string"}, "calendar_policy": {"type": "string", "enum": ["all", "workday", "holiday"]}, "holiday_dates": {"oneOf": [{"type": "string"}, {"type": "array", "items": {"type": "string"}}]}, "workday_dates": {"oneOf": [{"type": "string"}, {"type": "array", "items": {"type": "string"}}]}}},
        "UpdateExchangeTaskRequest": {"type": "object", "required": ["max_attempts"], "properties": {"max_attempts": positive}},
        "UpdateProductsRequest": {"type": "object", "required": ["account_id"], "properties": {"account_id": positive}},
        # Authentication contracts deliberately contain only browser-safe metadata. Session and refresh credentials stay HttpOnly cookies.
        "AuthUser": {"type": "object", "required": ["id", "username", "email", "role"], "properties": {"id": positive, "username": {"type": "string"}, "email": {"type": "string", "format": "email"}, "role": {"type": "string", "enum": ["user", "admin"]}}},
        "AuthResponse": {"type": "object", "required": ["expires_at", "refresh_expires_at", "user"], "properties": {"expires_at": {"type": "integer"}, "refresh_expires_at": {"type": "integer"}, "user": {"$ref": "#/components/schemas/AuthUser"}}},
        "RefreshTokenResponse": {"type": "object", "required": ["expires_at", "refresh_expires_at"], "properties": {"expires_at": {"type": "integer"}, "refresh_expires_at": {"type": "integer"}}},
        "RegisterRequest": {"type": "object", "required": ["username", "password"], "properties": {"username": {"type": "string", "minLength": 3, "maxLength": 50}, "password": {"type": "string", "minLength": 6, "writeOnly": True}, "email": {"type": "string", "format": "email"}}},
        "LoginRequest": {"type": "object", "required": ["username", "password"], "properties": {"username": {"type": "string"}, "password": {"type": "string", "writeOnly": True}}},
        "RefreshTokenRequest": {"type": "object", "properties": {"refresh_token": {"type": "string", "writeOnly": True}}},
        "SendPasswordResetCodeRequest": {"type": "object", "required": ["username", "email"], "properties": {"username": {"type": "string", "minLength": 3, "maxLength": 50}, "email": {"type": "string", "format": "email"}}},
        "ResetPasswordRequest": {"type": "object", "required": ["username", "email", "code", "new_password"], "properties": {"username": {"type": "string", "minLength": 3, "maxLength": 50}, "email": {"type": "string", "format": "email"}, "code": {"type": "string", "minLength": 6, "maxLength": 6}, "new_password": {"type": "string", "minLength": 6, "writeOnly": True}}},
        "DashboardTrendPoint": {"type": "object", "required": ["date", "cloud_count", "cloud_diff", "has_data"], "properties": {"date": {"type": "string"}, "cloud_count": {"type": "integer"}, "cloud_diff": {"type": "integer"}, "has_data": {"type": "boolean"}}},
        "DashboardAccountRank": {"type": "object", "required": ["account_id", "phone", "remark", "cloud_count"], "properties": {"account_id": positive, "phone": {"type": "string"}, "remark": {"type": "string"}, "cloud_count": {"type": "integer"}}},
        "DashboardData": {"type": "object", "required": ["total_cloud", "account_count", "today_gained", "yesterday_diff", "week_diff", "success_rate", "trend_data", "account_ranking"], "properties": {"total_cloud": {"type": "integer"}, "account_count": {"type": "integer"}, "today_gained": {"type": "integer"}, "yesterday_diff": {"type": "integer"}, "week_diff": {"type": "integer"}, "success_rate": {"type": "number"}, "trend_data": {"type": "array", "items": {"$ref": "#/components/schemas/DashboardTrendPoint"}}, "account_ranking": {"type": "array", "items": {"$ref": "#/components/schemas/DashboardAccountRank"}}}},
        "CloudStat": {"type": "object", "required": ["id", "user_id", "account_id", "date", "cloud_count", "cloud_diff", "cloud_diff_week", "created_at", "updated_at"], "properties": {"id": positive, "user_id": positive, "account_id": positive, "date": {"type": "string", "format": "date"}, "cloud_count": {"type": "integer"}, "cloud_diff": {"type": "integer"}, "cloud_diff_week": {"type": "integer"}, "created_at": time, "updated_at": time, "account": {"$ref": "#/components/schemas/Account"}}},
        "CloudStatList": {"type": "object", "required": ["cloud_stats", "total", "page", "page_size"], "properties": {"cloud_stats": {"type": "array", "items": {"$ref": "#/components/schemas/CloudStat"}}, "total": integer, "page": positive, "page_size": positive}},
        "TrendData": {"type": "object", "required": ["trend_data"], "properties": {"trend_data": {"type": "array", "items": {"$ref": "#/components/schemas/DashboardTrendPoint"}}}},
        "TotalCloudCount": {"type": "object", "required": ["total_cloud"], "properties": {"total_cloud": {"type": "integer"}}},
        "AdminUser": {"type": "object", "required": ["id", "username", "email", "role", "created_at"], "properties": {"id": positive, "username": {"type": "string"}, "email": {"type": "string", "format": "email"}, "role": {"type": "string", "enum": ["user", "admin"]}, "created_at": {"type": "string"}}},
        "AdminUserList": {"type": "object", "required": ["users", "total", "page", "size"], "properties": {"users": {"type": "array", "items": {"$ref": "#/components/schemas/AdminUser"}}, "total": integer, "page": positive, "size": positive}},
        "AdminAccountSearch": {"type": "object", "required": ["id", "phone", "remark", "user_id", "username", "is_active"], "properties": {"id": positive, "phone": {"type": "string"}, "remark": {"type": "string"}, "user_id": positive, "username": {"type": "string"}, "is_active": {"type": "boolean"}}},
        "AdminAccountSearchList": {"type": "object", "required": ["accounts"], "properties": {"accounts": {"type": "array", "items": {"$ref": "#/components/schemas/AdminAccountSearch"}}}},
        "AdminAccountSummary": {"type": "object", "required": ["id", "phone", "remark", "owner_username", "cloud_count", "is_active", "created_at", "today_gained", "yesterday_gained", "success_count", "failed_count", "last_executed_at"], "properties": {"id": positive, "phone": {"type": "string"}, "remark": {"type": "string"}, "owner_username": {"type": "string"}, "cloud_count": {"type": "integer"}, "is_active": {"type": "boolean"}, "created_at": {"type": "string"}, "today_gained": {"type": "integer"}, "yesterday_gained": {"type": "integer"}, "success_count": integer, "failed_count": integer, "last_executed_at": {"type": "string"}}},
        "AdminAccountSummaryList": {"type": "object", "required": ["summaries", "total", "page", "page_size"], "properties": {"summaries": {"type": "array", "items": {"$ref": "#/components/schemas/AdminAccountSummary"}}, "total": integer, "page": positive, "page_size": positive}},
        "AdminAccountRank": {"type": "object", "required": ["account_id", "phone", "remark", "owner_username", "cloud_count", "today_gained"], "properties": {"account_id": positive, "phone": {"type": "string"}, "remark": {"type": "string"}, "owner_username": {"type": "string"}, "cloud_count": {"type": "integer"}, "today_gained": {"type": "integer"}}},
        "AdminDashboardData": {"type": "object", "required": ["total_cloud", "account_count", "user_count", "today_gained", "yesterday_gained", "success_rate", "account_ranking"], "properties": {"total_cloud": {"type": "integer"}, "account_count": integer, "user_count": integer, "today_gained": {"type": "integer"}, "yesterday_gained": {"type": "integer"}, "success_rate": {"type": "number"}, "account_ranking": {"type": "array", "items": {"$ref": "#/components/schemas/AdminAccountRank"}}}},
        "AdminStatsOverview": {"type": "object", "required": ["user_count", "account_count", "total_cloud", "active_tasks"], "properties": {"user_count": integer, "account_count": integer, "total_cloud": {"type": "integer"}, "active_tasks": {"type": "integer"}}},
        "TaskConfig": {"type": "object", "required": ["id", "task_type", "task_name", "description", "is_enabled", "sort_order", "run_in_batch", "updated_at"], "properties": {"id": positive, "task_type": {"type": "string"}, "task_name": {"type": "string"}, "description": {"type": "string"}, "is_enabled": {"type": "boolean"}, "sort_order": {"type": "integer"}, "run_in_batch": {"type": "boolean"}, "updated_at": time}},
        "TaskConfigList": {"type": "object", "required": ["configs"], "properties": {"configs": {"type": "array", "items": {"$ref": "#/components/schemas/TaskConfig"}}}},
        "UpdateUserRoleRequest": {"type": "object", "required": ["role"], "properties": {"role": {"type": "string", "enum": ["user", "admin"]}}},
        "ResetUserPasswordRequest": {"type": "object", "required": ["password"], "properties": {"password": {"type": "string", "minLength": 6, "writeOnly": True}}},
        "UpdateAccountStatusRequest": {"type": "object", "required": ["is_active"], "properties": {"is_active": {"type": "boolean"}}},
        "UpdateTaskConfigRequest": {"type": "object", "required": ["is_enabled"], "properties": {"is_enabled": {"type": "boolean"}}},
        "ExchangeConfig": {"type": "object", "required": ["auto_update_products", "concurrency", "enabled", "exchange_monthly_enabled", "exchange_time", "monthly_prize_id", "immediate_exchange_enabled"], "properties": {"auto_update_products": {"type": "boolean"}, "concurrency": {"type": "integer", "minimum": 1, "maximum": 1000}, "enabled": {"type": "boolean"}, "exchange_monthly_enabled": {"type": "boolean"}, "exchange_time": {"type": "string", "format": "time"}, "monthly_prize_id": {"type": "string", "maxLength": 100}, "immediate_exchange_enabled": {"type": "boolean"}}},
        "ExchangeConfigPublic": {"type": "object", "required": ["enabled", "immediate_exchange_enabled"], "properties": {"enabled": {"type": "boolean"}, "immediate_exchange_enabled": {"type": "boolean"}}},
        "UpdateExchangeConfigRequest": {"type": "object", "required": ["auto_update_products", "concurrency", "enabled", "exchange_monthly_enabled", "exchange_time", "monthly_prize_id", "immediate_exchange_enabled"], "properties": {"auto_update_products": {"type": "boolean"}, "concurrency": {"type": "integer", "minimum": 1, "maximum": 1000}, "enabled": {"type": "boolean"}, "exchange_monthly_enabled": {"type": "boolean"}, "exchange_time": {"type": "string", "format": "time"}, "monthly_prize_id": {"type": "string", "maxLength": 100}, "immediate_exchange_enabled": {"type": "boolean"}}},
        "Announcement": {"type": "object", "required": ["id", "title", "content", "is_popup", "is_top", "is_published", "popup_count", "created_at", "updated_at"], "properties": {"id": positive, "title": {"type": "string", "maxLength": 200}, "content": {"type": "string"}, "is_popup": {"type": "boolean"}, "is_top": {"type": "boolean"}, "is_published": {"type": "boolean"}, "popup_count": integer, "created_at": time, "updated_at": time}},
        "AnnouncementList": {"type": "object", "required": ["announcements", "total"], "properties": {"announcements": {"type": "array", "items": {"$ref": "#/components/schemas/Announcement"}}, "total": integer}},
        "AnnouncementResponse": {"type": "object", "required": ["announcement"], "properties": {"announcement": {"$ref": "#/components/schemas/Announcement"}}},
        "AnnouncementPopupList": {"type": "object", "required": ["has_popup", "announcements"], "properties": {"has_popup": {"type": "boolean"}, "announcements": {"type": "array", "items": {"$ref": "#/components/schemas/Announcement"}}}},
        "CreateAnnouncementRequest": {"type": "object", "required": ["title", "content"], "properties": {"title": {"type": "string", "minLength": 1, "maxLength": 200}, "content": {"type": "string", "minLength": 1}, "is_popup": {"type": "boolean"}, "is_top": {"type": "boolean"}}},
        "UpdateAnnouncementRequest": {"type": "object", "required": ["title", "content", "is_popup", "is_top", "is_published"], "properties": {"title": {"type": "string", "minLength": 1, "maxLength": 200}, "content": {"type": "string", "minLength": 1}, "is_popup": {"type": "boolean"}, "is_top": {"type": "boolean"}, "is_published": {"type": "boolean"}}},
        "ReplayDeadLetterRequest": {"type": "object", "required": ["approval", "reason"], "properties": {"approval": {"type": "string", "const": "approved"}, "reason": {"type": "string", "minLength": 8, "maxLength": 500}}},
    }
    return schemas


def typed_success_response(schema_name: str, description: str = "Successful response") -> dict[str, Any]:
    envelope_name = "Envelope" if schema_name == "Envelope" else f"{schema_name}Envelope"
    return {
        "description": description,
        "content": {"application/json": {"schema": {"$ref": f"#/components/schemas/{envelope_name}"}}},
    }


def request_body(schema_name: str) -> dict[str, Any]:
    return {
        "required": True,
        "content": {"application/json": {"schema": {"$ref": f"#/components/schemas/{schema_name}"}}},
    }


def apply_typed_resource_contract(paths: dict[str, dict[str, dict[str, Any]]]) -> None:
    """Overlay public request/response DTOs on the versioned canonical routes."""
    bindings = {
        ("/api/v1/accounts", "get"): ("AccountList", None),
        ("/api/v1/accounts", "post"): ("Account", "CreateAccountRequest"),
        ("/api/v1/accounts/{id}", "get"): ("Account", None),
        ("/api/v1/accounts/{id}", "put"): ("Account", "UpdateAccountRequest"),
        ("/api/v1/accounts/{id}", "delete"): ("Envelope", None),
        ("/api/v1/accounts/{id}/status", "put"): ("Envelope", None, [query_parameter("is_active", {"type": "boolean"}, "Whether the account is active.", required=True)]),
        ("/api/v1/accounts/{id}/refresh", "post"): ("Account", None),
        ("/api/v1/accounts/sms/send", "post"): ("SmsSendResult", "SmsSendRequest"),
        ("/api/v1/accounts/sms/status/{task_id}", "get"): ("SmsStatus", None),
        ("/api/v1/accounts/sms/verify", "post"): ("Account", "SmsLoginRequest"),
        ("/api/v1/tasks/logs", "get"): ("TaskLogList", None),
        ("/api/v1/tasks/status", "get"): ("TaskStatus", None, [query_parameter("account_id", {"type": "integer", "minimum": 1}, "Optionally filter recent task status by account.")]),
        ("/api/v1/tasks/queue-status", "get"): ("QueueStatus", None),
        ("/api/v1/admin/tasks/dead-letters", "get"): ("DeadLetterList", None),
        ("/api/v1/admin/tasks/dead-letters/{id}/replay", "post"): ("Envelope", "ReplayDeadLetterRequest"),
        ("/api/v1/exchange/rules", "get"): ("ExchangeRuleList", None),
        ("/api/v1/exchange/rules", "post"): ("ExchangeRuleResponse", "AddExchangeRuleRequest"),
        ("/api/v1/exchange/rules/{id}", "put"): ("Envelope", "UpdateExchangeRuleRequest"),
        ("/api/v1/exchange/rules/{id}", "delete"): ("Envelope", None),
        ("/api/v1/exchange/accounts", "get"): ("ExchangeRuleList", None),
        ("/api/v1/exchange/accounts", "post"): ("ExchangeRuleResponse", "AddExchangeRuleRequest"),
        ("/api/v1/exchange/accounts/{id}", "put"): ("Envelope", "UpdateExchangeRuleRequest"),
        ("/api/v1/exchange/accounts/{id}", "delete"): ("Envelope", None),
        ("/api/v1/exchange/tasks", "get"): ("ExchangeTaskList", None),
        ("/api/v1/exchange/tasks", "post"): ("ExchangeTaskCreateResponse", "CreateExchangeTaskRequest"),
        ("/api/v1/exchange/tasks/{id}", "put"): ("Envelope", "UpdateExchangeTaskRequest"),
        ("/api/v1/exchange/tasks/{id}", "delete"): ("Envelope", None),
        # Responses for these asynchronous commands are supplied by apply_typed_operation_contract; this overlay adds only request DTOs.
        ("/api/v1/exchange/tasks/batch-execute", "post"): (None, "BatchExecuteExchangeTasksRequest"),
        ("/api/v1/exchange/immediate", "post"): (None, "ImmediateExchangeRequest"),
        ("/api/v1/exchange/records", "get"): ("ExchangeRecordList", None),
        ("/api/v1/products/search", "get"): ("ProductList", None),
        ("/api/v1/products/categories", "get"): ("ProductCategoryList", None),
        ("/api/v1/products/update", "post"): ("Envelope", "UpdateProductsRequest"),
        ("/api/v1/auth/login", "post"): ("AuthResponse", "LoginRequest"),
        ("/api/v1/auth/refresh", "post"): ("RefreshTokenResponse", "RefreshTokenRequest"),
        ("/api/v1/auth/password/reset-code/send", "post"): ("Envelope", "SendPasswordResetCodeRequest"),
        ("/api/v1/auth/password/reset", "post"): ("Envelope", "ResetPasswordRequest"),
        ("/api/v1/auth/me", "get"): ("AuthUser", None),
        ("/api/v1/auth/logout", "post"): ("Envelope", None),
        ("/api/v1/auth/logout-all", "post"): ("Envelope", None),
        ("/api/v1/stats/dashboard", "get"): ("DashboardData", None),
        ("/api/v1/stats/cloud", "get"): ("CloudStatList", None),
        ("/api/v1/stats/trend", "get"): ("TrendData", None),
        ("/api/v1/stats/calculate", "post"): ("Envelope", None),
        ("/api/v1/stats/total-cloud", "get"): ("TotalCloudCount", None),
        ("/api/v1/admin/users", "get"): ("AdminUserList", None),
        ("/api/v1/admin/accounts", "get"): ("AccountList", None),
        ("/api/v1/admin/accounts/search", "get"): ("AdminAccountSearchList", None),
        ("/api/v1/admin/accounts/summaries", "get"): ("AdminAccountSummaryList", None),
        ("/api/v1/admin/dashboard", "get"): ("AdminDashboardData", None),
        ("/api/v1/admin/stats/overview", "get"): ("AdminStatsOverview", None),
        ("/api/v1/admin/task-configs", "get"): ("TaskConfigList", None),
        ("/api/v1/admin/tasks/queue-status", "get"): ("QueueStatus", None),
        ("/api/v1/admin/users/{id}/role", "put"): ("Envelope", "UpdateUserRoleRequest"),
        ("/api/v1/admin/users/{id}/password", "put"): ("Envelope", "ResetUserPasswordRequest"),
        ("/api/v1/admin/users/{id}", "delete"): ("Envelope", None),
        ("/api/v1/admin/accounts/{id}/status", "put"): ("Envelope", "UpdateAccountStatusRequest"),
        ("/api/v1/admin/accounts/{id}", "delete"): ("Envelope", None),
        ("/api/v1/admin/task-configs/{task_type}", "put"): ("Envelope", "UpdateTaskConfigRequest"),
        ("/api/v1/exchange/config", "get"): ("ExchangeConfigPublic", None),
        ("/api/v1/announcements", "get"): ("AnnouncementList", None),
        ("/api/v1/announcements/popup", "get"): ("AnnouncementPopupList", None),
        ("/api/v1/admin/exchange/config", "get"): ("ExchangeConfig", None),
        ("/api/v1/admin/exchange/config", "put"): ("Envelope", "UpdateExchangeConfigRequest"),
        ("/api/v1/admin/exchange/rules", "get"): ("ExchangeRuleList", None),
        ("/api/v1/admin/exchange/rules/{id}", "put"): ("Envelope", "UpdateExchangeRuleRequest"),
        ("/api/v1/admin/exchange/accounts", "get"): ("ExchangeRuleList", None),
        ("/api/v1/admin/exchange/accounts/{id}", "put"): ("Envelope", "UpdateExchangeRuleRequest"),
        ("/api/v1/admin/exchange/tasks", "get"): ("ExchangeTaskList", None),
        ("/api/v1/admin/announcements", "get"): ("AnnouncementList", None),
        ("/api/v1/admin/announcements", "post"): ("AnnouncementResponse", "CreateAnnouncementRequest"),
        ("/api/v1/admin/announcements/{id}", "get"): ("AnnouncementResponse", None),
        ("/api/v1/admin/announcements/{id}", "put"): ("AnnouncementResponse", "UpdateAnnouncementRequest"),
        ("/api/v1/admin/announcements/{id}", "delete"): ("Envelope", None),
    }
    register_operation = paths.get("/api/v1/auth/register", {}).get("post")
    if register_operation is not None:
        register_operation["operationId"] = "post_auth_register"
        register_operation["responses"] = {
            "201": typed_success_response("AuthResponse", "Authenticated session created"),
            "400": {"$ref": "#/components/responses/BadRequest"},
            "409": {"$ref": "#/components/responses/Conflict"},
            "500": {"$ref": "#/components/responses/InternalError"},
        }
        register_operation["requestBody"] = request_body("RegisterRequest")

    for (path, method), binding in bindings.items():
        response_schema, request_schema, *binding_options = binding
        operation = paths.get(path, {}).get(method)
        if operation is None:
            continue
        # Async command routes keep their submit_* operation ID and 202 lifecycle response.
        if path not in ASYNC_OPERATION_PATHS:
            operation["operationId"] = f"{method}_{path.removeprefix('/api/v1/').replace('/', '_').replace('{', '').replace('}', '')}"
        if response_schema is not None:
            operation["responses"] = {
                "200": typed_success_response(response_schema),
                "400": {"$ref": "#/components/responses/BadRequest"},
                "401": {"$ref": "#/components/responses/Unauthorized"},
                "403": {"$ref": "#/components/responses/Forbidden"},
                "404": {"$ref": "#/components/responses/NotFound"},
                "429": {"$ref": "#/components/responses/TooManyRequests"},
                "500": {"$ref": "#/components/responses/InternalError"},
            }
        if request_schema:
            operation["requestBody"] = request_body(request_schema)
        if binding_options:
            existing = {(item.get("in"), item.get("name")) for item in operation.get("parameters", [])}
            operation["parameters"] = [
                *operation.get("parameters", []),
                *(parameter for parameter in binding_options[0] if (parameter["in"], parameter["name"]) not in existing),
            ]

    export_records = paths.get("/api/v1/exchange/records/export", {}).get("get")
    if export_records is not None:
        export_records["operationId"] = "get_exchange_records_export"
        export_records["parameters"] = [
            *export_records.get("parameters", []),
            query_parameter("account_id", {"type": "integer", "minimum": 1}, "Filter by account."),
            query_parameter("product_name", {"type": "string"}, "Filter by product name."),
            query_parameter("status", {"type": "string", "enum": ["success", "failed"]}, "Filter by terminal result."),
            query_parameter("start_date", {"type": "string", "format": "date"}, "Inclusive start date."),
            query_parameter("end_date", {"type": "string", "format": "date"}, "Inclusive end date."),
            query_parameter("format", {"type": "string", "enum": ["csv", "json"], "default": "csv"}, "Export format."),
        ]
        export_records["responses"] = {
            "200": {
                "description": "CSV attachment by default, or a JSON export when format=json.",
                "content": {
                    "text/csv": {"schema": {"type": "string", "format": "binary"}},
                    "application/json": {"schema": {"$ref": "#/components/schemas/ExchangeRecordExport"}},
                },
            },
            "401": {"$ref": "#/components/responses/Unauthorized"},
            "500": {"$ref": "#/components/responses/InternalError"},
        }


def resource_envelope_schemas(schemas: dict[str, dict[str, Any]]) -> dict[str, dict[str, Any]]:
    """Build response envelopes for typed public DTOs."""
    envelopes: dict[str, dict[str, Any]] = {}
    for name in schemas:
        if name.endswith("Request") or name == "Envelope":
            continue
        envelopes[f"{name}Envelope"] = {
            "type": "object",
            "required": ["code", "message", "data"],
            "properties": {
                "code": {"type": "integer"},
                "business_code": {"type": "string"},
                "message": {"type": "string"},
                "trace_id": {"type": "string"},
                "data": {"$ref": f"#/components/schemas/{name}"},
            },
        }
    return envelopes


def apply_typed_operation_contract(paths: dict[str, dict[str, dict[str, Any]]]) -> None:
    """Add stable command lifecycle contracts to registered runtime routes."""
    for path in ASYNC_OPERATION_PATHS:
        operation = paths.get(path, {}).get("post")
        if operation is None:
            continue
        operation["operationId"] = "submit_" + re.sub(r"[^a-z0-9]+", "_", path.removeprefix("/api/v1/").lower()).strip("_")
        operation["responses"] = typed_operation_responses()
        operation["parameters"] = [*operation.get("parameters", []), idempotency_parameter()]
        operation["x-operation-lifecycle"] = "queued -> running -> succeeded|failed|canceled"

    status_operations = paths.get(OPERATION_STATUS_PATH, {})
    get_operation = status_operations.get("get")
    if get_operation is not None:
        get_operation["operationId"] = "get_operation"
        get_operation["responses"] = {
            "200": {"$ref": "#/components/responses/OperationResult"},
            "401": {"$ref": "#/components/responses/Unauthorized"},
            "404": {"$ref": "#/components/responses/NotFound"},
            "500": {"$ref": "#/components/responses/InternalError"},
        }
    cancel_operation = paths.get(OPERATION_CANCEL_PATH, {}).get("post")
    if cancel_operation is not None:
        cancel_operation["operationId"] = "cancel_operation"
        cancel_operation["responses"] = {
            "200": {"$ref": "#/components/responses/OperationResult"},
            "401": {"$ref": "#/components/responses/Unauthorized"},
            "404": {"$ref": "#/components/responses/NotFound"},
            "409": {"$ref": "#/components/responses/Conflict"},
            "429": {"$ref": "#/components/responses/TooManyRequests"},
            "500": {"$ref": "#/components/responses/InternalError"},
        }


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
    apply_typed_operation_contract(paths)
    apply_typed_resource_contract(paths)
    typed_resources = resource_schemas()
    typed_resource_envelopes = resource_envelope_schemas(typed_resources)
    doc = {
        "openapi": "3.1.0",
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
                "CookieAuth": {"type": "apiKey", "in": "cookie", "name": "auth_token", "description": "HttpOnly session JWT"},
                "CSRFToken": {"type": "apiKey", "in": "header", "name": "X-CSRF-Token", "description": "Required for cookie-authenticated unsafe methods"},
            },
            "schemas": {
                "Envelope": {
                    "type": "object",
                    "required": ["code", "message"],
                    "properties": {
                        "code": {"type": "integer", "description": "0 for success; otherwise HTTP-aligned application code"},
                        "business_code": {"type": "string"},
                        "message": {"type": "string"},
                        "data": {},
                        "trace_id": {"type": "string", "description": "Request/trace correlation identifier"},
                    },
                },
                "OperationAccepted": {
                    "type": "object",
                    "required": ["code", "message", "data"],
                    "properties": {
                        "code": {"type": "integer", "const": 0},
                        "business_code": {"type": "string"},
                        "message": {"type": "string"},
                        "trace_id": {"type": "string"},
                        "data": {"$ref": "#/components/schemas/Operation"},
                    },
                    "description": "Accepted asynchronous command. Idempotency replay returns the same full Operation state.",
                },
                "Operation": {
                    "type": "object",
                    "required": ["operation_id", "type", "status", "attempt_count", "queued_at", "created_at", "updated_at"],
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
                        "created_at": {"type": "string", "format": "date-time"},
                        "updated_at": {"type": "string", "format": "date-time"},
                    },
                },
                "OperationEnvelope": {
                    "type": "object",
                    "required": ["code", "message", "data"],
                    "properties": {
                        "code": {"type": "integer"},
                        "business_code": {"type": "string"},
                        "message": {"type": "string"},
                        "trace_id": {"type": "string"},
                        "data": {"$ref": "#/components/schemas/Operation"},
                    },
                },
                **typed_resources,
                **typed_resource_envelopes,
            },
            "responses": {
                "Success": {"description": "Successful response", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Envelope"}}}},
                "Accepted": {"description": "Asynchronous command accepted", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/OperationAccepted"}}}},
                "BadRequest": {"description": "Invalid request", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Envelope"}}}},
                "Unauthorized": {"description": "Authentication required", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Envelope"}}}},
                "Forbidden": {"description": "Permission or CSRF validation failed", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Envelope"}}}},
                "Conflict": {"description": "Idempotency or resource conflict", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Envelope"}}}},
                "TooManyRequests": {"description": "Rate limit exceeded", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Envelope"}}}},
                "NotFound": {"description": "Resource not found", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Envelope"}}}},
                "OperationResult": {"description": "Current asynchronous command state", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/OperationEnvelope"}}}},
                "InternalError": {"description": "Unexpected server error", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Envelope"}}}},
            },
        },
        "paths": dict(sorted(paths.items())),
    }
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(doc, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"OpenAPI contract written to {out.relative_to(PROJECT_ROOT)} ({len(paths)} paths)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
