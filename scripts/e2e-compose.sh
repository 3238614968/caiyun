#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="$ROOT_DIR/.local/e2e.env"
PROJECT_NAME="${COMPOSE_PROJECT_NAME:-caiyun-e2e}"

mkdir -p "$ROOT_DIR/.local"

# 镜像内的 npm 钩子会跳过契约生成（容器没有 python），必须先在宿主机生成
# frontend/src/api/generated，再交给 docker build 的 COPY 上下文。
# 依次探测可用的解释器（Windows 的 python3 可能是商店占位符，跑不通）。
PY=""
for candidate in python3 python py; do
  if command -v "$candidate" >/dev/null 2>&1 && "$candidate" --version >/dev/null 2>&1; then
    PY="$candidate"
    break
  fi
done
if [ -z "$PY" ]; then
  echo "contract generation needs a working python (python3/python/py)" >&2
  exit 1
fi
"$PY" "$ROOT_DIR/scripts/generate-openapi-local.py" >/dev/null
"$PY" "$ROOT_DIR/scripts/generate-asyncapi-local.py" >/dev/null
"$PY" "$ROOT_DIR/scripts/generate-api-client-types.py" >/dev/null

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
BOOTSTRAP_ADMIN_USERNAME=e2e-admin
BOOTSTRAP_ADMIN_PASSWORD=E2eAdminPass123!
BOOTSTRAP_ADMIN_EMAIL=e2e-admin@example.local
GRAFANA_ADMIN_PASSWORD=caiyun_grafana_e2e_change_me
TASK_QUEUE_BACKEND=streams
ALLOWED_ORIGINS=http://frontend:8080,http://localhost,http://127.0.0.1
EOF
fi

grep -q '^API_MONITOR_TOKEN=' "$ENV_FILE" || printf '\nAPI_MONITOR_TOKEN=caiyun_api_e2e_token\n' >> "$ENV_FILE"
grep -q '^BOOTSTRAP_ADMIN_USERNAME=' "$ENV_FILE" || printf 'BOOTSTRAP_ADMIN_USERNAME=e2e-admin\n' >> "$ENV_FILE"
grep -q '^BOOTSTRAP_ADMIN_PASSWORD=' "$ENV_FILE" || printf 'BOOTSTRAP_ADMIN_PASSWORD=E2eAdminPass123!\n' >> "$ENV_FILE"
grep -q '^BOOTSTRAP_ADMIN_EMAIL=' "$ENV_FILE" || printf 'BOOTSTRAP_ADMIN_EMAIL=e2e-admin@example.local\n' >> "$ENV_FILE"

cleanup() {
  local status=$?
  # 失败时先导出容器状态与关键日志(down -v 之后无从取证)。
  if [ "$status" -ne 0 ] && [ "${KEEP_E2E_STACK:-0}" != "1" ]; then
    docker compose --project-name "$PROJECT_NAME" --env-file "$ENV_FILE" \
      -f "$ROOT_DIR/docker-compose.yml" -f "$ROOT_DIR/docker-compose.e2e.yml" ps -a || true
    docker compose --project-name "$PROJECT_NAME" --env-file "$ENV_FILE" \
      -f "$ROOT_DIR/docker-compose.yml" -f "$ROOT_DIR/docker-compose.e2e.yml" \
      logs --no-color --tail 10 backend-migrate backend-worker || true
    # api 日志最后输出且保留最多行,方便 CI 端按前缀截取注解。
    docker compose --project-name "$PROJECT_NAME" --env-file "$ENV_FILE" \
      -f "$ROOT_DIR/docker-compose.yml" -f "$ROOT_DIR/docker-compose.e2e.yml" \
      logs --no-color --tail 50 backend-api || true
  fi
  if [[ "${KEEP_E2E_STACK:-0}" != "1" ]]; then
    docker compose --project-name "$PROJECT_NAME" --env-file "$ENV_FILE" \
      -f "$ROOT_DIR/docker-compose.yml" -f "$ROOT_DIR/docker-compose.e2e.yml" down -v --remove-orphans
  fi
}
trap cleanup EXIT

docker compose --project-name "$PROJECT_NAME" --env-file "$ENV_FILE" \
  -f "$ROOT_DIR/docker-compose.yml" -f "$ROOT_DIR/docker-compose.e2e.yml" \
  up --build --abort-on-container-exit --exit-code-from e2e-runner e2e-runner
