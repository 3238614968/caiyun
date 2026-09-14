#!/data/data/com.termux/files/usr/bin/bash
# caiyun 一键启动（幂等，可重复执行）
# 用途：日常恢复服务；配合 Termux:Boot 实现开机自启——
#   mkdir -p ~/.termux/boot
#   echo 'bash $HOME/caiyun/start_all.sh >> $HOME/caiyun/logs/boot.log 2>&1' \
#     > ~/.termux/boot/00-caiyun.sh
#   chmod +x ~/.termux/boot/00-caiyun.sh
# 注意：Android 8+ 需手动打开过一次 Termux:Boot；国产 ROM 还需在系统设置里
#   给 Termux / Termux:Boot 放开「自启动」和「后台耗电不优化」，否则开机广播会被拦截。
set -u
BASE=$HOME/caiyun
LOG=$BASE/logs
. "$BASE/secrets.env"

MYSQLD=$(command -v mariadbd || command -v mysqld)
MYSQLADMIN=$(command -v mariadb-admin || echo mysqladmin)
BIN="$BASE/caiyun-linux"

# ---- Redis ----
redis-cli -a "$REDIS_PW" -p 6379 ping 2>/dev/null | grep -q PONG || {
  redis-server --port 6379 --requirepass "$REDIS_PW" --dir "$BASE/data/redis" \
    --daemonize yes --save '900 1' --logfile "$LOG/redis.log" \
    --maxmemory 256mb --maxmemory-policy allkeys-lru
  echo "[redis] started"
}

# ---- MariaDB ----
if ! "$MYSQLADMIN" --socket="$BASE/mysqld.sock" -u root ping 2>/dev/null | grep -q alive; then
  nohup "$MYSQLD" --datadir="$BASE/data/mysql" --socket="$BASE/mysqld.sock" --port=3306 \
    --pid-file="$BASE/mysqld.pid" --log-error="$LOG/mariadb.log" >> "$LOG/mariadb.out" 2>&1 &
  for i in $(seq 1 20); do
    "$MYSQLADMIN" --socket="$BASE/mysqld.sock" -u root ping 2>/dev/null | grep -q alive && break
    sleep 1
  done
  echo "[mariadb] started"
fi

# ---- API / Worker ----
start_role() {
  local role=$1
  local port=$2
  local pidfile="$BASE/$role.pid"
  if [ -f "$pidfile" ] && kill -0 "$(cat "$pidfile")" 2>/dev/null; then return; fi
  if curl -fsS "http://127.0.0.1:$port/readyz" >/dev/null 2>&1; then return; fi
  cd "$BASE"
  nohup "$BIN" "$role" >> "$LOG/$role.out" 2>&1 &
  echo $! > "$pidfile"
  sleep 3
  curl -fsS "http://127.0.0.1:$port/readyz" >/dev/null 2>&1 \
    && echo "[$role] started :$port" || { echo "[$role] FAIL"; tail -10 "$LOG/$role.out"; }
}
start_role api 8080
start_role worker 8081

# ---- nginx :5701 ----
if ! (echo >/dev/tcp/127.0.0.1/5701) 2>/dev/null; then
  nginx -p "$BASE/nginx" -c "$BASE/nginx/conf/nginx.conf" && echo "[nginx] started :5701"
fi

# ---- 保活锁（防止 Termux 被系统回收）----
command -v termux-wake-lock >/dev/null 2>&1 && termux-wake-lock 2>/dev/null

echo "[caiyun] all up: http://127.0.0.1:5701"
