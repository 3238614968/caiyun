# 2026-09-30 部署后接口错误修复

## 已确认的原因

服务端日志显示 `task_log_repo.go` 预加载 `accounts` 后触发凭据解密钩子，返回 `cipher: message authentication failed`，随后 `/api/v1/tasks/status` 返回 500。抢兑规则、任务列表也会读取完整账号或规则，从而受到同类凭据问题影响。

此错误可能来自旧版无 AAD 的 AES-GCM 密文，也可能来自加密密钥不一致或密文损坏。现有日志不能区分这三种情况。AES-GCM 的认证不能跳过，也不能通过重新生成密钥恢复旧数据。

领奖服务此前把账号读取的所有错误统一映射成 404，掩盖了凭据读取失败。API 的 OpenTelemetry HTTP 包装器还会在写正文、刷新流时重复调用 `WriteHeader(200)`，产生大量警告；它与凭据解密错误是独立问题。

## 修复内容

- 日志、抢兑规则和抢兑任务列表只读取账号/规则的展示字段，保持关联账号、商品信息，不加载凭据。
- 日志账号归属校验和领奖账号归属校验使用独立的元数据查询，仍保留用户隔离。
- 领奖服务保留数据库错误；凭据无法解密时返回 409 和可操作的提示，前端显示该提示。
- 修复 `reencrypt`：版本号相同时还验证 AAD，旧格式密文照常重加密；新格式密文不重复写入。
- 修复重复写响应头，保留 HTTP 追踪、SSE 和 WebSocket 能力。
- 内部错误日志记录请求 ID，便于与客户端 `trace_id` 对应。

## 现有服务器恢复步骤

替换 API 和 Worker 使用的 `/opt/caiyun/bin/caiyun-linux`，并同步更新前端。下列命令必须在原来的配置目录执行，或通过现有运行方式加载同一份数据库和加密配置。不要修改已有加密密钥。

先查看新版本和只读扫描结果：

```bash
chmod +x /opt/caiyun/bin/caiyun-linux
/opt/caiyun/bin/caiyun-linux version
/opt/caiyun/bin/caiyun-linux reencrypt
```

默认 `reencrypt` 是 dry-run，不写数据库。新版能够识别版本仍为 `v1` 的旧格式密文。

如果扫描成功，并报告存在需要迁移的字段，备份数据库后执行：

```bash
/opt/caiyun/bin/caiyun-linux reencrypt --apply
/opt/caiyun/bin/caiyun-linux reencrypt
```

第二次扫描应报告没有需要重加密的数据，然后使用原有服务管理器重新启动 API、Worker。若之前临时设置过 `FIELD_CRYPTO_ALLOW_LEGACY_NO_AAD=true`，迁移后删除或设为 `false` 并重启。

如果选择保留旧格式凭据，可在 API 和 Worker 的现有环境配置中启用 `FIELD_CRYPTO_ALLOW_LEGACY_NO_AAD=true` 并重启，无需执行 `reencrypt`。开启后支持读取新旧密文，新写入仍使用带 AAD 的新格式；继续保留原始密钥和版本。这个选项仍要求使用正确密钥并通过 AES-GCM 认证，不接受错误密钥或损坏密文。以后选择迁移时，再执行上述扫描与重加密步骤。

如果 dry-run 仍报告认证失败，停止写入迁移，核对历史数据对应的 `DATA_ENCRYPTION_KEYS`、版本及原始密钥。不得删除账号凭据或用新密钥覆盖历史数据来绕过错误。

## 验证

本地回归覆盖：历史凭据不可读时的列表查询、账号隔离、领奖错误映射与恢复、同版本无 AAD 密文迁移及幂等性、HTTP 错误状态、SSE 刷新和 WebSocket writer 能力。真实服务器的旧密文兼容性以 dry-run 结果为准。

一次浏览器 401 还不足以证明登录续期失败。用户提供的 Cookie 是网站会话凭据，与云盘账号的数据库凭据是两套机制；本次已确认的 500 来自后者。若迁移后仍有持续 401，应另行核对 `/api/v1/auth/refresh` 的响应。
