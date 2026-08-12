#!/usr/bin/env bash
# Capture the release and quarterly-drill SLO evidence from Prometheus. Output
# defaults to .local so operational evidence stays out of source control.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PROMETHEUS_URL="${PROMETHEUS_URL:-}"
PROMETHEUS_BEARER_TOKEN="${PROMETHEUS_BEARER_TOKEN:-}"
SLO_WINDOW="${SLO_WINDOW:-15m}"
SLO_API_P95_SECONDS="${SLO_API_P95_SECONDS:-0.3}"
SLO_OPERATION_FAILURE_RATIO="${SLO_OPERATION_FAILURE_RATIO:-0.01}"
SLO_QUEUE_PENDING_MAX="${SLO_QUEUE_PENDING_MAX:-100}"
SLO_WORKER_HEARTBEAT_MAX_AGE_SECONDS="${SLO_WORKER_HEARTBEAT_MAX_AGE_SECONDS:-90}"
SLO_ENFORCE="${SLO_ENFORCE:-false}"
SLO_SNAPSHOT_OUTPUT="${SLO_SNAPSHOT_OUTPUT:-$ROOT_DIR/.local/slo/snapshot-$(date -u +%Y%m%dT%H%M%SZ).json}"
# The report destination is ignored by Git; the optional RTO is measured by
# the executed recovery drill rather than inferred from a Prometheus scrape.
SLO_REPORT_PATH="${SLO_REPORT_PATH:-$ROOT_DIR/docs/architecture/quarterly-recovery-report.md}"
SLO_RTO_SECONDS="${SLO_RTO_SECONDS:-}"
SLO_ENVIRONMENT="${SLO_ENVIRONMENT:-unknown}"
SLO_RELEASE_SHA="${SLO_RELEASE_SHA:-unknown}"
SLO_DRILL="${SLO_DRILL:-snapshot}"
SLO_WRITE_LOCAL_REPORT="${SLO_WRITE_LOCAL_REPORT:-true}"
SLO_REPORT_RENDERER="${SLO_REPORT_RENDERER:-$ROOT_DIR/scripts/append-quarterly-slo-report.py}"
# Packaged releases keep the renderer beside this script; source checkouts keep
# it in scripts/. This fallback lets the same evidence command run in both.
if [[ ! -f "$SLO_REPORT_RENDERER" && -f "$SCRIPT_DIR/append-quarterly-slo-report.py" ]]; then
  SLO_REPORT_RENDERER="$SCRIPT_DIR/append-quarterly-slo-report.py"
fi

usage() {
  cat <<'EOF'
Usage: PROMETHEUS_URL=https://prometheus.example scripts/collect-slo-snapshot.sh

Environment:
  PROMETHEUS_URL                         Prometheus base URL (required)
  PROMETHEUS_BEARER_TOKEN                optional Authorization bearer token
  SLO_WINDOW                             PromQL observation range (default 15m)
  SLO_API_P95_SECONDS                    API P95 objective (default 0.3)
  SLO_OPERATION_FAILURE_RATIO            terminal Operation failure-ratio objective (default 0.01)
  SLO_QUEUE_PENDING_MAX                  maximum queue backlog (default 100)
  SLO_WORKER_HEARTBEAT_MAX_AGE_SECONDS   maximum heartbeat age (default 90)
  SLO_ENFORCE                            true makes any objective violation fail the command
  SLO_SNAPSHOT_OUTPUT                    JSON evidence path (default .local/slo/...)
  SLO_REPORT_PATH                        ignored local Markdown report path
  SLO_RTO_SECONDS                        measured recovery duration to append
  SLO_ENVIRONMENT / SLO_RELEASE_SHA / SLO_DRILL
                                         report metadata without credentials
  SLO_WRITE_LOCAL_REPORT                 true appends the local report (default true)
  SLO_REPORT_RENDERER                    report renderer; source/release default is resolved automatically
EOF
}

case "${1:---collect}" in
  --help|-h|help)
    usage
    exit 0
    ;;
  --collect)
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac

