#!/usr/bin/env bash
set -euo pipefail

API_URL="${API_URL:-http://127.0.0.1:8080/readyz}"
WORKER_URL="${WORKER_URL:-http://127.0.0.1:8081/readyz}"
PUBLIC_URL="${PUBLIC_URL:-}"
TIMEOUT="${TIMEOUT:-5}"
RETRIES="${RETRIES:-12}"
INTERVAL="${INTERVAL:-2}"

check() {
  local name="$1"
  local url="$2"
  local attempt
  echo "==> $name $url"
  for attempt in $(seq 1 "$RETRIES"); do
    if curl -fsS --max-time "$TIMEOUT" "$url"; then
      echo
      return 0
    fi
    if [[ "$attempt" -lt "$RETRIES" ]]; then
      echo "health check failed for $name (attempt $attempt/$RETRIES), retrying in ${INTERVAL}s..." >&2
      sleep "$INTERVAL"
    fi
  done
  echo "health check failed for $name after $RETRIES attempts" >&2
  return 1
}

check "api" "$API_URL"
check "worker" "$WORKER_URL"
if [[ -n "$PUBLIC_URL" ]]; then
  check "public" "$PUBLIC_URL"
fi

echo "health checks passed"
