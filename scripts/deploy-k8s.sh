#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MANIFEST="${MANIFEST:-$ROOT/k8s/caiyun.yaml}"
NAMESPACE="${NAMESPACE:-caiyun}"
JOB_NAME="${JOB_NAME:-caiyun-migrate}"
MIGRATION_TIMEOUT="${MIGRATION_TIMEOUT:-10m}"
ROLLOUT_TIMEOUT="${ROLLOUT_TIMEOUT:-10m}"

[[ -f "$MANIFEST" ]] || { echo "Missing manifest: $MANIFEST" >&2; exit 1; }
command -v kubectl >/dev/null 2>&1 || { echo "kubectl is required" >&2; exit 1; }

# Initial installation may not have created the namespace yet.
if ! kubectl get namespace "$NAMESPACE" >/dev/null 2>&1; then
  kubectl create namespace "$NAMESPACE"
fi

# Completed fixed-name Jobs are not rerun by kubectl apply and their pod
# templates are immutable. Recreate the migration gate for every release.
kubectl -n "$NAMESPACE" delete job "$JOB_NAME" --ignore-not-found=true --wait=true
kubectl apply -f "$MANIFEST"
kubectl -n "$NAMESPACE" wait --for=condition=complete "job/$JOB_NAME" --timeout="$MIGRATION_TIMEOUT"

# API/Worker InitCore validates critical schema, so new pods cannot become
# Ready before the migration has produced the expected structure.
for deployment in backend-api backend-worker frontend; do
  kubectl -n "$NAMESPACE" rollout status "deployment/$deployment" --timeout="$ROLLOUT_TIMEOUT"
done

echo "Kubernetes migration and rollouts completed"
