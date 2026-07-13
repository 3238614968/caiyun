# Redis 队列集成测试

本项目的任务队列支持两种 Redis 后端：

- `list`：默认后端，使用 pending / processing / delayed / dead-letter 多结构实现可靠队列。
- `streams`：可选后端，使用 Redis Streams + consumer group。

## 默认单元测试

普通单元测试不依赖真实 Redis：

```bash
cd backend
go test ./internal/queue
```

## 真实 Redis 集成测试

需要本机或测试环境有 Redis：

```bash
cd backend
CAIYUN_REDIS_INTEGRATION=1 \
CAIYUN_TEST_REDIS_ADDR=127.0.0.1:6379 \
CAIYUN_TEST_REDIS_DB=15 \
go test ./internal/queue -run RedisIntegration -count=1
```

Windows/PowerShell：

```powershell
.\scripts\ci-local.ps1 -SkipInstall -WithRedisIntegration -RedisAddr 127.0.0.1:6379 -RedisDB 15
```

## 覆盖场景

集成测试覆盖：

- 入队、出队、确认 ACK。
- processing 可见性超时恢复。
- Redis List 后端使用 Lua 将 processing→pending / delayed / dead-letter 等状态转移原子化，避免 remove-then-push 间隙。
- 延迟队列到期提升。
- 超过重试次数进入死信队列。
- List 与 Streams 后端的关键生命周期一致性。

建议使用独立测试 DB，例如 `CAIYUN_TEST_REDIS_DB=15`，避免清理生产或开发数据。
