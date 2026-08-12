# 源码部署与本地联调

源码部署适用于开发、调试、集成测试和预发布验证。生产环境优先使用固定版本制品、Docker 镜像或 systemd 管理的编译二进制，避免依赖 `go run`、Vite 开发服务器和在线安装依赖。

## 1. 开发环境

- Go 1.25.12
- Node.js 20+ 与 npm
- MySQL 8.0+
- Redis 7.0+
- Git、curl

```bash
go version
node --version
npm --version
mysql --version
redis-cli --version
```

## 2. 获取代码和依赖

```bash
git clone <REPOSITORY_URL> caiyun
cd caiyun

cd backend
go mod download
cd ../frontend
npm ci
cd ..
```

依赖版本由 `backend/go.sum` 和 `frontend/package-lock.json` 锁定。更新依赖后应提交对应锁文件并执行完整测试。

## 3. 准备 MySQL 和 Redis

可以使用本地服务，也可以使用 Compose 仅启动依赖。Compose 解析配置时需要根目录 `.env`：

```bash
cp .env.example .env
# 修改所有必填密码和密钥
docker compose up -d mysql redis
docker compose ps mysql redis
```

直接使用本机 MySQL 时，先创建空数据库和最小权限账户；业务表结构由版本化迁移创建：

```bash
mysql -uroot -p -e "CREATE DATABASE IF NOT EXISTS caiyun CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
cd backend
go run ./cmd/caiyun migrate
```

后续升级同样只运行 `caiyun migrate`，不再使用已移除的初始化 SQL 副本。

## 4. 创建后端开发配置

后端优先从当前工作目录加载 `.env`。在 `backend/.env` 中配置：

```dotenv
APP_ENV=development
DB_AUTO_MIGRATE=false
PORT=8080
DB_HOST=127.0.0.1
DB_PORT=3306
DB_USER=caiyun_app
DB_PASSWORD=<DB_PASSWORD>
DB_NAME=caiyun
REDIS_HOST=127.0.0.1
REDIS_PORT=6379
REDIS_PASSWORD=<REDIS_PASSWORD>
TASK_QUEUE_BACKEND=streams
RATE_LIMIT_BACKEND=redis
JWT_SECRET=<JWT_SECRET>
DATA_ENCRYPTION_KEYS=v1=<BASE64_32_BYTE_KEY>
DATA_ENCRYPTION_CURRENT_VERSION=v1
WORKER_MONITOR_TOKEN=<WORKER_MONITOR_TOKEN>
WORKER_MONITOR_HOST=127.0.0.1
WORKER_MONITOR_PORT=8081
ALLOWED_ORIGINS=http://localhost:5173
TRUSTED_PROXIES=none
LOG_JSON_FORMAT=false
```

```bash
chmod 600 backend/.env
```

开发环境也应使用与生产相同的版本化加密密钥格式，避免测试数据在切换环境后不可读。

## 5. 执行迁移

```bash
cd backend
go run ./cmd/caiyun migrate
go run ./cmd/caiyun migrate --validate-only
```

迁移失败时检查数据库权限、字符集、当前迁移版本和连接地址。不要通过启用 API 自动迁移替代发布迁移流程。

## 6. 启动各组件

使用独立终端：

```bash
# 终端 1：API
cd backend
go run ./cmd/caiyun api
```

```bash
# 终端 2：Worker
cd backend
go run ./cmd/caiyun worker
```

```bash
# 终端 3：前端
cd frontend
cp .env.example .env.local
npm run dev
```

前端联调配置示例：

```dotenv
VITE_API_BASE_URL=http://127.0.0.1:8080
VITE_PUSH_TRANSPORT=sse
VITE_SSE_URL=http://127.0.0.1:8080/events
VITE_WS_URL=ws://127.0.0.1:8080/ws
```

跨域直连时，后端 `ALLOWED_ORIGINS` 必须包含 `http://localhost:5173`。若使用 Vite/Nginx 同源代理，则可保持相对路径。

## 7. 运行验证

```bash
curl -fsS http://127.0.0.1:8080/startupz
curl -fsS http://127.0.0.1:8080/readyz
curl -fsS http://127.0.0.1:8081/readyz
```

浏览器访问 Vite 输出的本地地址，完成登录、账号列表、任务创建、任务执行和实时推送验证。

## 8. 测试和静态检查

```bash
# 后端
cd backend
go test ./...
go test -race ./...
go vet ./...

# 前端
cd ../frontend
npm run typecheck
npm run lint
npm run test:unit
npm run build
npm run bundle:budget
npm run e2e
```

需要 Redis 集成测试时：

```bash
cd backend
CAIYUN_REDIS_INTEGRATION=1 \
CAIYUN_TEST_REDIS_ADDR=127.0.0.1:6379 \
CAIYUN_TEST_REDIS_DB=15 \
go test ./internal/queue -run RedisIntegration -count=1
```

## 9. 从源码构建生产制品

```bash
make backend-build
cd frontend
npm ci
npm run build
npm run bundle:budget
cd ..
```

输出：

```text
backend/caiyun-linux
backend/SHA256SUMS
frontend/dist/
```

使用仓库部署脚本安装到单机目录：

```bash
sudo bash scripts/deploy-linux.sh \
  --source . \
  --target /www/wwwroot/caiyun \
  --env-file /www/wwwroot/caiyun/.env \
  --health-check
```

## 10. 开发数据清理

停止组件后按需清理测试环境。使用 Compose 时：

```bash
docker compose down          # 保留数据卷
docker compose down -v       # 删除数据卷和全部本地数据
```

删除卷属于不可逆的数据清理操作，应先确认环境中不含需要保留的数据。
