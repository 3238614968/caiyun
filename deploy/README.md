# 部署与运维模板

唯一后端制品是 `caiyun-linux`：

```text
caiyun-linux api
caiyun-linux worker
caiyun-linux migrate [--validate-only]
caiyun-linux reencrypt [--apply]
caiyun-linux all
```

生产建议同一制品分别运行 API/Worker；`all` 只用于单机便捷安装。

## 生产配置

`APP_ENV=production` 时必须配置 `DATA_ENCRYPTION_KEYS` 与 `DATA_ENCRYPTION_CURRENT_VERSION`。API/Worker 必须保持 `DB_AUTO_MIGRATE=false`。`TRUSTED_PROXIES` 直连设为 `none`，反代时仅填写真实代理 IP/CIDR。

## systemd 与发布顺序

```bash
cp deploy/systemd/caiyun-{api,worker,migrate}.service /etc/systemd/system/
systemctl daemon-reload

# 先备份数据库，再使用新制品迁移和校验
systemctl start caiyun-migrate
/www/wwwroot/caiyun/caiyun-linux migrate --validate-only

systemctl enable --now caiyun-api caiyun-worker
```

如果运行用户不是 `www:www`，先调整 unit 的 `User`、`Group` 和 `ReadWritePaths`。

## 裸机部署与回滚

```bash
make backend-build
cd frontend && npm run build
cd ..
bash scripts/deploy-linux.sh --target /www/wwwroot/caiyun --health-check
bash scripts/rollback-linux.sh --target /www/wwwroot/caiyun
```

部署脚本默认运行新制品的 `migrate`；仅当外部流水线已经完成同版本迁移时使用 `--skip-migrations`。迁移失败时脚本不替换文件，并尝试恢复旧服务。

## Kubernetes

固定名称的已完成 Job 不会被 `kubectl apply` 自动重跑，PodTemplate 更新还会遇到 immutable 校验。每次发布运行：

```bash
bash scripts/deploy-k8s.sh
```

脚本会删除旧 `caiyun-migrate` Job、应用清单、等待迁移成功，再等待 API/Worker/Frontend rollout。

## 健康探针

```text
/livez     进程存活
/readyz    MySQL、Redis、任务队列可用
/startupz  配置、依赖与初始化完成
```

```bash
bash scripts/health-check.sh
PUBLIC_URL=https://your-domain.example/readyz bash scripts/health-check.sh
```

## 数据加密轮换

```bash
./caiyun-linux reencrypt --table all --batch-size 200
./caiyun-linux reencrypt --table all --batch-size 200 --apply
```

详见 `backend/docs/ENCRYPTION_ROTATION.md`。

## 其他运维资源

- logrotate：`deploy/logrotate/caiyun`
- 节假日导入：`scripts/import-calendar.sh`
- Prometheus/Grafana：`deploy/monitoring`
- 发布打包：`scripts/package-release.sh`

发布包只包含一个 `caiyun-linux`，以及前端、迁移 SQL、监控、日历、SBOM、运维脚本和 `SHA256SUMS`。生产 `.env` 不会也不应被发布包覆盖。
