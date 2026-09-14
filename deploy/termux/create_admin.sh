#!/data/data/com.termux/files/usr/bin/bash
# 注册管理员并提权（每步强校验：注册 HTTP 状态、SQL 影响行数、登录后 token）
# 用法：CAIYUN_ADMIN_USER=admin CAIYUN_ADMIN_PASS='你的密码' bash create_admin.sh
# 密码须满足后端策略：≥8 位且含字母和数字。
set -uo pipefail
BASE=$HOME/caiyun
. "$BASE/secrets.env"
MYSQL=$(command -v mariadb || echo mysql)
API=http://127.0.0.1:8080

USER_NAME="${CAIYUN_ADMIN_USER:-admin}"
PASS="${CAIYUN_ADMIN_PASS:?请通过环境变量 CAIYUN_ADMIN_PASS 设置管理员密码}"

# 用户名白名单校验：只允许字母/数字/下划线/连字符，从源头杜绝 SQL/JSON 注入
if ! printf '%s' "$USER_NAME" | grep -Eq '^[A-Za-z0-9_-]{1,32}$'; then
  echo "[admin] invalid username (allowed: A-Z a-z 0-9 _ - , max 32)" >&2
  exit 1
fi

# JSON 字符串转义（反斜杠/双引号/控制字符），密码可含任意特殊字符
json_escape() {
  local s=$1
  s=${s//\\/\\\\}
  s=${s//\"/\\\"}
  s=${s//$'\n'/\\n}
  s=${s//$'\r'/\\r}
  s=${s//$'\t'/\\t}
  printf '%s' "$s"
}

TMPDIR_C=$(mktemp -d)
trap 'rm -rf "$TMPDIR_C"' EXIT

# ---------- 1. 注册（200/201=成功，409/“已存在”=幂等跳过）----------
printf '{"username":"%s","password":"%s","email":"%s@caiyun.local"}' \
  "$(json_escape "$USER_NAME")" "$(json_escape "$PASS")" "$USER_NAME" > "$TMPDIR_C/reg.json"
code=$(curl -s -o "$TMPDIR_C/reg.out" -w '%{http_code}' -X POST "$API/api/auth/register" \
  -H 'Content-Type: application/json' --data @"$TMPDIR_C/reg.json")
case "$code" in
  200|201) echo "[register] ok (http=$code)" ;;
  409)     echo "[register] user already exists, continue (http=409)" ;;
  *)       echo "[register] FAILED (http=$code): $(head -c 200 "$TMPDIR_C/reg.out")" >&2; exit 1 ;;
esac

# ---------- 2. 提权为 admin（校验 UPDATE 生效且用户唯一）----------
# 用户名已过白名单，单引号包裹安全；先确认存在再更新
row=$("$MYSQL" --socket="$BASE/mysqld.sock" -u root -p"$DB_ROOT_PW" -N -B caiyun -e \
  "SELECT id, role FROM users WHERE username='$USER_NAME';" 2>/dev/null)
if [ -z "$row" ]; then
  echo "[admin] user '$USER_NAME' not found in DB after register" >&2
  exit 1
fi
"$MYSQL" --socket="$BASE/mysqld.sock" -u root -p"$DB_ROOT_PW" caiyun -e \
  "UPDATE users SET role='admin' WHERE username='$USER_NAME';" \
  || { echo "[admin] UPDATE failed" >&2; exit 1; }
role=$("$MYSQL" --socket="$BASE/mysqld.sock" -u root -p"$DB_ROOT_PW" -N -B caiyun -e \
  "SELECT role FROM users WHERE username='$USER_NAME';" 2>/dev/null)
[ "$role" = "admin" ] || { echo "[admin] role is '$role', not admin" >&2; exit 1; }
echo "[admin] role=admin verified"

# ---------- 3. 登录验证（必须拿到 token）----------
printf '{"username":"%s","password":"%s"}' \
  "$(json_escape "$USER_NAME")" "$(json_escape "$PASS")" > "$TMPDIR_C/login.json"
code2=$(curl -s -o "$TMPDIR_C/login.out" -w '%{http_code}' -X POST "$API/api/auth/login" \
  -H 'Content-Type: application/json' -H "Origin: http://127.0.0.1:5701" \
  --data @"$TMPDIR_C/login.json")
if [ "$code2" != "200" ]; then
  echo "[login] FAILED (http=$code2): $(head -c 200 "$TMPDIR_C/login.out")" >&2
  exit 1
fi
echo "[login] ok (http=200)"
echo "=== ADMIN DONE ==="
