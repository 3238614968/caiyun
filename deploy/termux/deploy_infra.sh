#!/data/data/com.termux/files/usr/bin/bash
# caiyun 基础设施：Redis + MariaDB（Termux 原生，无 root）
# 所有密钥随机生成并保存到 ~/caiyun/secrets.env（chmod 600），重复执行不会覆盖。
set -uo pipefail
BASE=$HOME/caiyun
LOG=$BASE/logs
DATA=$BASE/data
mkdir -p "$BASE" "$LOG" "$DATA/redis" "$DATA/mysql"

# ---------- 密钥（首次生成后固定，勿重复运行覆盖）----------
SECRETS=$BASE/secrets.env
if [ ! -f "$SECRETS" ]; then
  gen() { head -c 48 /dev/urandom | base64 | tr -d '/+=' | head -c "$1"; }
  cat > "$SECRETS" <<EOF
DB_ROOT_PW=$(gen 24)
DB_APP_PW=$(gen 24)
REDIS_PW=$(gen 24)
JWT_SECRET=$(gen 48)
DATA_KEY=$(gen 32)
MONITOR_TOKEN=$(gen 24)
EOF
  chmod 600 "$SECRETS"
  echo "[secrets] generated -> $SECRETS"
else
  echo "[secrets] reuse existing"
fi
. "$SECRETS"

# ---------- Redis ----------
# 队列（Streams）、锁与会话键都在本实例，必须 noeviction：
# allkeys-lru 会在内存达到上限时淘汰 stream/pending/锁键，导致任务静默丢失或重复执行。
# 内存超限时 Redis 返回 OOM 错误而非丢键，由应用侧感知并告警。
REDIS_MAXMEMORY=${REDIS_MAXMEMORY:-256mb}
if redis-cli -a "$REDIS_PW" -p 6379 ping 2>/dev/null | grep -q PONG; then
  echo "[redis] already running, enforcing noeviction"
  redis-cli -a "$REDIS_PW" -p 6379 config set maxmemory-policy noeviction >/dev/null 2>&1 \
    || echo "[redis] WARN: failed to enforce noeviction on running instance" >&2
else
  redis-server --port 6379 --requirepass "$REDIS_PW" \
    --dir "$DATA/redis" --daemonize yes --save '900 1' \
    --logfile "$LOG/redis.log" --maxmemory "$REDIS_MAXMEMORY" \
    --maxmemory-policy noeviction
  sleep 1
  redis-cli -a "$REDIS_PW" -p 6379 ping 2>/dev/null | grep -q PONG \
    || { echo "[redis] FAIL"; tail -5 "$LOG/redis.log" >&2; exit 1; }
  echo "[redis] OK (noeviction)"
fi

# ---------- MariaDB ----------
MYSQLD=$(command -v mariadbd || command -v mysqld)
MYSQL=$(command -v mariadb || echo mysql)
MYSQLADMIN=$(command -v mariadb-admin || echo mysqladmin)
MYSQL_DATA="$DATA/mysql"
MYSQL_SOCK="$BASE/mysqld.sock"

if [ ! -d "$MYSQL_DATA/mysql" ]; then
  echo "[mariadb] installing datadir"
  mariadb-install-db --datadir="$MYSQL_DATA" --auth-root-authentication-method=normal \
    > "$LOG/mariadb-install.log" 2>&1 \
    || { echo "[mariadb] install FAIL"; tail -20 "$LOG/mariadb-install.log" >&2; exit 1; }
  echo "[mariadb] datadir installed"
fi

mysql_root_ok() {
  "$MYSQLADMIN" --socket="$MYSQL_SOCK" -u root -p"$DB_ROOT_PW" ping 2>/dev/null | grep -q alive
}
mysql_root_anon_ok() {
  "$MYSQLADMIN" --socket="$MYSQL_SOCK" -u root ping 2>/dev/null | grep -q alive
}

if [ -f "$BASE/mysqld.pid" ] && kill -0 "$(cat "$BASE/mysqld.pid")" 2>/dev/null && mysql_root_ok; then
  echo "[mariadb] already running"
else
  nohup "$MYSQLD" --datadir="$MYSQL_DATA" --socket="$MYSQL_SOCK" --port=3306 \
    --pid-file="$BASE/mysqld.pid" --log-error="$LOG/mariadb.log" \
    >> "$LOG/mariadb.out" 2>&1 &
  for i in $(seq 1 15); do
    mysql_root_ok && break
    sleep 1
  done
  mysql_root_ok || { echo "[mariadb] FAIL"; tail -20 "$LOG/mariadb.log" >&2; tail -20 "$LOG/mariadb.out" >&2; exit 1; }
  echo "[mariadb] started"
fi

# ---------- 设置 root 密码（首次部署后必须消除空密码 root）----------
# mariadb-install-db 创建的 root@localhost 初始为空密码；这里在 socket 本地会话内
# 立即设为 secrets 中的随机密码，并覆盖 127.0.0.1/::1 以便 TCP 连接同样受保护。
if mysql_root_anon_ok && ! mysql_root_ok; then
  echo "[mariadb] setting root password"
  "$MYSQL" --socket="$MYSQL_SOCK" -u root <<SQL || { echo "[mariadb] set root password FAIL" >&2; exit 1; }
ALTER USER 'root'@'localhost' IDENTIFIED BY '$DB_ROOT_PW';
CREATE USER IF NOT EXISTS 'root'@'127.0.0.1' IDENTIFIED BY '$DB_ROOT_PW';
GRANT ALL PRIVILEGES ON *.* TO 'root'@'127.0.0.1' WITH GRANT OPTION;
CREATE USER IF NOT EXISTS 'root'@'::1' IDENTIFIED BY '$DB_ROOT_PW';
GRANT ALL PRIVILEGES ON *.* TO 'root'@'::1' WITH GRANT OPTION;
FLUSH PRIVILEGES;
SQL
  mysql_root_ok || { echo "[mariadb] root password verification FAIL" >&2; exit 1; }
  echo "[mariadb] root password set and verified"
elif ! mysql_root_ok; then
  echo "[mariadb] cannot authenticate as root with DB_ROOT_PW" >&2
  exit 1
fi

# ---------- 建库建用户 ----------
"$MYSQL" --socket="$MYSQL_SOCK" -u root -p"$DB_ROOT_PW" <<SQL || { echo "[mariadb] db/user setup FAIL" >&2; exit 1; }
CREATE DATABASE IF NOT EXISTS caiyun CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'caiyun_app'@'localhost' IDENTIFIED BY '$DB_APP_PW';
CREATE USER IF NOT EXISTS 'caiyun_app'@'127.0.0.1' IDENTIFIED BY '$DB_APP_PW';
GRANT ALL PRIVILEGES ON caiyun.* TO 'caiyun_app'@'localhost';
GRANT ALL PRIVILEGES ON caiyun.* TO 'caiyun_app'@'127.0.0.1';
FLUSH PRIVILEGES;
SQL
echo "[mariadb] db/user ready"

# 应用用户连通性验证（TCP）
"$MYSQL" -h 127.0.0.1 -P 3306 -u caiyun_app -p"$DB_APP_PW" -e "SELECT 'app tcp ok';" >/dev/null \
  || { echo "[mariadb] app user TCP FAIL" >&2; exit 1; }
echo "=== INFRA DONE ==="
