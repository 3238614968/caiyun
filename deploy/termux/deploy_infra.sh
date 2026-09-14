#!/data/data/com.termux/files/usr/bin/bash
# caiyun 基础设施：Redis + MariaDB（Termux 原生，无 root）
# 所有密钥随机生成并保存到 ~/caiyun/secrets.env（chmod 600），重复执行不会覆盖。
set -u
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
if redis-cli -a "$REDIS_PW" -p 6379 ping 2>/dev/null | grep -q PONG; then
  echo "[redis] already running"
else
  redis-server --port 6379 --requirepass "$REDIS_PW" \
    --dir "$DATA/redis" --daemonize yes --save '900 1' \
    --logfile "$LOG/redis.log" --maxmemory 256mb --maxmemory-policy allkeys-lru
  sleep 1
  redis-cli -a "$REDIS_PW" -p 6379 ping 2>/dev/null | grep -q PONG \
    && echo "[redis] OK" || { echo "[redis] FAIL"; tail -5 "$LOG/redis.log"; exit 1; }
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
    > "$LOG/mariadb-install.log" 2>&1 || { echo "[mariadb] install FAIL"; tail -20 "$LOG/mariadb-install.log"; exit 1; }
  echo "[mariadb] datadir installed"
fi

if [ -f "$BASE/mysqld.pid" ] && "$MYSQLADMIN" --socket="$MYSQL_SOCK" -u root ping 2>/dev/null | grep -q alive; then
  echo "[mariadb] already running"
else
  nohup "$MYSQLD" --datadir="$MYSQL_DATA" --socket="$MYSQL_SOCK" --port=3306 \
    --pid-file="$BASE/mysqld.pid" --log-error="$LOG/mariadb.log" \
    >> "$LOG/mariadb.out" 2>&1 &
  for i in $(seq 1 15); do
    "$MYSQLADMIN" --socket="$MYSQL_SOCK" -u root ping 2>/dev/null | grep -q alive && break
    sleep 1
  done
  if "$MYSQLADMIN" --socket="$MYSQL_SOCK" -u root ping 2>/dev/null | grep -q alive; then
    echo "[mariadb] OK"
  else
    echo "[mariadb] FAIL"; tail -20 "$LOG/mariadb.log" 2>/dev/null; tail -20 "$LOG/mariadb.out" 2>/dev/null; exit 1
  fi
fi

# ---------- 建库建用户 ----------
"$MYSQL" --socket="$MYSQL_SOCK" -u root <<SQL
CREATE DATABASE IF NOT EXISTS caiyun CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'caiyun_app'@'localhost' IDENTIFIED BY '$DB_APP_PW';
CREATE USER IF NOT EXISTS 'caiyun_app'@'127.0.0.1' IDENTIFIED BY '$DB_APP_PW';
GRANT ALL PRIVILEGES ON caiyun.* TO 'caiyun_app'@'localhost';
GRANT ALL PRIVILEGES ON caiyun.* TO 'caiyun_app'@'127.0.0.1';
FLUSH PRIVILEGES;
SQL
echo "[mariadb] db/user ready"

# 应用用户连通性验证（TCP）
"$MYSQL" -h 127.0.0.1 -P 3306 -u caiyun_app -p"$DB_APP_PW" -e "SELECT 'app tcp ok';" \
  || { echo "[mariadb] app user TCP FAIL"; exit 1; }
echo "=== INFRA DONE ==="
