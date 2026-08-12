#!/usr/bin/env python3
"""Append one Prometheus SLO snapshot to the ignored quarterly recovery report."""
from __future__ import annotations

import argparse
from datetime import datetime, timezone
import json
from pathlib import Path


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--snapshot", required=True, type=Path, help="JSON written by collect-slo-snapshot.sh")
    parser.add_argument("--report", required=True, type=Path, help="ignored local Markdown report path")
    parser.add_argument("--environment", default="unknown")
    parser.add_argument("--release-sha", default="unknown")
    parser.add_argument("--drill", default="snapshot")
    parser.add_argument("--rto-seconds", default="", help="measured worker/API recovery time; blank means pending")
    return parser.parse_args()


def number(value: object) -> float | None:
    if value is None:
        return None
    if isinstance(value, bool):
        raise ValueError("boolean is not a metric")
    return float(value)


def show(value: float | None, suffix: str = "") -> str:
    return "待采集" if value is None else f"{value:.6g}{suffix}"


def row(name: str, value: float | None, threshold: float | None, suffix: str = "") -> str:
    if value is None:
        result = "待采集"
    elif threshold is None:
        result = "已记录"
    else:
        result = "PASS" if value <= threshold else "FAIL"
    return f"| {name} | {show(value, suffix)} | {show(threshold, suffix) if threshold is not None else '人工目标'} | {result} |"


def main() -> int:
    args = parse_args()
    snapshot = json.loads(args.snapshot.read_text(encoding="utf-8"))
    values = snapshot.get("values", {})
    thresholds = snapshot.get("thresholds", {})
    try:
        api_p95 = number(values.get("api_p95_seconds"))
        queue_lag = number(values.get("queue_pending"))
        heartbeat = number(values.get("worker_heartbeat_age_seconds"))
        failure_rate = number(values.get("operation_failure_ratio"))
        rto = number(args.rto_seconds) if args.rto_seconds.strip() else None
        api_threshold = number(thresholds.get("api_p95_seconds"))
        queue_threshold = number(thresholds.get("queue_pending"))
        heartbeat_threshold = number(thresholds.get("worker_heartbeat_age_seconds"))
        failure_threshold = number(thresholds.get("operation_failure_ratio"))
    except (TypeError, ValueError) as exc:
        raise SystemExit(f"invalid SLO snapshot: {exc}") from exc

    collected_at = snapshot.get("collected_at") or datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    args.report.parent.mkdir(parents=True, exist_ok=True)
    lines = [
        "",
        f"## 自动采集 SLO 快照 — {collected_at}",
        "",
        f"- 环境：`{args.environment}`；Release SHA-256：`{args.release_sha}`；演练：`{args.drill}`；观察窗口：`{snapshot.get('window', 'unknown')}`。",
        f"- 结构化证据：`{args.snapshot}`；该报告和证据均保留在本地忽略路径。",
        "",
        "| 指标 | 采集值 | 目标 | 结论 |",
        "|---|---:|---:|---|",
        row("API P95", api_p95, api_threshold, "s"),
        row("队列 lag（pending）", queue_lag, queue_threshold),
        row("Worker 心跳年龄", heartbeat, heartbeat_threshold, "s"),
        row("Operation 失败率", failure_rate, failure_threshold),
        row("恢复时间 RTO", rto, None, "s"),
        "",
    ]
    with args.report.open("a", encoding="utf-8") as handle:
        handle.write("\n".join(lines))
    print(f"local quarterly report updated: {args.report}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
