#!/usr/bin/env bash
# Controlled recovery drills. Defaults to a dry-run plan; --execute performs
# exactly one selected drill and then delegates verification to the read-only
# runtime checker.
set -euo pipefail

NAMESPACE="${NAMESPACE:-caiyun}"
DRILL="${1:---help}"
EXECUTE="${2:-}"

usage() {
  cat <<'EOF'
Usage: scripts/chaos-drill.sh {worker-recovery|api-rollout} [--execute]

Without --execute, prints the planned kubectl action only. Set NAMESPACE and
optionally BACKEND_IMAGE/REQUIRE_KEDA and set PROMETHEUS_URL for post-drill runtime and SLO evidence checks.
EOF
}

case "$DRILL" in
  worker-recovery)
    target="deployment/backend-worker"
    action="delete one backend-worker Pod and verify Streams recovery, heartbeat, and KEDA readiness"
    selector="app=backend-worker"
    ;;
  api-rollout)
    target="deployment/backend-api"
    action="restart API Pods and verify readiness plus migration/image invariants"
    selector=""
    ;;
  --help|-h|help)
    usage
    exit 0
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac

echo "drill=$DRILL namespace=$NAMESPACE target=$target action=$action"
if [[ "$EXECUTE" != "--execute" ]]; then
  echo "dry-run: append --execute after an approved maintenance window"
  exit 0
fi

command -v kubectl >/dev/null 2>&1 || { echo "kubectl is required" >&2; exit 1; }
[[ -n "${PROMETHEUS_URL:-}" ]] || { echo "PROMETHEUS_URL is required for recovery SLO evidence" >&2; exit 1; }
if [[ "$DRILL" == "worker-recovery" ]]; then
  pod="$(kubectl -n "$NAMESPACE" get pods -l "$selector" -o jsonpath='{.items[0].metadata.name}')"
  [[ -n "$pod" ]] || { echo "no worker Pod found" >&2; exit 1; }
  kubectl -n "$NAMESPACE" delete pod "$pod" --wait=true
else
  kubectl -n "$NAMESPACE" rollout restart "$target"
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
"$SCRIPT_DIR/verify-k8s-runtime.sh"
SLO_ENFORCE="${SLO_ENFORCE:-true}" "$SCRIPT_DIR/collect-slo-snapshot.sh"
