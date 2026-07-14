# Docker Compose 部署

Docker Compose 适合本地验证、预发布和单机部署。仓库的 `docker-compose.yml` 会构建前后端镜像，并启动 MySQL、Redis、数据库迁移、API、Worker、前端 Nginx 和 Grafana。

## 1. 部署拓扑

```text
Browser -> 127.0.0.1:80 (frontend/nginx) -> backend-api:8080
                                      -> /events 或 /ws
backend-worker -> Redis queue -> MySQL
backend-migrate -> MySQL（一次性发布门禁）
Grafana -> 127.0.0.1:3000
```

宿主机端口均绑定到 `127.0.0.1`：

| 服务 | 宿主机端口 | 容器端口 | 持久化卷 |
| --- | --- | --- | --- |
| Frontend | `127.0.0.1:80` | `8080` | 无 |
| API | `127.0.0.1:8080` | `8080` | 无 |
| Worker 监控 | `127.0.0.1:8081` | `8081` | 无 |
| MySQL | `127.0.0.1:3306` | `3306` | `mysql-data` |
| Redis | `127.0.0.1:6379` | `6379` | `redis-data` |
| Grafana | `127.0.0.1:3000` | `3000` | `grafana-data` |

公网访问应通过宿主机 Nginx、负载均衡器或隧道服务转发，不应直接修改数据库和 Redis 为全网监听。

## 2. 前置条件

- Linux x86_64/arm64 主机；Windows/macOS 可用于开发验证。
- Docker Engine 与 Docker Compose v2。
- 建议起步资源：2 vCPU、4 GiB 内存、20 GiB 可用磁盘；实际容量按账号数、任务并发、日志和数据库增长调整。
- 服务器时间与时区正确，建议启用 NTP。

验证环境：

```bash
docker version
docker compose version
docker info
```

## 3. 准备生产配置

```bash
cd /opt
git clone <REPOSITORY_URL> caiyun
cd caiyun
umask 077
cp .env.example .env
```

生成随机值：

```bash
openssl rand -hex 24       # 数据库、Redis、Grafana 密码
openssl rand -base64 48    # JWT_SECRET、WORKER_MONITOR_TOKEN
openssl rand -base64 32    # 32 字节数据加密密钥
```

将生成值写入 `.env`，至少修改：

```dotenv
APP_ENV=production
MYSQL_ROOT_PASSWORD=<mysql-root-password>
MYSQL_USER=caiyun_app
MYSQL_PASSWORD=<mysql-app-password>
REDIS_PASSWORD=<redis-password>
JWT_SECRET=<jwt-secret>
DATA_ENCRYPTION_KEYS=v1=<base64-32-byte-key>
DATA_ENCRYPTION_CURRENT_VERSION=v1
WORKER_MONITOR_TOKEN=<monitor-token>
GRAFANA_ADMIN_PASSWORD=<grafana-password>
ALLOWED_ORIGINS=https://caiyun.example.com
TRUSTED_PROXIES=none
```

设置权限并校验 Compose 语法：

```bash
chmod 600 .env
docker compose config --quiet
```

`docker compose config` 的完整输出会展开变量，排查时避免将输出复制到公开日志。

## 4. 前端构建配置

Compose 构建前端时读取 `frontend/.env.production`。同源部署保持：

```dotenv
VITE_API_BASE_URL=
VITE_PUSH_TRANSPORT=sse
VITE_SSE_URL=/events
VITE_WS_URL=/ws
```

修改任意 `VITE_*` 值后必须重新构建前端镜像。

## 5. 首次启动

```bash
docker compose build --pull
docker compose up -d
docker compose ps
```

启动顺序由 Compose 健康检查控制：

1. MySQL 和 Redis 达到健康状态。
2. `backend-migrate` 使用当前后端镜像执行数据库迁移。
3. 迁移成功后启动 API 与 Worker。
4. API 健康后启动前端。

查看迁移与启动日志：

```bash
docker compose logs --no-log-prefix backend-migrate
docker compose logs -f --tail=200 backend-api backend-worker frontend
```

如果 `backend-migrate` 失败，API 和 Worker 不会启动。修正配置或数据库问题后重新执行：

```bash
docker compose rm -f backend-migrate
docker compose up -d backend-migrate
docker compose up -d
```

## 6. 部署验证

```bash
curl -fsS http://127.0.0.1:8080/startupz
curl -fsS http://127.0.0.1:8080/readyz
curl -fsS http://127.0.0.1:8080/livez
curl -fsS http://127.0.0.1:8081/readyz
curl -I http://127.0.0.1/
```

也可以使用仓库脚本：

```bash
bash scripts/health-check.sh
```

验证 SSE：

```bash
curl -N --http1.1 \
  -H 'Cookie: auth_token=<AUTH_TOKEN>' \
  http://127.0.0.1/events
```

## 7. 日常管理

```bash
# 服务状态
docker compose ps

# 实时日志
docker compose logs -f --tail=200 backend-api backend-worker

# 重启业务服务
docker compose restart backend-api backend-worker

# 进入 MySQL
docker exec -it caiyun-mysql mysql -uroot -p caiyun

# 查看 Redis 状态
docker exec -it caiyun-redis redis-cli -a '<REDIS_PASSWORD>' INFO

# 查看资源使用
docker stats

# 停止服务但保留卷
docker compose down
```

`docker compose down -v` 会删除 MySQL、Redis 和 Grafana 卷，仅用于明确需要清空数据的环境。

## 8. 更新部署

更新前先执行数据库和配置备份，参见 [备份与恢复](./BACKUP_RESTORE.md)。

```bash
git fetch --all --tags
git checkout <VERSION_OR_COMMIT>
docker compose build --pull
docker compose up -d
docker compose ps
bash scripts/health-check.sh
```

Compose 会基于新后端镜像重新创建迁移容器，并在迁移成功后更新业务服务。生产发布应使用固定标签或 commit，不使用不可追溯的工作目录状态。

## 9. 使用预构建后端镜像

设置：

```dotenv
CAIYUN_BACKEND_IMAGE=registry.example.com/team/caiyun-backend:<VERSION>
```

登录镜像仓库并拉取：

```bash
docker login registry.example.com
docker compose pull backend-migrate backend-api backend-worker
docker compose up -d
```

当前前端服务仍由 `frontend/Dockerfile` 本地构建。如需完全使用预构建镜像，可在生产专用 Compose 覆盖文件中为 `frontend` 设置 `image` 并移除 `build`。

## 10. 日志与磁盘

容器内应用日志写入 stdout/stderr，由 Docker 日志驱动管理。建议在 `/etc/docker/daemon.json` 配置日志轮转：

```json
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "50m",
    "max-file": "5"
  }
}
```

修改后重启 Docker，并监控：

```bash
docker system df
df -h
docker volume ls
```

## 11. 生产安全检查

- `.env` 权限为 `600`，只允许部署用户读取。
- MySQL、Redis、API 和 Worker 监控端口保持回环地址绑定。
- 外部只开放 `80/443`，并由 Nginx 或负载均衡器终止 TLS。
- 不在镜像、Compose 文件或 CI 日志中写入真实密钥。
- 定期备份 MySQL、`.env` 和数据加密密钥，并执行恢复演练。
- 监控容器重启次数、MySQL 磁盘、Redis 内存、队列积压及 API 5xx。

更多内容参见 [Nginx、HTTPS 与实时推送](./NGINX_TLS.md) 和 [生产故障排查](./TROUBLESHOOTING.md)。
