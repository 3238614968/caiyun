# Linux 单机与 systemd 部署

本方案面向长期运行的 Linux 服务器：Nginx 提供 HTTPS 和静态文件，`caiyun-api.service` 与 `caiyun-worker.service` 分别托管 API 和 Worker，MySQL/Redis 可运行在本机或使用托管服务。

## 1. 推荐拓扑

```text
Internet -> Nginx :443
              |-- static files: /www/wwwroot/caiyun
              |-- /api, /events, /ws -> 127.0.0.1:8080
API :8080 -> MySQL + Redis
Worker :8081 -> MySQL + Redis + upstream APIs
```

公网通常只开放 `22`、`80` 和 `443`。`3306`、`6379`、`8080`、`8081` 应绑定内网或回环地址，并通过防火墙限制访问。

## 2. 前置条件

- 64 位 Linux，支持 systemd。
- MySQL 8.0+、Redis 7.0+。
- Nginx、curl、tar、gzip、sha256sum。
- 构建机需要 Go 1.25.12、Node.js 20+、npm；生产机使用发布包时无需安装 Go 和 Node.js。
- 域名已解析到服务器，系统时间已同步。

Debian/Ubuntu 示例：

```bash
sudo apt update
sudo apt install -y nginx mysql-client redis-tools curl ca-certificates tar gzip
```

数据库和 Redis 可以由托管服务提供；此时使用其专用网络地址，并在安全组中只允许应用服务器访问。

## 3. 创建运行用户和目录

仓库提供的 systemd 单元默认使用 `www:www` 和 `/www/wwwroot/caiyun`。如系统不存在该用户，可创建，或统一修改 unit 中的 `User`、`Group`、`WorkingDirectory`、`EnvironmentFile`、`ExecStart` 和 `ReadWritePaths`。

```bash
sudo groupadd --system www 2>/dev/null || true
sudo useradd --system --gid www --home /www/wwwroot/caiyun \
  --shell /usr/sbin/nologin www 2>/dev/null || true
sudo install -d -o www -g www -m 0750 /www/wwwroot/caiyun
sudo install -d -o www -g www -m 0750 /www/wwwroot/caiyun/logs
sudo install -d -o www -g www -m 0750 /www/wwwroot/caiyun/backups
```

## 4. 构建发布包

建议在 CI 或独立构建机完成：

```bash
make backend-build
cd frontend
npm ci
npm run typecheck
npm run test:unit
npm run build
cd ..

VERSION=<VERSION> bash scripts/package-release.sh
```

发布目录位于 `release/<VERSION>-linux-amd64/`，包含：

- `caiyun-linux`
- `caiyun-frontend-<VERSION>.tar.gz`
- `caiyun-migrations-<VERSION>.tar.gz`
- `SHA256SUMS`、签名和 SBOM（取决于构建环境）
- 部署、回滚、健康检查和运维脚本

将整个发布目录上传到服务器临时目录，例如 `/opt/releases/caiyun/<VERSION>/`。

## 5. 创建生产环境文件

创建 `/www/wwwroot/caiyun/.env`：

```dotenv
APP_ENV=production
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
RATE_LIMIT_BACKEND=redis
TASK_QUEUE_BACKEND=streams
JWT_SECRET=<JWT_SECRET>
JWT_ISSUER=caiyun-api
JWT_AUDIENCE=caiyun-web
DATA_ENCRYPTION_KEYS=v1=<BASE64_32_BYTE_KEY>
DATA_ENCRYPTION_CURRENT_VERSION=v1
WORKER_MONITOR_TOKEN=<WORKER_MONITOR_TOKEN>
WORKER_MONITOR_HOST=127.0.0.1
WORKER_MONITOR_PORT=8081
WORKER_MONITOR_ALLOW_PLAINTEXT=true
ALLOWED_ORIGINS=https://caiyun.example.com
TRUSTED_PROXIES=127.0.0.1
LOG_FILE_PATH=/www/wwwroot/caiyun/logs
LOG_JSON_FORMAT=true
TZ=Asia/Shanghai
```

```bash
sudo chown www:www /www/wwwroot/caiyun/.env
sudo chmod 600 /www/wwwroot/caiyun/.env
```

`DATA_ENCRYPTION_KEYS` 支持 32 字节原始字符串、Base64 或十六进制值。生产密钥应保存在独立密码库和离线备份中。

## 6. 初始化数据库

创建数据库和应用用户的示例：

```sql
CREATE DATABASE caiyun CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER 'caiyun_app'@'127.0.0.1' IDENTIFIED BY '<DB_PASSWORD>';
GRANT ALL PRIVILEGES ON caiyun.* TO 'caiyun_app'@'127.0.0.1';
FLUSH PRIVILEGES;
```

