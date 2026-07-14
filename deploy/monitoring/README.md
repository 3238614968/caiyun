# Prometheus 与 Grafana 监控部署

本目录提供 Prometheus 告警规则和 Grafana 看板。系统分别暴露 API 与 Worker 指标；生产环境应通过内网或受控反向代理抓取，并使用独立监控令牌保护端点。

## 1. 文件说明

- `prometheus-rules.yml`：抢兑成功率、失败激增、限流拒绝、审计日志丢弃、队列积压、API P95、Worker 探测和历史归档异常告警。
- `grafana-dashboard-caiyun.json`：抢兑结果、失败原因、调度命中/跳过、队列长度、接口耗时及历史归档看板。

`docker-compose.yml` 默认启动 Grafana，但不包含 Prometheus。可连接现有 Prometheus，或在独立监控主机部署 Prometheus 后导入本目录配置。

## 2. 监控端点

| 组件 | 默认地址 | 认证 |
| --- | --- | --- |
| API 指标 | `http://backend-api:8080/metrics` | `API_MONITOR_TOKEN`，支持 Bearer 或 `X-Monitor-Token`；管理员 JWT 也可访问 |
| API 存活 | `/livez`、`/startupz` | 无认证 |
| API 就绪 | `/readyz`、`/health` | 无认证，检查外部依赖 |
| Worker 指标 | `http://backend-worker:8081/metrics` | `WORKER_MONITOR_TOKEN` |
| Worker 状态 | `http://backend-worker:8081/status` | `WORKER_MONITOR_TOKEN` |
| Worker 探针 | `/livez`、`/startupz`、`/readyz`、`/health` | 无认证 |

Worker 监控默认绑定 `127.0.0.1:8081`。需要被容器或集群抓取时，设置 `WORKER_MONITOR_HOST=0.0.0.0`，并仅在 TLS 反代或受控内网后设置 `WORKER_MONITOR_ALLOW_PLAINTEXT=true`。

## 3. 配置监控令牌

为 API 和 Worker 使用不同的随机值：

```env
API_MONITOR_TOKEN=replace_with_random_api_monitor_token
WORKER_MONITOR_TOKEN=replace_with_random_worker_monitor_token
```

Docker Compose 中，`WORKER_MONITOR_TOKEN` 已映射到 Worker；启用 API Token 抓取时，还需在 `backend-api.environment` 中加入：

```yaml
API_MONITOR_TOKEN: ${API_MONITOR_TOKEN:?API_MONITOR_TOKEN is required}
```

Kubernetes 中应在 `caiyun-secrets` 增加 `api-monitor-token`，并向 API Deployment 注入：

```yaml
- name: API_MONITOR_TOKEN
  valueFrom:
    secretKeyRef:
      name: caiyun-secrets
      key: api-monitor-token
```

令牌不得出现在镜像、ConfigMap、Prometheus 仓库或公开日志中。

## 4. Prometheus 抓取示例

令牌推荐通过 Prometheus 的外部 Secret 挂载为文件：

```yaml
scrape_configs:
  - job_name: caiyun-api
    metrics_path: /metrics
    static_configs:
      - targets: ['backend-api:8080']
    authorization:
      type: Bearer
      credentials_file: /etc/prometheus/secrets/caiyun-api-token

  - job_name: caiyun-worker
    metrics_path: /metrics
    static_configs:
      - targets: ['backend-worker:8081']
    authorization:
      type: Bearer
      credentials_file: /etc/prometheus/secrets/caiyun-worker-token
```

单机部署时，将目标改为 Prometheus 可访问的内网地址；Kubernetes 可使用 ServiceMonitor，或通过 `kubernetes_sd_configs` 发现 Pod/Service。禁止将 8081 直接暴露到公网。

## 5. 加载告警规则

在 Prometheus 配置中引用规则文件：

```yaml
rule_files:
  - /etc/prometheus/rules/caiyun-prometheus-rules.yml
```

复制文件并检查配置：

```bash
cp deploy/monitoring/prometheus-rules.yml /etc/prometheus/rules/caiyun-prometheus-rules.yml
promtool check rules /etc/prometheus/rules/caiyun-prometheus-rules.yml
promtool check config /etc/prometheus/prometheus.yml
sudo systemctl reload prometheus
```

容器化 Prometheus 应将规则文件只读挂载，并在更新后调用 `/-/reload` 或滚动重启。

## 6. 导入 Grafana 看板

1. 在 Grafana 中配置 Prometheus 数据源。
2. 打开 **Dashboards → New → Import**。
3. 上传 `grafana-dashboard-caiyun.json`。
4. 选择对应 Prometheus 数据源并保存。
5. 验证时间范围、时区和变量是否匹配生产环境。

Compose 中 Grafana 默认映射到 `127.0.0.1:3000`。首次启动必须修改管理员密码，并通过 Nginx、VPN 或 SSH 隧道访问，不要直接开放公网端口。

## 7. 关键指标

历史归档相关指标包括：

- `caiyun_history_archive_runs_total{status=...}`
- `caiyun_history_archive_duration_seconds`
- `caiyun_history_archive_moved_rows_total{table=...}`
- `caiyun_history_archive_hit_batch_limit_total`
- `caiyun_history_archive_last_run_unix`
- `caiyun_history_archive_last_success_unix`

同时重点关注 API 请求量、5xx、P95/P99 延迟、限流拒绝、审计丢弃、Worker 心跳、任务成功率、队列 pending/processing/dead-letter 和 Redis/MySQL 容量。

## 8. 告警与容量建议

- 健康检查失败或 Worker 心跳中断：立即告警。
- API 5xx、P95 延迟、登录失败率持续升高：高优先级告警。
- 队列持续增长或 dead-letter 出现：检查 Worker、Redis 和外部接口。
- 历史归档超过 48 小时未成功或持续命中批次上限：检查数据库压力与归档参数。
- MySQL/Redis/PVC 磁盘超过 70% 预警、85% 严重告警。
- 告警必须包含环境、实例、版本、时间窗口和故障排查链接。

阈值应以实际流量基线和压测结果调整，避免直接照搬默认值。

## 9. 验证与排查

```bash
curl -fsS -H 'Authorization: Bearer API_TOKEN' http://127.0.0.1:8080/metrics | head
curl -fsS -H 'X-Monitor-Token: WORKER_TOKEN' http://127.0.0.1:8081/metrics | head
curl -fsS http://127.0.0.1:8080/readyz
curl -fsS http://127.0.0.1:8081/readyz
```

若 Prometheus 显示 `401/403`，检查令牌是否注入到对应进程；若显示连接失败，检查监听地址、防火墙、Service 和 NetworkPolicy；若端点正常但看板无数据，检查 Prometheus target、标签和 Grafana 数据源。

完整排查步骤见 [`../TROUBLESHOOTING.md`](../TROUBLESHOOTING.md)。