# CDN 推送传输配置（SSE 优先）

## 推荐策略

生产环境经过不支持 WebSocket Upgrade 的 CDN 时，使用显式的 SSE 配置，而不是先尝试 WebSocket 再自动降级：

```dotenv
VITE_PUSH_TRANSPORT=sse
VITE_SSE_URL=/events
```

`/events` 是携带 `auth_token` Cookie 的同源 `EventSource` 长连接；服务端发送 `text/event-stream`，每 3 秒发送 SSE 注释心跳（可通过 `SSE_HEARTBEAT_INTERVAL` 调整），Nginx 对该路径关闭缓冲和缓存。

开发环境或未经过 CDN 的独立源站可使用：

```dotenv
VITE_PUSH_TRANSPORT=ws
VITE_WS_URL=/ws
```

仅在同一份前端需要同时部署到 CDN 与直连源站、且无法在部署时注入环境变量时使用：

```dotenv
VITE_PUSH_TRANSPORT=auto
```

`auto` 会先连接 WS；连接关闭且未建立成功时切到 SSE。它会造成一次可预期的 Upgrade 失败和额外的建连延迟，因此不建议作为 CDN 生产默认值。

## 百度 CDN 配置检查

1. `/events` 配置为动态回源，禁止缓存；不要对它做 HTML/JSON 压缩、响应聚合或缓存。
2. 回源协议使用 HTTP/1.1；回源读超时不少于 `86400s`（或高于业务最长连接）。
3. 保留响应头 `Content-Type: text/event-stream`、`Cache-Control: no-cache, no-transform`、`X-Accel-Buffering: no`。
4. 若 CDN 有“响应缓冲/分块传输”开关，对 `/events` 关闭响应缓冲；否则客户端可能直到缓冲区写满才收到事件。
5. `/ws` 保留为直连源站、支持 WebSocket 的 CDN 产品或以后切换使用；当前 CDN 域名不应把客户端配置为 `ws`。

## 验证

登录后执行：

```bash
curl -N --http1.1 \
  -H 'Cookie: auth_token=<token>' \
  https://caiyun.apisky.cn/events
```

应立即看到 `retry: 3000`，随后每约 3 秒收到 `: ping`。浏览器网络面板中 `/events` 应为持续 Pending 的 `200` 请求，不再出现 `/ws` 的 `400 Upgrade header` 日志。
