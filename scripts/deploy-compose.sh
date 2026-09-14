#!/usr/bin/env bash
# Build, migrate, bootstrap and verify the complete Docker Compose stack.
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="$ROOT_DIR/.env"
MODE="local"
GENERATED_ADMIN_PASSWORD=""

cd "$ROOT_DIR"

usage() {
  cat <<'EOF'
Usage: scripts/deploy-compose.sh [--local|--production]

The script creates a secure .env on first use, builds the images, runs the
dedicated migration and admin bootstrap jobs, starts the stack, and checks
MySQL/Redis/API/Worker/frontend readiness. Existing .env and data volumes are
never overwritten or deleted.
EOF
}

rand_hex() {
  od -An -N "$1" -tx1 /dev/urandom | tr -d ' \n'
}

port_is_busy() {
  (echo >/dev/tcp/127.0.0.1/"$1") 2>/dev/null
}

pick_free_port() {
  local candidate="$1"
  while port_is_busy "$candidate"; do
    candidate=$((candidate + 1))
  done
  printf '%s' "$candidate"
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "required command not found: $1" >&2
    exit 1
  }
}

env_value() {
  local key="$1"
  grep -E "^${key}=" "$ENV_FILE" | tail -n 1 | cut -d= -f2-
}

require_env_value() {
  local key="$1" value
  value="$(env_value "$key")"
  if [[ -z "$value" || "$value" == *replace_with* || "$value" == *__GENERATE* ]]; then
    echo ".env contains an empty or placeholder value for $key" >&2
    exit 1
  fi
}

