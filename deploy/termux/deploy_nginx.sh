#!/data/data/com.termux/files/usr/bin/bash
# caiyun nginx：监听 5701，静态托管 dist + 反代 API/SSE/WS 到 8080
# 前置：pkg install -y nginx；前端 dist 已上传到 ~/caiyun/dist
set -u
PREFIX=/data/data/com.termux/files/usr
HOME_DIR=/data/data/com.termux/files/home
NGX=$HOME_DIR/caiyun/nginx
DIST=$HOME_DIR/caiyun/dist
mkdir -p "$NGX/logs" "$NGX/tmp" "$NGX/conf"

cat > "$NGX/conf/nginx.conf" <<EOF
pid $NGX/logs/nginx.pid;
error_log $NGX/logs/error.log warn;
worker_processes 1;
daemon on;
events { worker_connections 1024; }
http {
  include $PREFIX/etc/nginx/mime.types;
  default_type application/octet-stream;
  access_log $NGX/logs/access.log;
  client_body_temp_path $NGX/tmp;
  proxy_temp_path $NGX/tmp/proxy;
  fastcgi_temp_path $NGX/tmp/fastcgi;
  uwsgi_temp_path $NGX/tmp/uwsgi;
  scgi_temp_path $NGX/tmp/scgi;
  sendfile on;
  server {
    listen 5701;
    server_name _;
    root $DIST;
    index index.html;
    client_max_body_size 10m;

    location / { try_files \$uri /index.html; }
    location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)\$ { expires 30d; }

    location /api/ {
      proxy_pass http://127.0.0.1:8080;
      proxy_set_header Host \$host;
      proxy_set_header X-Real-IP \$remote_addr;
      proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
      proxy_set_header X-Forwarded-Proto \$scheme;
      proxy_read_timeout 60s;
    }
    location /health { proxy_pass http://127.0.0.1:8080/health; access_log off; }
    location = /events {
      proxy_pass http://127.0.0.1:8080/events;
      proxy_http_version 1.1;
      proxy_set_header Connection "";
      proxy_set_header Host \$host;
      proxy_set_header Accept-Encoding "";
      gzip off; proxy_buffering off; proxy_request_buffering off; proxy_cache off;
      add_header X-Accel-Buffering "no" always;
      proxy_read_timeout 86400s;
    }
    location /ws {
      proxy_pass http://127.0.0.1:8080/ws;
      proxy_http_version 1.1;
      proxy_set_header Upgrade \$http_upgrade;
      proxy_set_header Connection "upgrade";
      proxy_set_header Host \$host;
      proxy_read_timeout 86400s;
    }
  }
}
EOF

# 停掉旧实例（若存在）
if [ -f "$NGX/logs/nginx.pid" ] && kill -0 "$(cat "$NGX/logs/nginx.pid")" 2>/dev/null; then
  nginx -p "$NGX" -c "$NGX/conf/nginx.conf" -s quit 2>/dev/null; sleep 1
fi
nginx -p "$NGX" -c "$NGX/conf/nginx.conf" -t 2>&1 | tail -3 || { echo "[nginx] config FAIL"; exit 1; }
nginx -p "$NGX" -c "$NGX/conf/nginx.conf"
sleep 2
echo "[nginx] listening check:"
(echo >/dev/tcp/127.0.0.1/5701) 2>/dev/null && echo "5701 UP" || { echo "5701 DOWN"; tail -10 "$NGX/logs/error.log"; exit 1; }
echo "=== NGINX DONE ==="
