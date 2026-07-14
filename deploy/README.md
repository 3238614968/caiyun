# 部署与运维指南

本文档是移动云盘管理系统的部署入口。后端使用统一制品 `caiyun-linux`，通过 `api`、`worker`、`migrate` 和 `reencrypt` 子命令承担不同职责。生产环境应将 API、Worker 和数据库迁移作为独立运行单元管理。

## 部署方式选择

| 场景 | 推荐方式 | 数据组件 | 适用范围 |
| --- | --- | --- | --- |
| 本地验证、演示、小规模单机 | [Docker Compose](./DOCKER_COMPOSE.md) | Compose 内置 MySQL、Redis | 最快完成整套环境部署 |
| 已有容器平台或外部数据库 | [独立 Docker 容器](./DOCKER_STANDALONE.md) | 内置或外部 MySQL、Redis | 需要独立镜像、网络和生命周期控制 |
| Linux 生产服务器 | [单机 systemd 部署](./SINGLE_HOST.md) | 本机或托管 MySQL、Redis | 进程托管、Nginx、备份与明确回滚路径 |
| 开发、联调、预发布 | [源码部署](./SOURCE_DEPLOYMENT.md) | 本机或容器化依赖 | 需要调试、热更新或逐组件启动 |
| 多副本与集群调度 | [Kubernetes](./KUBERNETES.md) | 清单内置或外部托管服务 | 滚动升级、资源约束、探针和高可用 |

## 专题文档

- [Nginx、HTTPS 与实时推送](./NGINX_TLS.md)
- [版本升级与回滚](./UPGRADE_ROLLBACK.md)
- [备份与恢复](./BACKUP_RESTORE.md)
- [生产故障排查](./TROUBLESHOOTING.md)
- [SSE/CDN 部署](./CDN_SSE.md)
- [WebSocket/CDN 说明](./BAIDU_CDN_WEBSOCKET.md)
- [Prometheus/Grafana](./monitoring/README.md)
- [生产运行检查清单](../backend/docs/OPERATIONS_CHECKLIST.md)
- [数据加密密钥轮换](../backend/docs/ENCRYPTION_ROTATION.md)

## 统一运行模型

```text
caiyun-linux migrate [--validate-only]
caiyun-linux api
caiyun-linux worker
caiyun-linux reencrypt [--apply]
caiyun-linux version
```

标准发布顺序：

1. 备份 MySQL、生产 `.env`、数据加密密钥和当前制品。
2. 校验新制品的 `SHA256SUMS`，有签名时同时验证签名。
3. 使用新版本后端执行 `migrate`。
4. 执行 `migrate --validate-only` 验证数据库结构。
5. 使用同一版本启动或滚动更新 API 与 Worker。
6. 发布前端静态文件或前端镜像。
7. 验证 `/startupz`、`/readyz`、`/livez`、SSE 和业务任务。
8. 保留旧制品、数据库备份和部署日志，直至观察期结束。

## 生产配置基线

- `APP_ENV=production`
- `DB_AUTO_MIGRATE=false`
- `RATE_LIMIT_BACKEND=redis`
- `TASK_QUEUE_BACKEND=streams`
- `LOG_JSON_FORMAT=true`
- `JWT_SECRET`、数据库密码、Redis 密码和监控 Token 使用独立强随机值。
- `DATA_ENCRYPTION_KEYS` 与 `DATA_ENCRYPTION_CURRENT_VERSION` 必须配套保存；密钥丢失会导致已加密字段不可读。
- `ALLOWED_ORIGINS` 仅包含实际前端 Origin。
- `TRUSTED_PROXIES` 仅信任实际反向代理 IP/CIDR；直连使用 `none`。
- 数据库、Redis、API 内部端口和 Worker 监控端口不直接暴露到公网。

## 容量与高可用原则

- API 可横向扩容，共享 MySQL、Redis、限流状态和推送事件通道。
- Worker 扩容前应评估 `TASK_CONCURRENCY`、上游限流和数据库连接数。
- MySQL 是主要业务数据源，应优先使用可靠磁盘、定期备份和恢复演练。
- Redis 保存队列、锁、限流和事件状态，应启用持久化并监控内存、延迟和队列积压。
- 数据库迁移只执行一次，并作为 API/Worker 更新前的发布门禁。

## 官方技术参考

- [Docker Compose 生产部署](https://docs.docker.com/compose/how-tos/production/)
- [Kubernetes Deployment](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/)
- [Kubernetes 探针](https://kubernetes.io/docs/concepts/configuration/liveness-readiness-startup-probes/)
- [systemd.service](https://www.freedesktop.org/software/systemd/man/latest/systemd.service.html)
- [Nginx 代理模块](https://nginx.org/en/docs/http/ngx_http_proxy_module.html)
