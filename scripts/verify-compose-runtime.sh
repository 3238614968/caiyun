#!/usr/bin/env bash
# Verify and, when explicitly requested, exercise Compose graceful shutdown
# budgets. The command is deliberately opt-in because --execute restarts the
# selected service in the active Compose project.
set -euo pipefail

TARGET="${1:---help}"
EXECUTE="${2:-}"
API_SERVICE="${API_SERVICE:-backend-api}"
WORKER_SERVICE="${WORKER_SERVICE:-backend-worker}"
API_READY_URL="${API_READY_URL:-http://127.0.0.1:8080/readyz}"
WORKER_READY_URL="${WORKER_READY_URL:-http://127.0.0.1:8081/readyz}"
READY_TIMEOUT_SECONDS="${READY_TIMEOUT_SECONDS:-60}"

usage() {
  cat <<'EOF'
Usage: scripts/verify-compose-runtime.sh {api|worker|all} [--execute]

Validates configured Docker StopTimeout values (API=60s, Worker=75s). With
--execute it performs a bounded stop/start of the selected running service. API
and both process /readyz endpoints are checked after restart. Those readiness
checks include MySQL and Redis dependencies. Honor COMPOSE_FILE and COMPOSE_PROJECT_NAME
through the standard Docker Compose environment variables.
EOF
}

case "$TARGET" in
  api|worker|all) ;;
  --help|-h|help) usage; exit 0 ;;
  *) usage >&2; exit 2 ;;
esac

command -v docker >/dev/null 2>&1 || { echo "docker is required" >&2; exit 1; }
docker compose version >/dev/null

container_id() {
  docker compose ps -q "$1"
}

require_timeout() {
  local service="$1" expected="$2" id actual
  id="$(container_id "$service")"
  [[ -n "$id" ]] || { echo "Compose service $service is not running" >&2; exit 1; }
  actual="$(docker inspect --format '{{.Config.StopTimeout}}' "$id")"
  [[ "$actual" == "$expected" ]] || {
    echo "$service StopTimeout=$actual, expected $expected seconds" >&2
    exit 1
  }
}

wait_ready() {
  local label="$1" url="$2" deadline=$(( $(date +%s) + READY_TIMEOUT_SECONDS ))
  until curl --fail --silent --show-error "$url" >/dev/null; do
    if (( $(date +%s) >= deadline )); then
      echo "$label did not become ready within ${READY_TIMEOUT_SECONDS}s: $url" >&2
      exit 1
    fi
    sleep 2
  done
}

restart_with_budget() {
  local service="$1" timeout="$2" started elapsed
  started="$(date +%s)"
  docker compose stop -t "$timeout" "$service"
  elapsed=$(( $(date +%s) - started ))
  if (( elapsed > timeout )); then
    echo "$service exceeded graceful shutdown budget: ${elapsed}s > ${timeout}s" >&2
    exit 1
  fi
  docker compose up -d "$service"
  echo "$service graceful stop/start completed in ${elapsed}s (budget ${timeout}s)"
}

if [[ "$TARGET" == "api" || "$TARGET" == "all" ]]; then
  require_timeout "$API_SERVICE" 60
fi
if [[ "$TARGET" == "worker" || "$TARGET" == "all" ]]; then
  require_timeout "$WORKER_SERVICE" 75
fi
echo "Compose graceful shutdown configuration verified: target=$TARGET"

if [[ "$EXECUTE" != "--execute" ]]; then
  echo "dry-run: append --execute after opening a maintenance window"
  exit 0
fi

if [[ "$TARGET" == "api" || "$TARGET" == "all" ]]; then
  restart_with_budget "$API_SERVICE" 60
  wait_ready "API" "$API_READY_URL"
fi
if [[ "$TARGET" == "worker" || "$TARGET" == "all" ]]; then
  restart_with_budget "$WORKER_SERVICE" 75
  wait_ready "Worker" "$WORKER_READY_URL"
fi
echo "Compose graceful shutdown drill passed: target=$TARGET"
