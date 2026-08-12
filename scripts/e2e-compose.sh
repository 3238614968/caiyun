#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="$ROOT_DIR/.local/e2e.env"
PROJECT_NAME="${COMPOSE_PROJECT_NAME:-caiyun-e2e}"

mkdir -p "$ROOT_DIR/.local"

if [[ ! -f "$ENV_FILE" ]]; then
  cat > "$ENV_FILE" <<'EOF'
MYSQL_ROOT_PASSWORD=caiyun_root_e2e_change_me
MYSQL_USER=caiyun_app
MYSQL_PASSWORD=caiyun_app_e2e_change_me
REDIS_PASSWORD=caiyun_redis_e2e_change_me
JWT_SECRET=0123456789abcdef0123456789abcdef
DATA_ENCRYPTION_KEYS=v1=0123456789abcdef0123456789abcdef
DATA_ENCRYPTION_CURRENT_VERSION=v1
TRUSTED_PROXIES=none
WORKER_MONITOR_TOKEN=caiyun_worker_e2e_token
API_MONITOR_TOKEN=caiyun_api_e2e_token
GRAFANA_ADMIN_PASSWORD=caiyun_grafana_e2e_change_me
TASK_QUEUE_BACKEND=streams
ALLOWED_ORIGINS=http://frontend:8080,http://localhost,http://127.0.0.1
EOF
fi

grep -q '^API_MONITOR_TOKEN=' "$ENV_FILE" || printf '\nAPI_MONITOR_TOKEN=caiyun_api_e2e_token\n' >> "$ENV_FILE"

cleanup() {
  if [[ "${KEEP_E2E_STACK:-0}" != "1" ]]; then
    docker compose --project-name "$PROJECT_NAME" --env-file "$ENV_FILE" \
      -f "$ROOT_DIR/docker-compose.yml" -f "$ROOT_DIR/docker-compose.e2e.yml" down -v --remove-orphans
  fi
}
trap cleanup EXIT

docker compose --project-name "$PROJECT_NAME" --env-file "$ENV_FILE" \
  -f "$ROOT_DIR/docker-compose.yml" -f "$ROOT_DIR/docker-compose.e2e.yml" \
  up --build --abort-on-container-exit --exit-code-from e2e-runner e2e-runner
