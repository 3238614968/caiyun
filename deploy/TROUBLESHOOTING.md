# 故障排查手册

排查时先确认故障范围，再按“入口代理 → 前端 → API → MySQL/Redis → Worker → 外部接口”逐层定位。所有操作应记录时间、版本、请求 ID 和变更内容，避免在问题现场直接覆盖日志或删除数据。

## 1. 快速采集状态

### Docker Compose

```bash
docker compose ps
docker compose logs --since=30m --tail=500 backend-api backend-worker frontend mysql redis
docker stats --no-stream
docker system df
```

### Linux 单机

```bash
sudo systemctl status caiyun-api caiyun-worker nginx --no-pager
sudo journalctl -u caiyun-api -u caiyun-worker --since '-30 min' --no-pager
sudo nginx -t
ss -lntp | grep -E ':80|:443|:8080|:8081|:3306|:6379'
df -h
free -h
```

### Kubernetes

```bash
kubectl -n caiyun get pods,svc,job,pvc -o wide
kubectl -n caiyun get events --sort-by=.lastTimestamp
kubectl -n caiyun logs deployment/backend-api --since=30m --tail=500
kubectl -n caiyun logs deployment/backend-worker --since=30m --tail=500
kubectl -n caiyun describe pod POD_NAME
```

采集日志时先脱敏密码、Token、Cookie、手机号和加密密钥。

## 2. 常见症状速查

| 症状 | 优先检查 |
| --- | --- |
| 页面打不开 | DNS、TLS、Nginx/Ingress、前端容器或服务 |
| 502/504 | API 是否监听、上游地址、超时、网络策略 |
| 登录失败/401 | JWT 密钥、时钟、Cookie、CORS、数据库用户状态 |
| API 启动失败 | `.env`、数据库迁移、端口冲突、密钥长度 |
| Worker 不执行任务 | Redis、队列积压、Worker 日志、并发配置、外部接口 |
| SSE 无实时更新 | 代理缓冲、读超时、CDN 缓存、连接数 |
| WebSocket 断开 | Upgrade 头、HTTP/1.1、空闲超时、负载均衡 |
| 数据库连接耗尽 | 连接池、慢查询、异常重试、MySQL `max_connections` |
| Pod Pending | PVC、资源不足、节点选择、镜像拉取凭据 |
| Pod CrashLoopBackOff | 配置、Secret、迁移、权限、只读文件系统 |

## 3. API 无法启动

1. 检查配置文件位置。二进制会从当前工作目录和可执行文件目录加载 `.env`。
2. 确认 `JWT_SECRET`、`DATA_ENCRYPTION_KEYS`、数据库和 Redis 密码已设置。
3. 检查端口是否被占用。
4. 手动执行配置与迁移验证。

```bash
cd /www/wwwroot/caiyun
./caiyun-linux migrate --validate-only
./caiyun-linux api
```

若提示加密密钥错误，活动密钥必须是 32 字节原始值，或解码后为 32 字节的 Base64/Hex 值；版本化格式示例为 `v1=KEY`。

## 4. MySQL 与迁移故障

```bash
mysqladmin -h DB_HOST -u DB_USER -p ping
mysql -h DB_HOST -u DB_USER -p -e 'SELECT VERSION(); SHOW PROCESSLIST;'
./caiyun-linux migrate --validate-only
```

重点检查：

- 数据库地址、端口、账号、库名和字符集。
- 应用账号是否具备迁移所需的 DDL/DML 权限。
- MySQL 磁盘是否已满，是否出现锁等待或慢查询。
- 多个发布流程是否同时执行迁移。

迁移 Job 失败时保留日志，不要绕过迁移直接启动新版本。

## 5. Redis 与队列故障

```bash
redis-cli -h REDIS_HOST -p REDIS_PORT -a REDIS_PASSWORD PING
redis-cli -h REDIS_HOST -p REDIS_PORT -a REDIS_PASSWORD INFO memory
redis-cli -h REDIS_HOST -p REDIS_PORT -a REDIS_PASSWORD INFO persistence
```

检查连接密码、内存上限、AOF 状态、网络延迟和键过期策略。Worker 无消费时，同时查看 Worker 日志和队列迁移说明：[`../backend/docs/REDIS_STREAMS_QUEUE_MIGRATION.md`](../backend/docs/REDIS_STREAMS_QUEUE_MIGRATION.md)。

