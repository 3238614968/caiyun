#!/usr/bin/env python3
"""Verify hand-written API clients consume the reviewed generated DTO contract."""
from __future__ import annotations

from pathlib import Path
import re

ROOT = Path(__file__).resolve().parent.parent
GENERATED = ROOT / "frontend" / "src" / "api" / "generated" / "operation-contract.ts"
CLIENTS: dict[str, tuple[str, ...]] = {
    "frontend/src/api/account.ts": (
        "AdminUser", "AdminUserList", "AdminAccountSearch", "AdminAccountSearchList",
        "AdminStatsOverview", "AdminAccountSummary", "AdminAccountSummaryList",
        "AdminDashboardData", "AdminAccountRank", "TaskConfig", "TaskConfigList",
        "UpdateUserRoleRequest", "ResetUserPasswordRequest", "UpdateAccountStatusRequest",
        "UpdateTaskConfigRequest", "SmsSendRequest", "SmsSendResult", "SmsStatus", "SmsLoginRequest",
    ),
    "frontend/src/api/task.ts": (
        "CloudStat", "CloudStatList", "DashboardData", "DashboardTrendPoint",
        "DashboardAccountRank", "TrendData", "TotalCloudCount", "TaskStatus", "TaskStatusItem",
    ),
    "frontend/src/api/auth.ts": (
        "AuthUser", "AuthResponse", "RefreshTokenResponse", "RegisterRequest", "LoginRequest",
        "SendPasswordResetCodeRequest", "ResetPasswordRequest",
    ),
    "frontend/src/api/announcement.ts": (
        "Announcement", "AnnouncementList", "AnnouncementResponse", "AnnouncementPopupList",
        "CreateAnnouncementRequest", "UpdateAnnouncementRequest",
    ),
    "frontend/src/api/exchange/types.ts": (
        "ExchangeConfig", "ExchangeConfigPublic", "UpdateExchangeConfigRequest", "ProductCategoryList",
        "ExchangeRuleResponse", "BatchExecuteExchangeTasksRequest", "ImmediateExchangeRequest",
    ),
}


def require(condition: bool, message: str) -> None:
    if not condition:
        raise SystemExit(message)


def main() -> int:
    generated = GENERATED.read_text(encoding="utf-8")
    for client_name, types in CLIENTS.items():
        path = ROOT / client_name
        source = path.read_text(encoding="utf-8")
        for name in types:
            require(f"export interface {name} {{" in generated, f"generated contract lacks {name}")
            alias = re.compile(rf"\b{name}\s+as\s+{re.escape(name)}Contract\b")
            require(alias.search(source) is not None, f"{client_name} must import generated {name} as {name}Contract")
            require(f"export interface {name} {{" not in source, f"{client_name} redeclares generated {name}")

    legacy_path = re.compile(r"/api/(?!v1/)")
    legacy_escaped_path = re.compile(r"\\/api\\/(?!v1\\/)")
    for directory in (ROOT / "frontend" / "src" / "api", ROOT / "frontend" / "e2e"):
        for path in directory.rglob("*.ts"):
            if path == GENERATED:
                continue
            source = path.read_text(encoding="utf-8")
            require(legacy_path.search(source) is None and legacy_escaped_path.search(source) is None, f"{path.relative_to(ROOT)} still calls a deprecated /api path")
    print("Frontend generated DTO usage and /api/v1 migration valid")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