write_initial_env() {
  local app_env cookie_secure origin trusted_proxies
  if [[ "$MODE" == "production" ]]; then
    origin="${PUBLIC_ORIGIN:-}"
    [[ "$origin" == https://* ]] || {
      echo "--production requires PUBLIC_ORIGIN=https://..." >&2
      exit 1
    }
    app_env=production
    cookie_secure=true
    trusted_proxies="${TRUSTED_PROXIES:-none}"
  else
    origin="${PUBLIC_ORIGIN:-http://localhost,http://127.0.0.1}"
    app_env=development
    cookie_secure=false
    trusted_proxies=none
  fi

  local admin_password
  # Include one letter and one digit deterministically for the backend policy.
  admin_password="A1$(rand_hex 22)"
  GENERATED_ADMIN_PASSWORD="$admin_password"
  umask 077
  cat > "$ENV_FILE" <<EOF
APP_ENV=$app_env
COOKIE_SECURE=$cookie_secure
CAIYUN_BIND_ADDRESS=${CAIYUN_BIND_ADDRESS:-127.0.0.1}
CAIYUN_HTTP_PORT=${CAIYUN_HTTP_PORT:-$(pick_free_port 8088)}
CAIYUN_API_PORT=${CAIYUN_API_PORT:-$(pick_free_port 18080)}
CAIYUN_WORKER_PORT=${CAIYUN_WORKER_PORT:-$(pick_free_port 18081)}
CAIYUN_GOPROXY=${CAIYUN_GOPROXY:-https://goproxy.cn,direct}
MYSQL_ROOT_PASSWORD=$(rand_hex 24)
MYSQL_USER=caiyun_app
MYSQL_PASSWORD=$(rand_hex 24)
REDIS_PASSWORD=$(rand_hex 24)
DB_AUTO_MIGRATE=false
TASK_CONFIG_SYNC_ON_STARTUP=false
RATE_LIMIT_BACKEND=redis
TASK_QUEUE_BACKEND=streams
TASK_QUEUE_STREAM_GROUP=caiyun-workers
JWT_SECRET=$(rand_hex 32)
JWT_ISSUER=caiyun-api
JWT_AUDIENCE=caiyun-web
JWT_ACCESS_TTL=15m
JWT_REFRESH_TTL=720h
DATA_ENCRYPTION_KEYS=v1=$(rand_hex 32)
DATA_ENCRYPTION_CURRENT_VERSION=v1
FIELD_CRYPTO_ALLOW_LEGACY_NO_AAD=false
ALLOWED_ORIGINS=$origin
TRUSTED_PROXIES=$trusted_proxies
API_MONITOR_TOKEN=$(rand_hex 24)
WORKER_MONITOR_TOKEN=$(rand_hex 24)
WORKER_MONITOR_HOST=0.0.0.0
WORKER_MONITOR_PORT=8081
WORKER_MONITOR_ALLOW_PLAINTEXT=true
BOOTSTRAP_ADMIN_USERNAME=admin
BOOTSTRAP_ADMIN_PASSWORD=$admin_password
BOOTSTRAP_ADMIN_EMAIL=admin@localhost
GRAFANA_ADMIN_USER=admin
GRAFANA_ADMIN_PASSWORD=$(rand_hex 24)
LOG_FILE_PATH=
LOG_JSON_FORMAT=true
TZ=Asia/Shanghai
EOF
  chmod 600 "$ENV_FILE"
}

wait_service_health() {
  local service="$1" timeout="${2:-120}" id status deadline
  deadline=$(( $(date +%s) + timeout ))
  while (( $(date +%s) < deadline )); do
    id="$(docker compose --env-file "$ENV_FILE" ps -q "$service" 2>/dev/null || true)"
    if [[ -n "$id" ]]; then
      status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$id" 2>/dev/null || true)"
      [[ "$status" == healthy ]] && return 0
      [[ "$status" == exited || "$status" == dead ]] && break
    fi
    sleep 2
  done
  echo "service did not become healthy: $service" >&2
  return 1
}

on_error() {
  local status=$?
  set +e
  echo "Deployment failed; preserving containers and volumes for diagnosis." >&2
  docker compose --env-file "$ENV_FILE" ps -a >&2
  docker compose --env-file "$ENV_FILE" logs --no-color --tail=100 \
    backend-migrate backend-admin-init backend-api backend-worker frontend >&2
  exit "$status"
}

while (($#)); do
  case "$1" in
    --local) MODE=local ;;
    --production) MODE=production ;;
    --help|-h) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done

require_command docker
docker compose version >/dev/null
docker info >/dev/null
trap on_error ERR

if [[ ! -f "$ENV_FILE" ]]; then
  write_initial_env
  echo "Created $ENV_FILE with mode=$MODE"
  if [[ -n "$GENERATED_ADMIN_PASSWORD" ]]; then
    echo "Initial admin username: admin"
    if [[ "${CI:-}" == true ]]; then
      echo "Initial admin password was generated and stored in .env (not printed in CI)."
    else
      echo "Initial admin password: $GENERATED_ADMIN_PASSWORD"
      echo "Save this password; it will not be printed again."
    fi
  fi
fi

for key in MYSQL_ROOT_PASSWORD MYSQL_PASSWORD REDIS_PASSWORD JWT_SECRET \
  DATA_ENCRYPTION_KEYS DATA_ENCRYPTION_CURRENT_VERSION API_MONITOR_TOKEN \
  WORKER_MONITOR_TOKEN GRAFANA_ADMIN_PASSWORD BOOTSTRAP_ADMIN_USERNAME \
  BOOTSTRAP_ADMIN_PASSWORD; do
  require_env_value "$key"
done

if [[ "$MODE" == "production" ]]; then
  [[ "$(env_value APP_ENV)" == production ]] || {
    echo "existing .env is not APP_ENV=production" >&2
    exit 1
  }
  [[ "$(env_value COOKIE_SECURE)" == true ]] || {
    echo "production requires COOKIE_SECURE=true" >&2
    exit 1
  }
  [[ "$(env_value ALLOWED_ORIGINS)" == https://* ]] || {
    echo "production requires an HTTPS ALLOWED_ORIGINS value" >&2
    exit 1
  }
fi

docker compose --env-file "$ENV_FILE" config --quiet
docker compose --env-file "$ENV_FILE" build --pull
docker compose --env-file "$ENV_FILE" up -d mysql redis
wait_service_health mysql 180
wait_service_health redis 120

# These jobs are intentionally run explicitly so failures stop the deployment
# before serving traffic. Compose remains responsible for rerunning them when
# the normal stack is reconciled below.
docker compose --env-file "$ENV_FILE" rm -sf backend-migrate backend-admin-init >/dev/null 2>&1 || true
docker compose --env-file "$ENV_FILE" run --rm backend-migrate
docker compose --env-file "$ENV_FILE" run --rm backend-admin-init
docker compose --env-file "$ENV_FILE" up -d backend-api backend-worker frontend grafana

wait_service_health backend-api 180
wait_service_health backend-worker 180
wait_service_health frontend 120

http_port="$(env_value CAIYUN_HTTP_PORT)"
http_port="${http_port:-80}"
api_port="$(env_value CAIYUN_API_PORT)"
api_port="${api_port:-8080}"
worker_port="$(env_value CAIYUN_WORKER_PORT)"
worker_port="${worker_port:-8081}"
curl --fail --silent --show-error "http://127.0.0.1:${api_port}/readyz" >/dev/null
curl --fail --silent --show-error "http://127.0.0.1:${worker_port}/readyz" >/dev/null
curl --fail --silent --show-error "http://127.0.0.1:${http_port}/" >/dev/null
docker compose --env-file "$ENV_FILE" ps

echo "Docker Compose deployment completed."
echo "Web: http://127.0.0.1:${http_port}"
echo "API readiness: http://127.0.0.1:${api_port}/readyz"
echo "Worker readiness: http://127.0.0.1:${worker_port}/readyz"
