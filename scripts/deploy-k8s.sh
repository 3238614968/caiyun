#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MANIFEST="${MANIFEST:-$ROOT/k8s/caiyun.yaml}"
KUSTOMIZE_OVERLAY="${KUSTOMIZE_OVERLAY:-}"
NAMESPACE="${NAMESPACE:-caiyun}"
JOB_NAME="${JOB_NAME:-caiyun-migrate}"
MIGRATION_TIMEOUT="${MIGRATION_TIMEOUT:-10m}"
ROLLOUT_TIMEOUT="${ROLLOUT_TIMEOUT:-10m}"
BACKEND_IMAGE="${BACKEND_IMAGE:-}"
FRONTEND_IMAGE="${FRONTEND_IMAGE:-}"
OTEL_COLLECTOR_IMAGE="${OTEL_COLLECTOR_IMAGE:-}"
RENDERED_MANIFEST=""
OBSERVABILITY_MANIFEST="${OBSERVABILITY_MANIFEST:-$ROOT/k8s/observability-prometheus-operator.yaml}"
KEDA_MANIFEST="${KEDA_MANIFEST:-$ROOT/k8s/autoscaling-keda.yaml}"

command -v kubectl >/dev/null 2>&1 || { echo "kubectl is required" >&2; exit 1; }

if [[ -n "$KUSTOMIZE_OVERLAY" ]]; then
  [[ -d "$KUSTOMIZE_OVERLAY" ]] || { echo "Missing Kustomize overlay: $KUSTOMIZE_OVERLAY" >&2; exit 1; }
else
  [[ -f "$MANIFEST" ]] || { echo "Missing manifest: $MANIFEST" >&2; exit 1; }
fi

cleanup() {
  [[ -z "$RENDERED_MANIFEST" ]] || rm -f "$RENDERED_MANIFEST"
}
trap cleanup EXIT

# API, Worker and Migration Job must use the exact same immutable backend
# image. Requiring a digest makes the migration gate reproducible across a
# rollout and prevents a mutable tag from changing schema behavior mid-release.
if [[ -z "$BACKEND_IMAGE" ]]; then
  echo "BACKEND_IMAGE must be an immutable image digest, for example registry/caiyun-backend@sha256:<64-hex>" >&2
  exit 1
fi
if [[ ! "$BACKEND_IMAGE" =~ @sha256:[a-fA-F0-9]{64}$ ]]; then
  echo "BACKEND_IMAGE must end with @sha256:<64-hex>" >&2
  exit 1
fi
if [[ -n "$KUSTOMIZE_OVERLAY" ]]; then
  RENDERED_MANIFEST=$(mktemp)
  kubectl kustomize "$KUSTOMIZE_OVERLAY" > "$RENDERED_MANIFEST"
  MANIFEST="$RENDERED_MANIFEST"
  if [[ -z "$FRONTEND_IMAGE" ]]; then
    echo "FRONTEND_IMAGE must be an immutable image digest when KUSTOMIZE_OVERLAY is used" >&2
    exit 1
  fi
fi

backend_occurrences=$(grep -Ec '^[[:space:]]*image:[[:space:]]+[^[:space:]]*caiyun-backend[^[:space:]]*$' "$MANIFEST" || true)
if [[ "$backend_occurrences" -ne 3 ]]; then
  echo "Manifest must contain exactly API, Worker and Migration Job backend images; found $backend_occurrences" >&2
  exit 1
fi
if [[ -n "$FRONTEND_IMAGE" && ! "$FRONTEND_IMAGE" =~ @sha256:[a-fA-F0-9]{64}$ ]]; then
  echo "FRONTEND_IMAGE must end with @sha256:<64-hex>" >&2
  exit 1
fi
collector_occurrences=$(grep -Ec '^[[:space:]]*image:[[:space:]]+[^[:space:]]*otel/opentelemetry-collector-contrib[^[:space:]]*$' "$MANIFEST" || true)
if (( collector_occurrences > 0 )); then
  if [[ "$collector_occurrences" -ne 1 ]]; then
    echo "Manifest must contain exactly one OTel Collector image; found $collector_occurrences" >&2
    exit 1
  fi
  if [[ ! "$OTEL_COLLECTOR_IMAGE" =~ @sha256:[a-fA-F0-9]{64}$ ]]; then
    echo "OTEL_COLLECTOR_IMAGE must be an immutable digest when an OTel Collector is deployed" >&2
    exit 1
  fi
fi
IMAGE_MANIFEST=$(mktemp)
if [[ -n "$FRONTEND_IMAGE" ]]; then
  sed -E \
    -e "s|^([[:space:]]*image:[[:space:]]+)[^[:space:]]*caiyun-backend[^[:space:]]*$|\\1$BACKEND_IMAGE|" \
    -e "s|^([[:space:]]*image:[[:space:]]+)[^[:space:]]*caiyun-frontend[^[:space:]]*$|\\1$FRONTEND_IMAGE|" \
    -e "s|^([[:space:]]*image:[[:space:]]+)[^[:space:]]*otel/opentelemetry-collector-contrib[^[:space:]]*$|\\1$OTEL_COLLECTOR_IMAGE|" \
    "$MANIFEST" > "$IMAGE_MANIFEST"
else
  sed -E \
    -e "s|^([[:space:]]*image:[[:space:]]+)[^[:space:]]*caiyun-backend[^[:space:]]*$|\\1$BACKEND_IMAGE|" \
    -e "s|^([[:space:]]*image:[[:space:]]+)[^[:space:]]*otel/opentelemetry-collector-contrib[^[:space:]]*$|\\1$OTEL_COLLECTOR_IMAGE|" \
    "$MANIFEST" > "$IMAGE_MANIFEST"
fi
[[ -z "$RENDERED_MANIFEST" ]] || rm -f "$RENDERED_MANIFEST"
RENDERED_MANIFEST="$IMAGE_MANIFEST"
MANIFEST="$RENDERED_MANIFEST"

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

if kubectl api-resources --api-group=monitoring.coreos.com --output=name 2>/dev/null | grep -qx 'servicemonitors.monitoring.coreos.com'; then
  [[ -f "$OBSERVABILITY_MANIFEST" ]] || { echo "Missing observability manifest: $OBSERVABILITY_MANIFEST" >&2; exit 1; }
  sed -E "s|^  namespace: caiyun$|  namespace: $NAMESPACE|" "$OBSERVABILITY_MANIFEST" | kubectl apply -f -
else
  echo "Prometheus Operator ServiceMonitor CRD not installed; skipped optional ServiceMonitor resources"
fi

# The base manifest keeps a resource HPA as a cluster-independent fallback. A
# ScaledObject owns its own HPA, therefore remove that one target HPA only when
# KEDA is present; otherwise the fallback remains active.
if kubectl api-resources --api-group=keda.sh --output=name 2>/dev/null | grep -qx 'scaledobjects.keda.sh'; then
  [[ -f "$KEDA_MANIFEST" ]] || { echo "Missing KEDA manifest: $KEDA_MANIFEST" >&2; exit 1; }
  kubectl -n "$NAMESPACE" delete hpa backend-worker --ignore-not-found=true --wait=true
  sed -E "s|^  namespace: caiyun$|  namespace: $NAMESPACE|" "$KEDA_MANIFEST" | kubectl apply -f -
else
  echo "KEDA ScaledObject CRD not installed; retained backend-worker resource HPA fallback"
fi

echo "Kubernetes migration and rollouts completed"