[[ -n "$PROMETHEUS_URL" ]] || { echo "PROMETHEUS_URL is required" >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { echo "curl is required" >&2; exit 1; }
command -v python >/dev/null 2>&1 || { echo "python is required" >&2; exit 1; }

PROMETHEUS_URL="${PROMETHEUS_URL%/}"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT
mkdir -p "$(dirname "$SLO_SNAPSHOT_OUTPUT")"

curl_args=(--fail --silent --show-error --get)
if [[ -n "$PROMETHEUS_BEARER_TOKEN" ]]; then
  curl_args+=(-H "Authorization: Bearer $PROMETHEUS_BEARER_TOKEN")
fi

query_metric() {
  local name="$1"
  local query="$2"
  curl "${curl_args[@]}" --data-urlencode "query=$query" "$PROMETHEUS_URL/api/v1/query" > "$tmp_dir/$name.json"
}

query_metric api_p95 "histogram_quantile(0.95, sum(rate(caiyun_http_request_duration_seconds_bucket[$SLO_WINDOW])) by (le))"
query_metric operation_failure_ratio "(sum(rate(caiyun_operation_transitions_total{status=\"failed\"}[$SLO_WINDOW])) or vector(0)) / clamp_min((sum(rate(caiyun_operation_transitions_total{status=~\"succeeded|failed|canceled\"}[$SLO_WINDOW])) or vector(0)), 1)"
query_metric queue_pending "max(caiyun_queue_pending)"
query_metric worker_heartbeat_age "time() - max(caiyun_worker_heartbeat_unix{job=\"caiyun-worker\"})"

python - "$SLO_SNAPSHOT_OUTPUT" "$SLO_WINDOW" "$SLO_API_P95_SECONDS" "$SLO_OPERATION_FAILURE_RATIO" "$SLO_QUEUE_PENDING_MAX" "$SLO_WORKER_HEARTBEAT_MAX_AGE_SECONDS" "$SLO_ENFORCE" "$tmp_dir" <<'PY'
from __future__ import annotations

from datetime import datetime, timezone
import json
from pathlib import Path
import sys

output = Path(sys.argv[1])
window = sys.argv[2]
thresholds = {
    "api_p95_seconds": float(sys.argv[3]),
    "operation_failure_ratio": float(sys.argv[4]),
    "queue_pending": float(sys.argv[5]),
    "worker_heartbeat_age_seconds": float(sys.argv[6]),
}
enforce = sys.argv[7].lower() == "true"
temp_dir = Path(sys.argv[8])


def metric(name: str) -> float | None:
    response = json.loads((temp_dir / f"{name}.json").read_text(encoding="utf-8"))
    if response.get("status") != "success":
        raise SystemExit(f"Prometheus query failed for {name}: {response}")
    results = response.get("data", {}).get("result", [])
    if not results:
        return None
    value = results[0].get("value", [None, None])[1]
    try:
        return float(value)
    except (TypeError, ValueError) as exc:
        raise SystemExit(f"Prometheus returned non-numeric {name}: {value!r}") from exc

values = {
    "api_p95_seconds": metric("api_p95"),
    "operation_failure_ratio": metric("operation_failure_ratio"),
    "queue_pending": metric("queue_pending"),
    "worker_heartbeat_age_seconds": metric("worker_heartbeat_age"),
}
violations: list[str] = []
for name, threshold in thresholds.items():
    value = values[name]
    if value is None:
        violations.append(f"{name} metric missing")
    elif value > threshold:
        violations.append(f"{name}={value:.6g} exceeds {threshold:.6g}")

snapshot = {
    "collected_at": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
    "window": window,
    "thresholds": thresholds,
    "values": values,
    "passed": not violations,
    "violations": violations,
}
output.write_text(json.dumps(snapshot, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(f"SLO snapshot written: {output}")
for name, value in values.items():
    print(f"{name}={value if value is not None else 'missing'} threshold={thresholds[name]}")
if violations:
    print("SLO violations: " + "; ".join(violations), file=sys.stderr)
    if enforce:
        raise SystemExit(1)
PY

if [[ "$SLO_WRITE_LOCAL_REPORT" == "true" ]]; then
  [[ -f "$SLO_REPORT_RENDERER" ]] || { echo "SLO report renderer not found: $SLO_REPORT_RENDERER" >&2; exit 1; }
  python "$SLO_REPORT_RENDERER" \
    --snapshot "$SLO_SNAPSHOT_OUTPUT" \
    --report "$SLO_REPORT_PATH" \
    --environment "$SLO_ENVIRONMENT" \
    --release-sha "$SLO_RELEASE_SHA" \
    --drill "$SLO_DRILL" \
    --rto-seconds "$SLO_RTO_SECONDS"
fi
