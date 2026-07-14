# 备份与恢复

系统恢复能力依赖 MySQL 数据、Redis 状态、运行密钥和可重建的应用发布物。至少按业务 RPO/RTO 制定备份频率，并定期在隔离环境执行完整恢复演练。

## 1. 备份范围

| 对象 | 必要性 | 说明 |
| --- | --- | --- |
| MySQL | 必须 | 用户、账户、任务、兑换记录和系统配置 |
| `.env`/Secret | 必须 | JWT、数据库、Redis、SMTP、监控令牌和加密密钥 |
| `DATA_ENCRYPTION_KEYS` | 必须 | 缺失历史密钥将导致既有敏感数据不可解密 |
| Redis | 建议 | 队列、缓存、锁和短期状态；恢复策略取决于允许的数据损失 |
| 发布包/镜像 | 必须 | 用于应用回滚和灾难重建 |
| Nginx/TLS/systemd | 建议 | 缩短主机重建时间 |
| 日志与审计记录 | 按合规要求 | 用于问题追踪，不应作为数据库备份替代品 |

备份文件应加密、异地保存、限制读取权限，并配置保留周期和删除策略。

## 2. Linux 单机 MySQL 备份

```bash
sudo install -d -m 0700 /var/backups/caiyun
BACKUP=/var/backups/caiyun/mysql-$(date +%Y%m%d-%H%M%S).sql.gz

mysqldump \
  --single-transaction \
  --routines --triggers --events \
  --default-character-set=utf8mb4 \
  -h 127.0.0.1 -u caiyun -p caiyun \
  | gzip -9 > "$BACKUP"

gzip -t "$BACKUP"
sha256sum "$BACKUP" > "$BACKUP.sha256"
```

不要把数据库密码直接写入脚本。可使用权限为 `0600` 的 MySQL option file、受控环境变量或专用备份账户。

### 恢复 MySQL

恢复前停止应用写入，并确认目标数据库为空或允许被覆盖：

```bash
sudo systemctl stop caiyun-worker caiyun-api
gunzip -c /var/backups/caiyun/mysql-TIMESTAMP.sql.gz \
  | mysql -h 127.0.0.1 -u caiyun -p caiyun
```

恢复后先验证迁移，再启动服务：

```bash
cd /www/wwwroot/caiyun
./caiyun-linux migrate --validate-only
sudo systemctl start caiyun-api caiyun-worker
```

## 3. Docker Compose 备份

MySQL 容器内已有根密码环境变量，可在容器内部执行导出：

```bash
mkdir -p backups
BACKUP=backups/mysql-$(date +%Y%m%d-%H%M%S).sql.gz

docker compose exec -T mysql sh -c \
  'exec mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" --single-transaction --routines --triggers --events caiyun' \
  | gzip -9 > "$BACKUP"

gzip -t "$BACKUP"
sha256sum "$BACKUP" > "$BACKUP.sha256"
```

### Compose 恢复

```bash
docker compose stop backend-worker backend-api
gunzip -c backups/mysql-TIMESTAMP.sql.gz \
  | docker compose exec -T mysql sh -c \
    'exec mysql -uroot -p"$MYSQL_ROOT_PASSWORD" caiyun'

docker compose run --rm backend-migrate migrate --validate-only
docker compose start backend-api backend-worker
```

恢复前建议先创建当前数据库的应急快照。不要通过删除卷的方式“清理”数据库。

## 4. Redis 备份

Redis 主要承载队列、缓存和锁。数据库备份与 Redis 快照时间不一致时，恢复后可能出现任务重复、积压丢失或状态不一致，因此必须让任务处理具备幂等性，并在恢复后人工核对关键任务。

单机 Redis：

```bash
redis-cli -a REDIS_PASSWORD BGSAVE
redis-cli -a REDIS_PASSWORD LASTSAVE
sudo cp /var/lib/redis/dump.rdb /var/backups/caiyun/redis-$(date +%Y%m%d-%H%M%S).rdb
```

Compose：

```bash
docker compose exec redis redis-cli -a REDIS_PASSWORD BGSAVE
docker compose exec redis redis-cli -a REDIS_PASSWORD LASTSAVE
docker cp caiyun-redis:/data/dump.rdb backups/redis-$(date +%Y%m%d-%H%M%S).rdb
```

仓库默认 Redis 启用了 AOF，实际恢复时应同时保存 `/data/appendonlydir`。复制数据文件前优先停止 Redis，或使用存储卷快照保证一致性。

## 5. Kubernetes 备份

生产环境优先使用云数据库备份、Redis 托管服务备份和 CSI VolumeSnapshot。内置 MySQL 的逻辑备份示例：

```bash
kubectl -n caiyun exec mysql-0 -- sh -c \
  'exec mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" --single-transaction --routines --triggers --events caiyun' \
  | gzip -9 > mysql-$(date +%Y%m%d-%H%M%S).sql.gz
```

同时导出非敏感配置和资源定义：

```bash
kubectl -n caiyun get configmap caiyun-config -o yaml > caiyun-config.yaml
kubectl -n caiyun get deploy,statefulset,service,pvc,ingress -o yaml > caiyun-resources.yaml
```

Secret 应通过专用密钥管理或加密备份方案保存，不要将 `kubectl get secret -o yaml` 的明文等价内容提交到 Git。

## 6. 配置与密钥备份

至少保存以下内容：

- 生产 `.env` 或 Kubernetes Secret 的受控副本。
- 所有历史 `DATA_ENCRYPTION_KEYS` 及其版本号。
- Nginx 配置、TLS 证书来源和自动续期方式。
- systemd unit、自定义 EnvironmentFile 路径和目录权限。
- 镜像仓库地址、镜像摘要和发布包校验和。

密钥备份应使用独立加密密钥，并安排双人恢复或审计流程。

## 7. 标准恢复顺序

1. 宣布维护并阻断外部写入。
2. 停止 Worker，再停止 API；保留日志和故障现场。
3. 恢复 MySQL 到目标时间点。
4. 根据恢复策略恢复或清理 Redis 队列状态。
5. 恢复匹配版本的配置、加密密钥和应用发布物。
6. 执行 `migrate --validate-only`，必要时执行迁移。
7. 仅启动 API，完成健康检查和只读验证。
8. 启动 Worker，低并发观察任务处理。
9. 恢复外部流量，持续监控错误率、队列和数据一致性。

## 8. 恢复演练与保留策略

- 每月至少进行一次随机备份校验，每季度进行完整恢复演练。
- 同时验证 SQL 可导入、加密字段可解密、用户可登录、任务可执行。
- 使用“每日 + 每周 + 每月”多层保留，并保留至少一份异地副本。
- 对备份失败、文件过小、校验和不一致和超出恢复时间目标建立告警。
- 演练后记录实际 RPO、RTO、缺失步骤和改进负责人。