#!/usr/bin/env bash
# Capacity drill gate for a pre-reviewed, synthetic load Job. It never creates
# load by default; --execute requires an explicit local manifest and checks the
# observed worker scale before the release runtime invariants are revalidated.
set -euo pipefail

NAMESPACE="${NAMESPACE:-caiyun}"
CAPACITY_JOB_NAME="${CAPACITY_JOB_NAME:-caiyun-capacity-drill}"
CAPACITY_JOB_MANIFEST="${CAPACITY_JOB_MANIFEST:-}"
ROLLOUT_TIMEOUT="${ROLLOUT_TIMEOUT:-15m}"
TARGET_WORKER_REPLICAS="${TARGET_WORKER_REPLICAS:-3}"
EXECUTE="${1:---dry-run}"

usage() {
  cat <<'EOF'
Usage: scripts/capacity-drill.sh [--execute]

Environment:
  NAMESPACE               Kubernetes namespace (default caiyun)
  CAPACITY_JOB_MANIFEST   reviewed synthetic-load Job manifest; required for --execute
  CAPACITY_JOB_NAME       Job name expected after apply (default caiyun-capacity-drill)
  TARGET_WORKER_REPLICAS  minimum available backend-worker replicas to observe (default 3)
  ROLLOUT_TIMEOUT         overall wait timeout (default 15m)
  PROMETHEUS_URL          required for --execute SLO evidence collection

Without --execute, this command prints the quarterly drill plan and performs no
cluster mutation. The manifest must produce only synthetic authenticated load,
be uniquely labelled app=caiyun-capacity-drill, and have a finite deadline.
EOF
}

case "$EXECUTE" in
  --help|-h|help)
    usage
    exit 0
    ;;
  --dry-run)
    ;;
  --execute)
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac

echo "capacity-drill namespace=$NAMESPACE job=$CAPACITY_JOB_NAME target_worker_replicas=$TARGET_WORKER_REPLICAS timeout=$ROLLOUT_TIMEOUT"
if [[ "$EXECUTE" != "--execute" ]]; then
  echo "dry-run: prepare a reviewed CAPACITY_JOB_MANIFEST, define SLO thresholds, then append --execute during the maintenance window"
  exit 0
fi

command -v kubectl >/dev/null 2>&1 || { echo "kubectl is required" >&2; exit 1; }
[[ -n "$CAPACITY_JOB_MANIFEST" && -f "$CAPACITY_JOB_MANIFEST" ]] || { echo "CAPACITY_JOB_MANIFEST must point to a reviewed local Job manifest" >&2; exit 1; }
[[ "$TARGET_WORKER_REPLICAS" =~ ^[1-9][0-9]*$ ]] || { echo "TARGET_WORKER_REPLICAS must be a positive integer" >&2; exit 1; }
[[ -n "${PROMETHEUS_URL:-}" ]] || { echo "PROMETHEUS_URL is required for capacity SLO evidence" >&2; exit 1; }

# Reject an unbounded or unlabelled fixture before applying it. This permits
# operators to retain the actual request mix and temporary credentials locally.
grep -q 'kind: Job' "$CAPACITY_JOB_MANIFEST" || { echo "capacity manifest must contain a Job" >&2; exit 1; }
grep -q 'app: caiyun-capacity-drill' "$CAPACITY_JOB_MANIFEST" || { echo "capacity manifest must use app=caiyun-capacity-drill" >&2; exit 1; }
grep -Eq 'activeDeadlineSeconds: [1-9][0-9]*' "$CAPACITY_JOB_MANIFEST" || { echo "capacity manifest must set activeDeadlineSeconds" >&2; exit 1; }

kubectl -n "$NAMESPACE" delete job "$CAPACITY_JOB_NAME" --ignore-not-found --wait=true
kubectl -n "$NAMESPACE" apply -f "$CAPACITY_JOB_MANIFEST"

duration_seconds() {
  local value="$1"
  if [[ "$value" =~ ^([1-9][0-9]*)s$ ]]; then
    echo "${BASH_REMATCH[1]}"
  elif [[ "$value" =~ ^([1-9][0-9]*)m$ ]]; then
    echo "$(( ${BASH_REMATCH[1]} * 60 ))"
  elif [[ "$value" =~ ^([1-9][0-9]*)h$ ]]; then
    echo "$(( ${BASH_REMATCH[1]} * 3600 ))"
  else
    echo "ROLLOUT_TIMEOUT must use a positive s, m, or h duration" >&2
    return 1
  fi
}

# The check intentionally runs while the fixture is active; it captures the
# scale-up response instead of merely a completed Job's post-cooldown state.
deadline=$((SECONDS + $(duration_seconds "$ROLLOUT_TIMEOUT")))
scaled=false
while (( SECONDS < deadline )); do
  available="$(kubectl -n "$NAMESPACE" get deployment backend-worker -o jsonpath='{.status.availableReplicas}' 2>/dev/null || true)"
  available="${available:-0}"
  if [[ "$available" =~ ^[0-9]+$ ]] && (( available >= TARGET_WORKER_REPLICAS )); then
    scaled=true
    break
  fi
  sleep 10
done
[[ "$scaled" == "true" ]] || { echo "backend-worker did not reach $TARGET_WORKER_REPLICAS available replicas during capacity window" >&2; exit 1; }

kubectl -n "$NAMESPACE" wait --for=condition=complete "job/$CAPACITY_JOB_NAME" --timeout="$ROLLOUT_TIMEOUT"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
"$SCRIPT_DIR/verify-k8s-runtime.sh"
SLO_ENFORCE="${SLO_ENFORCE:-true}" "$SCRIPT_DIR/collect-slo-snapshot.sh"
echo "capacity drill passed: worker_replicas>=$TARGET_WORKER_REPLICAS job=$CAPACITY_JOB_NAME"
