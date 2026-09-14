#!/data/data/com.termux/files/usr/bin/bash
# caiyun 应用部署：.env + 二进制 + migrate + API/Worker 启动
# 前置：
#   1. 已运行 deploy_infra.sh（生成 ~/caiyun/secrets.env，Redis noeviction、MariaDB root 已设密码）
#   2. 二进制已放在 ~/caiyun/caiyun-arm64.new（或已存在 ~/caiyun/caiyun-linux）
#      ⚠️ Termux/Android 上必须用 CGO_ENABLED=1 本机编译，
#      交叉编译的 CGO_ENABLED=0 静态二进制没有 /etc/resolv.conf，DNS 会全部失败：
#        pkg install -y golang clang
#        CGO_ENABLED=1 GOOS=android GOARCH=arm64 CC=clang \
#          go build -trimpath -ldflags="-s -w" -o caiyun-linux ./cmd/caiyun
#   3. 前端 dist 已 PC 构建并上传到 ~/caiyun/dist
# 可选环境变量：
#   CAIYUN_ORIGIN  允许跨域来源，默认 http://127.0.0.1:5701（经 nginx 访问时填面板地址）
set -euo pipefail
BASE=$HOME/caiyun
LOG=$BASE/logs
. "$BASE/secrets.env"
ORIGIN="${CAIYUN_ORIGIN:-http://127.0.0.1:5701}"
BIN="$BASE/caiyun-linux"

# ---------- .env ----------
cat > "$BASE/.env" <<EOF
APP_ENV=production
COOKIE_SECURE=false
PORT=8080
DB_HOST=127.0.0.1
DB_PORT=3306
DB_USER=caiyun_app
DB_PASSWORD=$DB_APP_PW
DB_NAME=caiyun
DB_AUTO_MIGRATE=false
TASK_CONFIG_SYNC_ON_STARTUP=false
REDIS_HOST=127.0.0.1
REDIS_PORT=6379
REDIS_PASSWORD=$REDIS_PW
RATE_LIMIT_BACKEND=redis
TASK_QUEUE_BACKEND=streams
TASK_QUEUE_STREAM_GROUP=caiyun-workers
DATA_ENCRYPTION_KEYS=v1=$DATA_KEY
DATA_ENCRYPTION_CURRENT_VERSION=v1
FIELD_CRYPTO_ALLOW_LEGACY_NO_AAD=false
JWT_SECRET=$JWT_SECRET
JWT_ISSUER=caiyun-api
JWT_AUDIENCE=caiyun-web
JWT_ACCESS_TTL=15m
JWT_REFRESH_TTL=720h
CLOCK_SKEW_MAX=30s
ALLOWED_ORIGINS=$ORIGIN,http://127.0.0.1:5701,http://localhost:5701
TRUSTED_PROXIES=127.0.0.1
WORKER_MONITOR_TOKEN=$MONITOR_TOKEN
WORKER_MONITOR_HOST=127.0.0.1
WORKER_MONITOR_PORT=8081
WORKER_MONITOR_ALLOW_PLAINTEXT=true
API_MONITOR_TOKEN=$MONITOR_TOKEN
LOG_FILE_PATH=$LOG
LOG_JSON_FORMAT=true
TZ=Asia/Shanghai
EXCHANGE_REQUEST_PACING_ENABLED=true
EOF
chmod 600 "$BASE/.env"
echo "[env] written"

# ---------- 二进制 ----------
if [ -f "$BASE/caiyun-arm64.new" ]; then
  mv "$BASE/caiyun-arm64.new" "$BIN"
fi
if [ ! -x "$BIN" ]; then
  echo "[bin] missing executable $BIN (see prerequisites in this script header)" >&2
  exit 1
fi
chmod +x "$BIN"
"$BIN" version >/dev/null || { echo "[bin] '$BIN version' FAILED" >&2; exit 1; }
echo "[bin] ok"

# ---------- 迁移（失败必须终止，不能让 tail 吞掉退出码）----------
cd "$BASE"
if ! "$BIN" migrate > "$LOG/migrate.log" 2>&1; then
  echo "[migrate] FAILED, see $LOG/migrate.log" >&2
  tail -20 "$LOG/migrate.log" >&2
  exit 1
fi
tail -5 "$LOG/migrate.log"
if ! "$BIN" migrate --validate-only > "$LOG/migrate-validate.log" 2>&1; then
  echo "[migrate] validate-only FAILED, see $LOG/migrate-validate.log" >&2
  tail -20 "$LOG/migrate-validate.log" >&2
  exit 1
fi
echo "[migrate] validated"

# ---------- 启动 API / Worker ----------
start_role() {
  local role=$1 port=$2
  local pidfile="$BASE/$role.pid"
  if [ -f "$pidfile" ] && kill -0 "$(cat "$pidfile")" 2>/dev/null; then
    echo "[$role] already running pid=$(cat "$pidfile")"
    return 0
  fi
  cd "$BASE"
  nohup "$BIN" "$role" >> "$LOG/$role.out" 2>&1 &
  echo $! > "$pidfile"
  local i
  for i in $(seq 1 30); do
    curl -fsS "http://127.0.0.1:$port/readyz" >/dev/null 2>&1 && { echo "[$role] ready on :$port"; return 0; }
    sleep 2
  done
  echo "[$role] NOT ready on :$port after 60s" >&2
  tail -15 "$LOG/$role.out" >&2
  return 1
}

rc=0
start_role api 8080 || rc=1
start_role worker 8081 || rc=1
if [ "$rc" -ne 0 ]; then
  echo "=== APP FAILED: one or more services not ready ===" >&2
  exit 1
fi
echo "=== APP DONE ==="
