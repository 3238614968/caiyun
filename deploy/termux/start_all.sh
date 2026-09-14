#!/data/data/com.termux/files/usr/bin/bash
# caiyun 一键启动（幂等，可重复执行；任一服务未就绪则以非零退出）
# 用途：日常恢复服务；配合 Termux:Boot 实现开机自启——
#   mkdir -p ~/.termux/boot
#   echo 'bash $HOME/caiyun/start_all.sh >> $HOME/caiyun/logs/boot.log 2>&1' \
#     > ~/.termux/boot/00-caiyun.sh
#   chmod +x ~/.termux/boot/00-caiyun.sh
# 注意：Android 8+ 需手动打开过一次 Termux:Boot；国产 ROM 还需在系统设置里
#   给 Termux / Termux:Boot 放开「自启动」和「后台耗电不优化」，否则开机广播会被拦截。
set -uo pipefail
BASE=$HOME/caiyun
LOG=$BASE/logs
. "$BASE/secrets.env"

MYSQLD=$(command -v mariadbd || command -v mysqld)
MYSQLADMIN=$(command -v mariadb-admin || echo mysqladmin)
MYSQL_SOCK="$BASE/mysqld.sock"
BIN="$BASE/caiyun-linux"

# ---- Redis（noeviction：队列/锁/会话键不可被 LRU 淘汰，见 deploy_infra.sh 注释）----
redis-cli -a "$REDIS_PW" -p 6379 ping 2>/dev/null | grep -q PONG || {
  redis-server --port 6379 --requirepass "$REDIS_PW" --dir "$BASE/data/redis" \
    --daemonize yes --save '900 1' --logfile "$LOG/redis.log" \
    --maxmemory "${REDIS_MAXMEMORY:-256mb}" --maxmemory-policy noeviction
  echo "[redis] started"
}

# ---- MariaDB（root 已设密码，探测必须带凭据）----
if ! "$MYSQLADMIN" --socket="$MYSQL_SOCK" -u root -p"$DB_ROOT_PW" ping 2>/dev/null | grep -q alive; then
  nohup "$MYSQLD" --datadir="$BASE/data/mysql" --socket="$MYSQL_SOCK" --port=3306 \
    --pid-file="$BASE/mysqld.pid" --log-error="$LOG/mariadb.log" >> "$LOG/mariadb.out" 2>&1 &
  for i in $(seq 1 20); do
    "$MYSQLADMIN" --socket="$MYSQL_SOCK" -u root -p"$DB_ROOT_PW" ping 2>/dev/null | grep -q alive && break
    sleep 1
  done
  if "$MYSQLADMIN" --socket="$MYSQL_SOCK" -u root -p"$DB_ROOT_PW" ping 2>/dev/null | grep -q alive; then
    echo "[mariadb] started"
  else
    echo "[mariadb] FAIL to start" >&2
    tail -10 "$LOG/mariadb.log" >&2 2>/dev/null || true
  fi
fi

# ---- API / Worker ----
start_role() {
  local role=$1
  local port=$2
  local pidfile="$BASE/$role.pid"
  if [ -f "$pidfile" ] && kill -0 "$(cat "$pidfile")" 2>/dev/null; then return 0; fi
  if curl -fsS "http://127.0.0.1:$port/readyz" >/dev/null 2>&1; then return 0; fi
  cd "$BASE"
  nohup "$BIN" "$role" >> "$LOG/$role.out" 2>&1 &
  echo $! > "$pidfile"
  local i
  for i in $(seq 1 30); do
    curl -fsS "http://127.0.0.1:$port/readyz" >/dev/null 2>&1 && { echo "[$role] started :$port"; return 0; }
    sleep 2
  done
  echo "[$role] NOT ready on :$port after 60s" >&2
  tail -10 "$LOG/$role.out" >&2
  return 1
}

rc=0
start_role api 8080 || rc=1
start_role worker 8081 || rc=1

# ---- nginx :5701 ----
if ! (echo >/dev/tcp/127.0.0.1/5701) 2>/dev/null; then
  nginx -p "$BASE/nginx" -c "$BASE/nginx/conf/nginx.conf" \
    && echo "[nginx] started :5701" \
    || { echo "[nginx] FAIL" >&2; rc=1; }
fi

# ---- 保活锁（防止 Termux 被系统回收）----
command -v termux-wake-lock >/dev/null 2>&1 && termux-wake-lock 2>/dev/null

if [ "$rc" -ne 0 ]; then
  echo "[caiyun] STARTUP FAILED: one or more services not ready" >&2
  exit 1
fi
echo "[caiyun] all up: http://127.0.0.1:5701"
