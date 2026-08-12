#!/usr/bin/env bash
# Verify the production OTLP Collector that receives spans from Caiyun API and
# Worker. The check is read-only to the cluster; port-forward is used only to
# probe the Collector's private health endpoint from the operator workstation.
set -euo pipefail

NAMESPACE="${NAMESPACE:-caiyun}"
COLLECTOR_DEPLOYMENT="${OTEL_COLLECTOR_DEPLOYMENT:-caiyun-otel-collector}"
COLLECTOR_SERVICE="${OTEL_COLLECTOR_SERVICE:-caiyun-otel-collector}"
EXPECTED_ENDPOINT="${OTEL_COLLECTOR_ENDPOINT:-${COLLECTOR_SERVICE}:4317}"
ROLLOUT_TIMEOUT="${ROLLOUT_TIMEOUT:-5m}"
LOCAL_HEALTH_PORT="${OTEL_LOCAL_HEALTH_PORT:-18133}"
HEALTH_TIMEOUT_SECONDS="${OTEL_HEALTH_TIMEOUT_SECONDS:-30}"
PORT_FORWARD_PID=""

command -v kubectl >/dev/null 2>&1 || { echo "kubectl is required" >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { echo "curl is required" >&2; exit 1; }

cleanup() {
  if [[ -n "$PORT_FORWARD_PID" ]]; then
    kill "$PORT_FORWARD_PID" >/dev/null 2>&1 || true
    wait "$PORT_FORWARD_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

require_equal() {
  local actual="$1" expected="$2" label="$3"
  [[ "$actual" == "$expected" ]] || { echo "$label: got '$actual', want '$expected'" >&2; exit 1; }
}

kubectl -n "$NAMESPACE" rollout status "deployment/$COLLECTOR_DEPLOYMENT" --timeout="$ROLLOUT_TIMEOUT"
kubectl -n "$NAMESPACE" get "service/$COLLECTOR_SERVICE" >/dev/null
kubectl -n "$NAMESPACE" get secret caiyun-otel-receiver-tls >/dev/null
kubectl -n "$NAMESPACE" get secret caiyun-otel-receiver-ca >/dev/null
kubectl -n "$NAMESPACE" get secret caiyun-otel-exporter >/dev/null

endpoint="$(kubectl -n "$NAMESPACE" get configmap caiyun-config -o jsonpath='{.data.OTEL_EXPORTER_OTLP_ENDPOINT}')"
insecure="$(kubectl -n "$NAMESPACE" get configmap caiyun-config -o jsonpath='{.data.OTEL_EXPORTER_OTLP_INSECURE}')"
certificate="$(kubectl -n "$NAMESPACE" get configmap caiyun-config -o jsonpath='{.data.OTEL_EXPORTER_OTLP_CERTIFICATE}')"
require_equal "$endpoint" "$EXPECTED_ENDPOINT" "OTLP endpoint"
require_equal "$insecure" "false" "OTLP transport security"
[[ -n "$certificate" ]] || { echo "OTLP CA certificate path must be configured" >&2; exit 1; }

for deployment in backend-api backend-worker; do
  manifest="$(kubectl -n "$NAMESPACE" get "deployment/$deployment" -o yaml)"
  [[ "$manifest" == *"caiyun-otel-receiver-ca"* && "$manifest" == *"$certificate"* ]] || {
    echo "$deployment does not mount the configured OTLP CA certificate" >&2
    exit 1
  }
done

kubectl -n "$NAMESPACE" port-forward "service/$COLLECTOR_SERVICE" "${LOCAL_HEALTH_PORT}:13133" >/dev/null 2>&1 &
PORT_FORWARD_PID="$!"
deadline=$((SECONDS + HEALTH_TIMEOUT_SECONDS))
until curl --fail --silent --show-error "http://127.0.0.1:${LOCAL_HEALTH_PORT}/" >/dev/null 2>&1; do
  if (( SECONDS >= deadline )); then
    echo "Collector health endpoint did not become ready within ${HEALTH_TIMEOUT_SECONDS}s" >&2
    exit 1
  fi
  sleep 1
done

echo "OTel Collector runtime verified: namespace=$NAMESPACE endpoint=$endpoint"
