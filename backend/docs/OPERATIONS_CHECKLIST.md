# 生产运行与观测检查清单

## 启动前

- `.env` 中必须设置强随机 `JWT_SECRET`、`REDIS_PASSWORD`、`MYSQL_PASSWORD`、`WORKER_MONITOR_TOKEN`。
- `ALLOWED_ORIGINS` 必须包含用户实际访问前端的完整 Origin。
- 推荐设置：
  - `LOG_JSON_FORMAT=true`
  - `LOG_FILE_PATH=/www/wwwroot/caiyun/logs`
  - `TASK_QUEUE_BACKEND=streams`；如需回退可临时改为 `list`
  - `EXCHANGE_TASK_RUNNING_TIMEOUT=15m`

## 健康检查

```bash
bash scripts/health-check.sh
PUBLIC_URL=https://your-domain.example/health bash scripts/health-check.sh
```

API 与 Worker 的 `/health` 会返回版本、commit、构建时间和 Go 版本，便于确认替换是否生效。

## 指标

- API：`/metrics`，需要管理员登录态。
- Worker：`/metrics`，需要 `WORKER_MONITOR_TOKEN`。
- 推荐监控：
  - 任务 pending/running/completed 数量。
  - Redis pending/processing/delayed/dead-letter 长度。
  - 兑换成功率与失败原因。
  - Worker 进程重启次数。
  - API 5xx 与 429 速率。

## 日志

应用支持文件轮转与 JSON 格式：

```env
LOG_FILE_PATH=/www/wwwroot/caiyun/logs
LOG_JSON_FORMAT=true
LOG_MAX_SIZE=100
LOG_MAX_BACKUPS=7
LOG_COMPRESS=true
```

裸机部署可同时安装 `deploy/logrotate/caiyun` 作为系统层兜底。

## 回滚

每次 `scripts/deploy-linux.sh` 会创建：

```text
/www/wwwroot/caiyun/backups/deploy-YYYYmmdd-HHMMSS
```

回滚：

```bash
bash scripts/rollback-linux.sh --target /www/wwwroot/caiyun
```
