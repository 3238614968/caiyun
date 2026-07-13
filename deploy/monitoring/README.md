# Prometheus / Grafana 接入

文件说明：

- `prometheus-rules.yml`：抢兑成功率、失败激增、限流拒绝、审计日志丢弃、队列积压、API P95、Worker metrics 探测，以及历史归档失败 / 超 48 小时未成功 / 命中批次上限等告警规则。
- `grafana-dashboard-caiyun.json`：抢兑成功率、失败原因、调度命中/跳过、队列长度和接口耗时看板。

当前历史归档相关指标包括：

- `caiyun_history_archive_runs_total{status=...}`
- `caiyun_history_archive_duration_seconds`
- `caiyun_history_archive_moved_rows_total{table=...}`
- `caiyun_history_archive_hit_batch_limit_total`
- `caiyun_history_archive_last_run_unix`
- `caiyun_history_archive_last_success_unix`

Prometheus 抓取 Worker `/metrics` 时，如果设置了 `WORKER_MONITOR_TOKEN`，需要配置 Bearer Token 或 `X-Monitor-Token`。
