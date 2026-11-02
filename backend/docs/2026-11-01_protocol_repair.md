# 2026-11-01 协议变更修复

依据 `2026-11-01_protocol_breakage_repair_strategy.md` 及参考项目的 13.2.5 差分报告、`Account.login()` 和 `verify_1325_live.py` 完成协议更新。

## 已实施

- 普通 JWT 登录及抢兑独立 H5 会话都按 `m.mcloud.139.com → gp.mcloud.139.com` 回退。两个端点均发送原 MD5 签名头、数字 `openAccount: 0`、`marketName: sign_in_3`、`sourceId: 1097`，保留调用方的 Cookie Jar，并要求明确的 `code: 0` 和非空 token。
- `QuerySpecToken` 与 JWT 出票统一使用 orches POST `{account, toSourceId}`。移除 JWT 候选中的 PC `.action` 路径，保留仍可用的 tellin 刷新接口及 `:7071/market/*`。
- AI 对话使用 `Accept: text/event-stream, application/json`；图像接口继续使用 JSON。对话成功判定解析 JSON/SSE 字段并精确比较 `code`，同时验证 HTTP 状态，避免错误码 `01000002`、`01000001` 命中成功子串。
- 短信发送及验证码验证默认拒绝为已有活跃、未过期凭据的手机号重新登录；验证时再次检查。新增可选请求字段 `force`（默认 false），界面须明确勾选强制替换。生产查询覆盖同一手机号在不同平台用户下的凭据，且只读取元数据。
- 短信验证到保存、手动刷新、Worker 失败换证共用手机号互斥锁；有 Redis 时同时获取分布式锁，Redis 锁失败不继续换证。锁内换证请求限制两分钟，租约三分钟，保存完成后释放。
- 取消提前五天轮换 authorization。出票成功即复用现有凭据；仅使用失败且确认失效或到期时尝试恢复。`05050006` 识别为凭据失效，不重复出票，不回退到旧 JWT；恢复失败暂停该账号并提示重新登录。保留 tellin `4006` 的失效语义。
- 云手机任务默认关闭、移出批次；历史已启用配置也会被过滤，单独触发返回 skipped，避免继续访问尚未打通的新登录链。
- 活动版本串更新至 `13.2.5` / `13.2.5.0`，对应 WebView UA 同步。短信取证和设备 SDK 的既有 12.5.4 协议参数保留。

## 验证与上线

回归用例覆盖网关 404、业务失败、网络失败、缺失 code 的回退，签名和数字字段，orches 出票，失效码分类，同手机号并发、跨用户保护、显式 force、不同手机号独立登录，以及刷新路径与短信登录共享互斥锁。前端浏览器用例检查发送及验证请求的 force 值。

执行 `go test ./...`、`go vet ./...`、前端 `typecheck`、`lint`、`test:unit`、`build`，及 `playwright test e2e/sms-login-rotation.spec.ts`。本机 Go 缓存使用临时目录；race 测试需要额外提供可用的 CGO/C 编译器。

没有新增数据库表或 SQL 迁移。API、Worker 和前端需要一起发布；可按现有流程执行 migrate 同步任务定义。历史云手机启用标记即使保留，也不会再进入批次。

本次本地测试使用模拟上游，未使用真实账号重新发送短信、换证或执行领奖/抢兑，也未部署线上服务。上线后仍需使用当前有效凭据进行只读验收：orches 出票 → gp JWT → taskListV3 → infoV3，并核对 API、Worker 的 build 标记一致。

待领豆 615 设备锁、抢兑 GK 610 以及云手机新 SSO 尚未解决，沿用原诊断边界。知识库附件模型等参考报告另列的变更不能由本次 Accept 更新证明恢复；最终完成情况以服务端任务状态为准。
