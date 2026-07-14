# Nginx、HTTPS 与实时推送部署

Nginx 负责 TLS 终止、前端静态文件、API 反向代理以及 SSE/WebSocket 长连接。仓库提供两套配置：

- `nginx-server.conf`：Linux 单机站点配置，默认静态目录为 `/www/wwwroot/caiyun`。
- `frontend/nginx.conf`：前端容器内部配置，通过容器 DNS `backend-api:8080` 转发请求。

## 1. 部署前配置

假设：

```text
域名：caiyun.example.com
静态目录：/www/wwwroot/caiyun
API：127.0.0.1:8080
Worker 监控：127.0.0.1:8081（不公开）
```

后端环境变量：

```dotenv
ALLOWED_ORIGINS=https://caiyun.example.com
TRUSTED_PROXIES=127.0.0.1
VITE_PUSH_TRANSPORT=sse
```

如果 Nginx 运行在独立主机、容器网关或负载均衡器后，`TRUSTED_PROXIES` 应填写实际代理源地址或 CIDR，不设置全网段信任。

## 2. HTTP 到 HTTPS

```nginx
server {
    listen 80;
    server_name caiyun.example.com;

    location /.well-known/acme-challenge/ {
        root /var/www/letsencrypt;
    }

    location / {
        return 301 https://$host$request_uri;
    }
}
```

## 3. HTTPS 站点模板

```nginx
server {
    listen 443 ssl http2;
    server_name caiyun.example.com;

    ssl_certificate /etc/letsencrypt/live/caiyun.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/caiyun.example.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 10m;

    root /www/wwwroot/caiyun;
    index index.html;
    client_max_body_size 10m;
    server_tokens off;

    add_header X-Content-Type-Options nosniff always;
    add_header X-Frame-Options DENY always;
    add_header Referrer-Policy strict-origin-when-cross-origin always;

    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_connect_timeout 10s;
        proxy_read_timeout 60s;
    }

    location = /events {
        proxy_pass http://127.0.0.1:8080/events;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Accept-Encoding "";

        gzip off;
        proxy_buffering off;
        proxy_request_buffering off;
        proxy_cache off;
        chunked_transfer_encoding on;
        tcp_nodelay on;

        add_header Cache-Control "no-store, no-cache, must-revalidate, max-age=0, no-transform" always;
        add_header X-Accel-Buffering "no" always;
        proxy_connect_timeout 10s;
        proxy_read_timeout 86400s;
        proxy_send_timeout 86400s;
        send_timeout 86400s;
    }

    location /ws {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
        proxy_buffering off;
    }

    location / {
        try_files $uri $uri/ /index.html;
    }
}
```

实际部署可直接基于 `nginx-server.conf` 修改。仓库文件包含具体域名和证书路径示例，复制到生产环境前必须替换。

## 4. 申请证书

使用 Certbot 的 Debian/Ubuntu 示例：

```bash
sudo apt install -y certbot python3-certbot-nginx
sudo certbot --nginx -d caiyun.example.com
sudo certbot renew --dry-run
systemctl list-timers | grep certbot
```

如果证书由云负载均衡器、Ingress 或 CDN 终止，源站仍应使用受限网络；需要端到端 TLS 时，为源站配置独立证书。

## 5. Docker 前端接入

Compose 前端默认监听 `127.0.0.1:80`，外层 Nginx 可直接代理整个站点：

```nginx
location / {
    proxy_pass http://127.0.0.1:80;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_buffering off;
}
```

若使用独立 Docker 指南中的 `127.0.0.1:8088`，将上游端口替换为 `8088`。前端容器会继续在内部处理 `/api`、`/events` 和 `/ws`。

## 6. SSE 验证

```bash
curl -i -N --http1.1 \
  -H 'Accept: text/event-stream' \
  -H 'Cookie: auth_token=<AUTH_TOKEN>' \
  https://caiyun.example.com/events
```

检查要点：

- 响应状态为 `200`。
- `Content-Type` 为 `text/event-stream`。
- 连接持续保持，周期性收到心跳注释。
- 响应头包含禁止缓存/缓冲相关设置。
- Nginx access log 中请求持续时间较长属于正常现象。

CDN 场景参见 [`CDN_SSE.md`](./CDN_SSE.md)。已确认 CDN 不支持 WebSocket 时，前端显式设置 `VITE_PUSH_TRANSPORT=sse`。

## 7. WebSocket 验证

浏览器开发者工具应看到 `/ws` 返回 `101 Switching Protocols`。命令行可使用支持 WebSocket 的客户端进行验证。出现 `400` 且缺少 Upgrade 头时，重点检查：

- 中间 CDN/负载均衡器是否支持 WebSocket。
- Nginx 是否传递 `Upgrade` 和 `Connection`。
- 前端构建时的 `VITE_WS_URL` 是否正确。
- HTTPS 页面是否使用 `wss://` 或同源相对路径。

## 8. 配置检查与重新加载

```bash
sudo nginx -t
sudo systemctl reload nginx
sudo systemctl status nginx --no-pager
sudo tail -f /var/log/nginx/access.log /var/log/nginx/error.log
```

重新加载前始终执行 `nginx -t`。证书、域名、静态目录或上游端口变更后，应同时验证首页、API、SSE 和 WebSocket。

## 9. CDN 与真实客户端 IP

CDN 接入后需要：

1. 在边缘层转发可信客户端 IP 头。
2. 在源站只信任 CDN 官方回源网段。
3. 配置 Nginx `real_ip_header` 和 `set_real_ip_from`。
4. 将应用 `TRUSTED_PROXIES` 限制为 Nginx 或网关地址。
5. 防火墙限制源站只接受 CDN/负载均衡器回源流量。

不得直接信任来自任意互联网客户端的 `X-Forwarded-For`。

## 10. 常见问题

| 现象 | 检查项 |
| --- | --- |
| 首页刷新返回 404 | `location /` 是否使用 `try_files ... /index.html` |
| API 返回 502 | API 是否监听 `127.0.0.1:8080`，Nginx 上游是否一致 |
| SSE 延迟或批量到达 | 关闭 `proxy_buffering`、gzip、缓存和请求缓冲 |
| WebSocket 返回 400 | Upgrade 头、CDN 能力和 `VITE_WS_URL` |
| 登录后跨域失败 | `ALLOWED_ORIGINS` 是否包含完整 HTTPS Origin |
| 获取到错误客户端 IP | `TRUSTED_PROXIES` 与 Nginx real IP 配置是否匹配 |
