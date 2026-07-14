# 独立 Docker 容器部署

本方案不依赖 Compose，由运维系统分别管理镜像、网络、数据卷和容器。适用于已有外部 MySQL/Redis、使用私有镜像仓库，或需要将各运行单元纳入现有容器平台的场景。

## 1. 构建镜像

```bash
VERSION=<VERSION>
COMMIT=$(git rev-parse --short HEAD)
BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)

docker build -t registry.example.com/team/caiyun-backend:${VERSION} \
  --build-arg VERSION=${VERSION} \
  --build-arg COMMIT=${COMMIT} \
  --build-arg BUILD_TIME=${BUILD_TIME} \
  backend

docker build -t registry.example.com/team/caiyun-frontend:${VERSION} frontend
```

前端构建前确认 `frontend/.env.production` 中的 API 与推送地址。`VITE_*` 配置已固化到静态文件，运行容器时再设置不会改变前端行为。

推送镜像：

```bash
docker login registry.example.com
docker push registry.example.com/team/caiyun-backend:${VERSION}
docker push registry.example.com/team/caiyun-frontend:${VERSION}
```

生产环境使用不可变版本标签；条件允许时记录并部署镜像 digest。

## 2. 创建网络与数据卷

```bash
docker network create caiyun-network
docker volume create caiyun-mysql-data
docker volume create caiyun-redis-data
```

如果使用托管 MySQL/Redis，可跳过对应容器和数据卷，只保留网络，并在后端环境文件中配置服务地址。

## 3. 启动 MySQL 和 Redis（可选）

```bash
docker run -d \
  --name caiyun-mysql \
  --network caiyun-network \
  --restart unless-stopped \
  -e MYSQL_ROOT_PASSWORD='<MYSQL_ROOT_PASSWORD>' \
  -e MYSQL_DATABASE='caiyun' \
  -e MYSQL_USER='caiyun_app' \
  -e MYSQL_PASSWORD='<DB_PASSWORD>' \
  -e TZ='Asia/Shanghai' \
  -v caiyun-mysql-data:/var/lib/mysql \
  mysql:8.0

docker run -d \
  --name caiyun-redis \
  --network caiyun-network \
  --restart unless-stopped \
  -v caiyun-redis-data:/data \
  redis:7.0 redis-server --appendonly yes --requirepass '<REDIS_PASSWORD>'
```

等待依赖就绪：

```bash
docker exec caiyun-mysql mysqladmin ping -h 127.0.0.1 -uroot -p'<MYSQL_ROOT_PASSWORD>'
docker exec caiyun-redis redis-cli -a '<REDIS_PASSWORD>' ping
```

## 4. 创建后端环境文件

在宿主机创建 `/opt/caiyun/config/backend.env`：

```dotenv
APP_ENV=production
DB_AUTO_MIGRATE=false
DB_HOST=caiyun-mysql
DB_PORT=3306
DB_USER=caiyun_app
DB_PASSWORD=<DB_PASSWORD>
DB_NAME=caiyun
REDIS_HOST=caiyun-redis
REDIS_PORT=6379
REDIS_PASSWORD=<REDIS_PASSWORD>
RATE_LIMIT_BACKEND=redis
TASK_QUEUE_BACKEND=streams
JWT_SECRET=<JWT_SECRET>
JWT_ISSUER=caiyun-api
JWT_AUDIENCE=caiyun-web
DATA_ENCRYPTION_KEYS=v1=<BASE64_32_BYTE_KEY>
DATA_ENCRYPTION_CURRENT_VERSION=v1
WORKER_MONITOR_TOKEN=<WORKER_MONITOR_TOKEN>
WORKER_MONITOR_HOST=0.0.0.0
WORKER_MONITOR_PORT=8081
WORKER_MONITOR_ALLOW_PLAINTEXT=true
ALLOWED_ORIGINS=https://caiyun.example.com
TRUSTED_PROXIES=none
LOG_FILE_PATH=
LOG_JSON_FORMAT=true
TZ=Asia/Shanghai
```