若数据库为远程服务，将用户来源限制为应用服务器内网地址。应用表结构由 `caiyun-linux migrate` 管理，不在 API/Worker 启动时自动变更。

## 7. 首次安装

在 systemd 服务尚未安装时，先部署制品并执行迁移：

```bash
cd /opt/releases/caiyun/<VERSION>
sudo bash deploy-linux.sh \
  --release-dir /opt/releases/caiyun/<VERSION> \
  --target /www/wwwroot/caiyun \
  --env-file /www/wwwroot/caiyun/.env \
  --skip-services \
  --skip-nginx
```

脚本会执行以下操作：

1. 验证 `SHA256SUMS`；存在 Sigstore bundle 且安装 cosign 时执行签名验证。
2. 使用新后端制品运行数据库迁移。
3. 将现有文件备份到 `backups/deploy-YYYYmmdd-HHMMSS`。
4. 安装统一后端二进制和前端静态文件。

校验数据库结构：

```bash
cd /www/wwwroot/caiyun
sudo -u www env $(grep -v '^#' .env | xargs) ./caiyun-linux migrate --validate-only
```

环境值包含空格或特殊字符时，不使用上述 `env $(...)` 形式，改用：

```bash
sudo -u www bash -c 'set -a; . /www/wwwroot/caiyun/.env; set +a; exec /www/wwwroot/caiyun/caiyun-linux migrate --validate-only'
```

## 8. 安装 systemd 单元

```bash
sudo cp deploy/systemd/caiyun-api.service /etc/systemd/system/
sudo cp deploy/systemd/caiyun-worker.service /etc/systemd/system/
sudo cp deploy/systemd/caiyun-migrate.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable caiyun-api.service caiyun-worker.service
sudo systemctl start caiyun-api.service
sudo systemctl start caiyun-worker.service
```

服务状态与日志：

```bash
systemctl status caiyun-api caiyun-worker --no-pager
journalctl -u caiyun-api -n 200 --no-pager
journalctl -u caiyun-worker -n 200 --no-pager
journalctl -u caiyun-api -u caiyun-worker -f
```

单独运行迁移服务：

```bash
sudo systemctl start caiyun-migrate.service
sudo journalctl -u caiyun-migrate.service -n 200 --no-pager
```

## 9. 配置 Nginx 和 HTTPS

仓库根目录的 `nginx-server.conf` 是完整站点示例。复制前必须修改域名、证书路径和静态文件目录：

```bash
sudo cp nginx-server.conf /etc/nginx/conf.d/caiyun.conf
sudo nginx -t
sudo systemctl reload nginx
```

详细配置参见 [Nginx、HTTPS 与实时推送](./NGINX_TLS.md)。

## 10. 安装日志轮转

```bash
sudo cp deploy/logrotate/caiyun /etc/logrotate.d/caiyun
sudo logrotate -d /etc/logrotate.d/caiyun
```

应用自身支持日志轮转，系统 logrotate 作为文件日志部署的补充保护。应同时监控 journal、应用日志目录和磁盘空间。

## 11. 防火墙示例

使用 UFW：

```bash
sudo ufw allow OpenSSH
sudo ufw allow 'Nginx Full'
sudo ufw default deny incoming
sudo ufw enable
sudo ufw status
```

如果 MySQL/Redis 位于远程主机，只在对应安全组中放行应用服务器内网 IP，不将数据库端口开放给互联网。

## 12. 部署验证

```bash
bash /opt/releases/caiyun/<VERSION>/health-check.sh
curl -fsS http://127.0.0.1:8080/health
curl -fsS http://127.0.0.1:8081/health
curl -fsS https://caiyun.example.com/readyz
curl -I https://caiyun.example.com/
```

检查实际运行版本：

```bash
/www/wwwroot/caiyun/caiyun-linux version
```

## 13. 后续升级与回滚

```bash
sudo bash /opt/releases/caiyun/<VERSION>/deploy-linux.sh \
  --release-dir /opt/releases/caiyun/<VERSION> \
  --target /www/wwwroot/caiyun \
  --env-file /www/wwwroot/caiyun/.env \
  --health-check
```

回滚应用文件：

```bash
sudo bash /opt/releases/caiyun/<VERSION>/rollback-linux.sh \
  --target /www/wwwroot/caiyun \
  --health-check
```

数据库迁移可能改变结构，应用回滚前应确认旧版本与当前数据库兼容。完整流程参见 [版本升级与回滚](./UPGRADE_ROLLBACK.md)。

## 14. systemd 安全说明

仓库 unit 已启用 `NoNewPrivileges`、`PrivateTmp`、`ProtectSystem=full` 和 `ReadWritePaths`。修改目录后必须同步调整可写路径。进一步加固前先在预发布环境验证，避免阻止证书、日志、临时目录或网络访问。
