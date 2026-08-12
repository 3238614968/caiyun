#!/usr/bin/env bash
# Verify the post-deploy runtime invariants that static manifests cannot prove.
# This script is read-only and is intended for a release pipeline or operator
# workstation after scripts/deploy-k8s.sh has completed.
set -euo pipefail

NAMESPACE="${NAMESPACE:-caiyun}"
JOB_NAME="${JOB_NAME:-caiyun-migrate}"
BACKEND_IMAGE="${BACKEND_IMAGE:-}"
ROLLOUT_TIMEOUT="${ROLLOUT_TIMEOUT:-5m}"
REQUIRE_METRICS_SERVER="${REQUIRE_METRICS_SERVER:-true}"
REQUIRE_SERVICEMONITOR="${REQUIRE_SERVICEMONITOR:-false}"
REQUIRE_KEDA="${REQUIRE_KEDA:-false}"
REQUIRE_OTEL_COLLECTOR="${REQUIRE_OTEL_COLLECTOR:-false}"
OTEL_COLLECTOR_ENDPOINT="${OTEL_COLLECTOR_ENDPOINT:-caiyun-otel-collector:4317}"
OTEL_COLLECTOR_IMAGE="${OTEL_COLLECTOR_IMAGE:-}"

command -v kubectl >/dev/null 2>&1 || { echo "kubectl is required" >&2; exit 1; }

require_jsonpath() {
  local actual="$1" expected="$2" label="$3"
  [[ "$actual" == "$expected" ]] || { echo "$label: got '$actual', want '$expected'" >&2; exit 1; }
}

kubectl -n "$NAMESPACE" wait --for=condition=complete "job/$JOB_NAME" --timeout="$ROLLOUT_TIMEOUT"
for deployment in backend-api backend-worker frontend; do
  kubectl -n "$NAMESPACE" rollout status "deployment/$deployment" --timeout="$ROLLOUT_TIMEOUT"
done

if [[ -n "$BACKEND_IMAGE" ]]; then
  [[ "$BACKEND_IMAGE" =~ @sha256:[a-fA-F0-9]{64}$ ]] || { echo "BACKEND_IMAGE must be an immutable digest" >&2; exit 1; }
  migrate_image="$(kubectl -n "$NAMESPACE" get job "$JOB_NAME" -o jsonpath='{.spec.template.spec.containers[?(@.name=="migrate")].image}')"
  api_image="$(kubectl -n "$NAMESPACE" get deployment backend-api -o jsonpath='{.spec.template.spec.containers[?(@.name=="api")].image}')"
  worker_image="$(kubectl -n "$NAMESPACE" get deployment backend-worker -o jsonpath='{.spec.template.spec.containers[?(@.name=="worker")].image}')"
  require_jsonpath "$migrate_image" "$BACKEND_IMAGE" "migration image"
  require_jsonpath "$api_image" "$BACKEND_IMAGE" "API image"
  require_jsonpath "$worker_image" "$BACKEND_IMAGE" "Worker image"
fi

# Match the measured application shutdown budgets: API stops realtime clients
# before its 30s HTTP drain, while Worker drains consumers and background work.
api_grace="$(kubectl -n "$NAMESPACE" get deployment backend-api -o jsonpath='{.spec.template.spec.terminationGracePeriodSeconds}')"
worker_grace="$(kubectl -n "$NAMESPACE" get deployment backend-worker -o jsonpath='{.spec.template.spec.terminationGracePeriodSeconds}')"
require_jsonpath "$api_grace" "60" "API termination grace period"
require_jsonpath "$worker_grace" "75" "Worker termination grace period"

if [[ "$REQUIRE_METRICS_SERVER" == "true" ]]; then
  kubectl get --raw /apis/metrics.k8s.io/v1beta1/nodes >/dev/null
fi

if kubectl api-resources --api-group=monitoring.coreos.com --output=name 2>/dev/null | grep -qx 'servicemonitors.monitoring.coreos.com'; then
  kubectl -n "$NAMESPACE" get servicemonitor caiyun-api caiyun-worker >/dev/null
elif [[ "$REQUIRE_SERVICEMONITOR" == "true" ]]; then
  echo "Prometheus Operator ServiceMonitor CRD is required but unavailable" >&2
  exit 1
fi

if kubectl api-resources --api-group=keda.sh --output=name 2>/dev/null | grep -qx 'scaledobjects.keda.sh'; then
  kubectl -n "$NAMESPACE" get scaledobject backend-worker-redis-streams >/dev/null
  ready="$(kubectl -n "$NAMESPACE" get scaledobject backend-worker-redis-streams -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}')"
  require_jsonpath "$ready" "True" "KEDA ScaledObject readiness"
  # KEDA owns its generated HPA; the resource-only fallback must be absent.
  if kubectl -n "$NAMESPACE" get hpa backend-worker >/dev/null 2>&1; then
    echo "fallback backend-worker HPA still exists while KEDA is enabled" >&2
    exit 1
  fi
elif [[ "$REQUIRE_KEDA" == "true" ]]; then
  echo "KEDA ScaledObject CRD is required but unavailable" >&2
  exit 1
fi

if [[ "$REQUIRE_OTEL_COLLECTOR" == "true" ]]; then
  kubectl -n "$NAMESPACE" rollout status deployment/caiyun-otel-collector --timeout="$ROLLOUT_TIMEOUT"
  kubectl -n "$NAMESPACE" get service/caiyun-otel-collector >/dev/null
  kubectl -n "$NAMESPACE" get secret caiyun-otel-receiver-tls caiyun-otel-receiver-ca caiyun-otel-exporter >/dev/null
  otel_endpoint="$(kubectl -n "$NAMESPACE" get configmap caiyun-config -o jsonpath='{.data.OTEL_EXPORTER_OTLP_ENDPOINT}')"
  otel_insecure="$(kubectl -n "$NAMESPACE" get configmap caiyun-config -o jsonpath='{.data.OTEL_EXPORTER_OTLP_INSECURE}')"
  otel_certificate="$(kubectl -n "$NAMESPACE" get configmap caiyun-config -o jsonpath='{.data.OTEL_EXPORTER_OTLP_CERTIFICATE}')"
  require_jsonpath "$otel_endpoint" "$OTEL_COLLECTOR_ENDPOINT" "OTLP Collector endpoint"
  require_jsonpath "$otel_insecure" "false" "OTLP Collector TLS setting"
  [[ -n "$otel_certificate" ]] || { echo "OTLP Collector CA path is not configured" >&2; exit 1; }
  [[ "$OTEL_COLLECTOR_IMAGE" =~ @sha256:[a-fA-F0-9]{64}$ ]] || { echo "OTEL_COLLECTOR_IMAGE must be an immutable digest when REQUIRE_OTEL_COLLECTOR=true" >&2; exit 1; }
  collector_image="$(kubectl -n "$NAMESPACE" get deployment caiyun-otel-collector -o jsonpath='{.spec.template.spec.containers[?(@.name=="otel-collector")].image}')"
  require_jsonpath "$collector_image" "$OTEL_COLLECTOR_IMAGE" "OTel Collector image"
fi

echo "Kubernetes runtime baseline verified: namespace=$NAMESPACE migration=$JOB_NAME"
