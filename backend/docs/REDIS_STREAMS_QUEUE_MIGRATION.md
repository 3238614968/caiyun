# Redis Streams 队列迁移评估

项目默认使用 Redis List 可靠队列；可通过环境变量切换到 Redis Streams：

```env
TASK_QUEUE_BACKEND=streams
```

## List 后端特点

优点：

- 结构简单，兼容性好。
- `BRPOPLPUSH` 能原子地把任务从 pending 移到 processing。
- 适合单 Worker 或少量 Worker 场景。

注意点：

- processing 恢复和延迟队列提升需要维护循环。
- 多步转移需要谨慎处理失败恢复。

## Streams 后端特点

优点：

- 原生 consumer group，适合多 Worker 横向扩展。
- pending entry list 能更自然地表达未确认消息。
- `XAUTOCLAIM` 适合恢复超时消息。

注意点：

- 运维复杂度略高，需要关注 stream 长度和 consumer group 状态。
- 需要 Redis 版本支持相关命令。

## 建议迁移路径

1. 生产默认使用 `TASK_QUEUE_BACKEND=streams`；仅回滚或排障时临时使用 `list`。
2. 在测试环境开启 `TASK_QUEUE_BACKEND=streams`。
3. 跑真实 Redis 集成测试：

```bash
CAIYUN_REDIS_INTEGRATION=1 TASK_QUEUE_BACKEND=streams go test ./internal/queue -run RedisIntegration -count=1
```

4. 观察队列状态接口、Worker 日志、死信数量和任务重复率。
5. 生产灰度时先单副本 Worker，再逐步扩大副本数。

如果任务吞吐和多副本可靠性要求继续提高，推荐将 Streams 作为生产默认后端。
