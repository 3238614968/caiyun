# 百度智能云 CDN 与 WebSocket 部署

## 结论

百度智能云普通 CDN 适合静态/HTTP 缓存分发；需要长期双向连接时，应使用 **全站加速 DRCDN** 的 WebSocket 加速能力，或让 `/ws` 走不接入 CDN 的独立源站域名。普通 CDN 节点没有将浏览器的 `Upgrade: websocket` 透传到源站时，Go API 会返回 `400`，日志会出现：

```text
websocket: the client is not using the websocket protocol
```

## 方案 A：将主域名切换为 DRCDN（推荐）

在百度智能云控制台为业务域名启用/切换到 DRCDN，并配置：

1. 源站为当前 Nginx 公网地址，回源协议与源站一致；HTTPS 源站使用 HTTPS 回源。
2. 将 `/ws` 配为动态路径，**不缓存**、不进行 URL 参数忽略或重写。
3. 在 WebSocket/协议加速能力中开启 WebSocket；若控制台提供回源协议版本选项，选择 HTTP/1.1。
4. 保留 `/api/*` 为动态不缓存路径；不要给 `/ws` 添加重定向、鉴权挑战页或页面规则。
5. 等待配置全网生效后，使用浏览器 DevTools 的 Network → WS 验证响应状态为 `101 Switching Protocols`。

源站 Nginx 的 `/ws` 块已在 `nginx-server.conf` 中明确透传 `Upgrade`/`Connection`，并已关闭代理缓冲和缓存。

## 方案 B：WebSocket 走独立源站域名

如果现有域名必须保留普通 CDN：

1. 创建如 `ws.example.com` 的 DNS 记录，直接 A/AAAA 到 Nginx 源站，**不 CNAME 到 CDN**。
2. 为该子域配置 TLS 证书，并在同一 Nginx server 中保留 `/ws` 反代配置。
3. 前端构建环境设置：

```dotenv
VITE_WS_URL=wss://ws.example.com/ws
```

4. 将前端 CSP 的 `connect-src` 添加 `wss://ws.example.com`；生产根 Nginx 也要同步允许该域名。
5. 重新构建前端并发布。

## 源站验证

在已登录浏览器中，DevTools → Network → WS 的请求头必须包含：

```http
Connection: Upgrade
Upgrade: websocket
Sec-WebSocket-Version: 13
Sec-WebSocket-Key: ...
```

响应必须为：

```http
HTTP/1.1 101 Switching Protocols
```

可在源站检查实际生效配置：

```bash
nginx -T | grep -n -A20 -B2 'location /ws'
nginx -t && systemctl reload nginx
```

若源站直连为 `101`、经 CDN 为 `400`，问题即位于 CDN 产品/路径规则，不在 Go WebSocket 服务。