不要在生产环境直接执行 `FLUSHALL`、批量删除队列键或修改消费组状态；先备份并确认任务重放策略。

## 6. 502、504 与代理问题

从主机本地绕过代理测试 API：

```bash
curl -v http://127.0.0.1:8080/health
curl -v http://127.0.0.1:8081/health
```

若本地正常而域名异常：

```bash
sudo nginx -t
sudo tail -n 200 /var/log/nginx/error.log
curl -vk https://cloud.example.com/health
```

确认 Nginx/Ingress 的上游名称和端口与部署方式一致。主机 Nginx 通常指向 `127.0.0.1:8080`；Docker 前端容器指向 `backend-api:8080`；Kubernetes 入口指向 `frontend` Service。

## 7. SSE 实时事件中断

- 关闭代理缓冲和响应缓存。
- 提高 `proxy_read_timeout`，避免长连接被默认超时关闭。
- 确认 CDN 不缓存 `/events`，并支持长连接回源。
- 检查浏览器连接数、负载均衡空闲超时和 API 重启次数。

```bash
curl -N -H 'Accept: text/event-stream' https://cloud.example.com/events
```

详细配置见 [`CDN_SSE.md`](CDN_SSE.md) 和 [`NGINX_TLS.md`](NGINX_TLS.md)。

## 8. WebSocket 连接失败

代理必须使用 HTTP/1.1，并传递 `Upgrade` 与 `Connection` 头。检查浏览器 Network 面板是否收到 `101 Switching Protocols`：

```bash
curl -i \
  -H 'Connection: Upgrade' \
  -H 'Upgrade: websocket' \
  https://cloud.example.com/ws
```

若经 CDN 后失败但直连正常，检查 CDN WebSocket 开关、回源协议和连接时长。百度云配置见 [`BAIDU_CDN_WEBSOCKET.md`](BAIDU_CDN_WEBSOCKET.md)。

## 9. CORS、Cookie 与 401/403

- `ALLOWED_ORIGINS` 必须使用完整协议与域名，如 `https://cloud.example.com`。
- HTTPS 环境应设置安全 Cookie，代理需正确传递 `Host` 和 `X-Forwarded-Proto`。
- 多实例必须使用一致的 `JWT_SECRET` 和加密密钥。
- 校准应用、数据库和用户终端时钟。
- 只将可信代理加入 `TRUSTED_PROXIES`。

浏览器预检可用以下命令验证：

```bash
curl -i -X OPTIONS https://cloud.example.com/api/health \
  -H 'Origin: https://cloud.example.com' \
  -H 'Access-Control-Request-Method: GET'
```

## 10. Worker 积压或重复任务

1. 记录积压开始时间和受影响任务类型。
2. 检查 Redis、外部移动云盘接口和数据库延迟。
3. 检查 Worker 是否频繁重启、超时或触发限流。
4. 先降低并发控制失败扩散，再按处理能力逐步扩容。
5. 确认任务幂等键、重试次数和失败队列状态。

Kubernetes 临时扩容示例：

```bash
kubectl -n caiyun scale deployment/backend-worker --replicas=4
kubectl -n caiyun logs deployment/backend-worker -f
```

扩容前确认外部接口配额和数据库容量，避免把下游故障放大。

## 11. 资源与磁盘问题

```bash
df -h
du -sh /var/log/* 2>/dev/null | sort -h
docker system df
kubectl top pod -n caiyun
```

检查 MySQL 数据目录、Redis AOF、容器日志、发布备份和 Nginx 日志。清理前先确认文件用途与备份状态；生产数据库卷和当前发布目录不得作为普通缓存清理。

## 12. 恢复服务后的确认

- 记录根因、触发条件、修复和回滚点。
- 验证健康检查、登录、任务创建、事件推送和 Worker 消费。
- 观察至少一个完整任务周期的错误率、延迟和队列。
- 补充监控、测试、容量或发布门禁，避免同类问题重复发生。
- 涉及数据恢复时按 [`BACKUP_RESTORE.md`](BACKUP_RESTORE.md) 完成一致性核对。