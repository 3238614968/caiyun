# 升级与回滚

本文定义 Docker Compose、Linux 单机和 Kubernetes 的标准发布流程。原则是：**先备份、再迁移、分组件更新、验证后放量**。数据库迁移通常向前兼容，但应用回滚不代表数据库自动回退。

## 1. 发布前检查

1. 记录当前版本、镜像摘要、Git 提交、数据库版本和配置版本。
2. 完成 MySQL、Redis 及运行配置备份，验证备份文件可读。
3. 比较新旧 `.env.example`、Compose、systemd、Nginx 和 Kubernetes 配置。
4. 检查磁盘、内存、数据库连接数和队列积压。
5. 验证发布包校验和与签名；禁止使用来源不明的二进制。
6. 阅读变更说明，确认是否包含不可逆迁移或密钥轮换。

```bash
./caiyun-linux version
./caiyun-linux migrate --validate-only
sha256sum -c SHA256SUMS
```

详细备份方法见 [`BACKUP_RESTORE.md`](BACKUP_RESTORE.md)。

## 2. 推荐发布顺序

1. 暂停高风险批处理或降低 Worker 并发。
2. 备份数据库、Redis 和配置。
3. 执行 `migrate --validate-only`。
4. 运行正式数据库迁移。
5. 更新 API，完成健康检查。
6. 更新 Worker，观察任务消费和错误率。
7. 更新前端并清理 CDN 缓存。
8. 执行端到端验证，恢复正常并发。

## 3. Docker Compose 升级

```bash
# 在项目目录中更新代码或镜像标签
cp .env .env.backup.$(date +%Y%m%d%H%M%S)
docker compose config > compose.rendered.yaml

docker compose build backend-migrate backend-api backend-worker frontend
docker compose run --rm backend-migrate migrate --validate-only
docker compose run --rm backend-migrate migrate

docker compose up -d --no-deps backend-api
docker compose up -d --no-deps backend-worker
docker compose up -d --no-deps frontend
docker compose ps
docker compose logs --tail=200 backend-api backend-worker
```

如使用远程镜像，将 `build` 换为 `docker compose pull`。不要使用 `docker compose down -v`，该命令会删除命名卷中的数据库与 Redis 数据。

### Compose 应用回滚

1. 将镜像标签或代码恢复到上一稳定版本。
2. 保留当前数据库，先确认旧应用兼容已执行的迁移。
3. 重新启动应用组件：

```bash
docker compose up -d --no-deps backend-api backend-worker frontend
docker compose ps
```

若迁移与旧版本不兼容，应进入维护窗口，通过已验证的数据库备份恢复，而不是直接启动旧应用。

## 4. Linux 单机升级

优先使用仓库部署脚本，它会创建时间戳备份、校验发布物、运行迁移并重启服务：

```bash
sudo bash scripts/deploy-linux.sh \
  --release-dir /tmp/caiyun-release \
  --target /www/wwwroot/caiyun \
  --env-file /etc/caiyun/caiyun.env
```

使用源码目录部署时：

```bash
sudo bash scripts/deploy-linux.sh \
  --source . \
  --target /www/wwwroot/caiyun \
  --env-file /etc/caiyun/caiyun.env
```

发布后检查：

```bash
sudo systemctl status caiyun-api caiyun-worker --no-pager
sudo journalctl -u caiyun-api -u caiyun-worker -n 200 --no-pager
curl -fsS http://127.0.0.1:8080/health
```

### 单机回滚

```bash
sudo bash scripts/rollback-linux.sh --target /www/wwwroot/caiyun
```

如需指定备份目录，先查看 `/www/wwwroot/caiyun/backups/`，再按脚本帮助选择：

```bash
bash scripts/rollback-linux.sh --help
```

回滚后必须重新检查服务、数据库兼容性、登录、任务创建和队列消费。

## 5. Kubernetes 升级

1. 将清单镜像标签更新为新的不可变版本。
2. 重新创建迁移 Job并等待成功。
3. 逐个滚动 API、Worker 和前端。

```bash
bash scripts/deploy-k8s.sh
kubectl -n caiyun get pods,job
kubectl -n caiyun rollout status deployment/backend-api
kubectl -n caiyun rollout status deployment/backend-worker
kubectl -n caiyun rollout status deployment/frontend
```

也可单独更新镜像：

```bash
kubectl -n caiyun set image deployment/backend-api api=REGISTRY/backend:VERSION
kubectl -n caiyun rollout status deployment/backend-api --timeout=5m
```

### Kubernetes 应用回滚

```bash
kubectl -n caiyun rollout history deployment/backend-api
kubectl -n caiyun rollout undo deployment/backend-api
kubectl -n caiyun rollout undo deployment/backend-worker
kubectl -n caiyun rollout undo deployment/frontend
```

先确认旧版本兼容当前数据库。迁移 Job 本身不随 Deployment 的 `rollout undo` 回滚。

## 6. 回滚决策

| 情况 | 建议动作 |
| --- | --- |
| 前端静态资源错误 | 仅回滚前端，必要时清 CDN 缓存 |
| API 新版本健康检查失败 | 停止放量并回滚 API 镜像/二进制 |
| Worker 错误率上升 | 暂停或缩容 Worker，保留 API 服务 |
| 配置错误 | 恢复上一份配置并重启受影响组件 |
| 迁移失败且未提交 | 修复配置或权限后重新执行迁移 |
| 迁移已成功但旧应用不兼容 | 使用向前修复版本，或维护窗口恢复数据库备份 |
| 数据被错误修改 | 停止写入，保留现场，按恢复流程处理 |

## 7. 发布后验证

- `/health` 与就绪检查正常。
- 登录、账户管理、任务创建、兑换和实时事件链路正常。
- API 5xx、P95 延迟和 Worker 错误率无异常。
- 队列积压持续下降，未出现重复执行。
- 数据库连接、慢查询、锁等待和磁盘使用正常。
- 新版本日志中无密钥、密码或 Token 泄露。

每次发布应保存：版本、提交 SHA、镜像摘要、操作人、开始/结束时间、迁移结果、验证记录和回滚点。