```bash
chmod 600 /opt/caiyun/config/backend.env
```

如果数据库或 Redis 位于其他主机，替换 `DB_HOST`、`REDIS_HOST`，并通过防火墙或安全组限制来源为容器宿主机。

## 5. 执行数据库迁移

每个版本发布时先使用同版本后端镜像运行一次迁移：

```bash
docker run --rm \
  --name caiyun-migrate \
  --network caiyun-network \
  --env-file /opt/caiyun/config/backend.env \
  registry.example.com/team/caiyun-backend:${VERSION} migrate
```

结构验证：

```bash
docker run --rm \
  --network caiyun-network \
  --env-file /opt/caiyun/config/backend.env \
  registry.example.com/team/caiyun-backend:${VERSION} migrate --validate-only
```

迁移失败时停止发布，保留旧 API/Worker 和数据库备份用于分析。

## 6. 启动 API、Worker 和前端

```bash
docker run -d \
  --name backend-api \
  --network caiyun-network \
  --restart unless-stopped \
  --env-file /opt/caiyun/config/backend.env \
  -e INSTANCE_ID=docker-api-1 \
  -p 127.0.0.1:8080:8080 \
  registry.example.com/team/caiyun-backend:${VERSION} api

docker run -d \
  --name backend-worker \
  --network caiyun-network \
  --restart unless-stopped \
  --env-file /opt/caiyun/config/backend.env \
  -e INSTANCE_ID=docker-worker-1 \
  -p 127.0.0.1:8081:8081 \
  registry.example.com/team/caiyun-backend:${VERSION} worker

docker run -d \
  --name caiyun-frontend \
  --network caiyun-network \
  --restart unless-stopped \
  -p 127.0.0.1:8088:8080 \
  registry.example.com/team/caiyun-frontend:${VERSION}
```

前端镜像中的 Nginx 通过容器 DNS 名 `backend-api:8080` 转发 `/api`、`/events` 和 `/ws`，因此 API 容器名称应保持为 `backend-api`，或同步修改 `frontend/nginx.conf` 后重新构建镜像。

## 7. 验证与管理

```bash
docker ps --filter name=caiyun --filter name=backend-
docker logs --tail=200 backend-api
docker logs --tail=200 backend-worker
curl -fsS http://127.0.0.1:8080/readyz
curl -fsS http://127.0.0.1:8081/readyz
curl -I http://127.0.0.1:8088/
```

外部 Nginx 将公网域名代理到 `127.0.0.1:8088`。配置方法参见 [Nginx、HTTPS 与实时推送](./NGINX_TLS.md)。

## 8. 更新容器

1. 备份数据库和环境文件。
2. 拉取新镜像并验证 digest。
3. 使用新镜像执行迁移与结构验证。
4. 停止并移除旧 API、Worker、Frontend 容器。
5. 使用相同名称和配置启动新版本。
6. 执行健康检查和业务验证。

```bash
docker pull registry.example.com/team/caiyun-backend:<NEW_VERSION>
docker pull registry.example.com/team/caiyun-frontend:<NEW_VERSION>

docker stop backend-api backend-worker caiyun-frontend
docker rm backend-api backend-worker caiyun-frontend
# 按“启动 API、Worker 和前端”重新创建容器
```

容器删除不会删除具名数据卷。删除 MySQL/Redis 卷前应完成备份并经过明确确认。

## 9. 资源与安全参数

生产环境可为 `docker run` 增加：

```text
--memory <LIMIT>
--cpus <LIMIT>
--pids-limit <LIMIT>
--read-only
--tmpfs /tmp
--security-opt no-new-privileges
```

后端镜像已使用非 root 用户。启用 `--read-only` 时，确保应用保持 `LOG_FILE_PATH=`，将日志写到 stdout。前端镜像同样为非特权 Nginx，并监听容器内 `8080`。
