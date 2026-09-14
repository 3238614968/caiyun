#!/data/data/com.termux/files/usr/bin/bash
# 注册管理员并提权（幂等：已存在则跳过注册步骤，仅确保 role=admin）
# 用法：CAIYUN_ADMIN_USER=admin CAIYUN_ADMIN_PASS='你的密码' bash create_admin.sh
# 密码须满足后端策略：≥8 位且含字母和数字。
set -u
BASE=$HOME/caiyun
MYSQL=$(command -v mariadb || echo mysql)
USER="${CAIYUN_ADMIN_USER:-admin}"
PASS="${CAIYUN_ADMIN_PASS:?请通过环境变量 CAIYUN_ADMIN_PASS 设置管理员密码}"

# 通过 API 注册（bcrypt 由后端生成）
code=$(curl -s -o /tmp/reg.json -w '%{http_code}' -X POST http://127.0.0.1:8080/api/auth/register \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"$USER\",\"password\":\"$PASS\",\"email\":\"admin@caiyun.local\"}")
echo "[register] http=$code $(head -c 300 /tmp/reg.json)"

# 提权为 admin
"$MYSQL" --socket="$BASE/mysqld.sock" -u root caiyun -e \
  "UPDATE users SET role='admin' WHERE username='$USER'; SELECT id,username,role FROM users WHERE username='$USER';"

# 登录验证
code2=$(curl -s -o /tmp/login.json -w '%{http_code}' -X POST http://127.0.0.1:8080/api/auth/login \
  -H 'Content-Type: application/json' -H "Origin: http://127.0.0.1:5701" \
  -d "{\"username\":\"$USER\",\"password\":\"$PASS\"}")
echo "[login] http=$code2 $(head -c 200 /tmp/login.json)"
echo "=== ADMIN DONE ==="
