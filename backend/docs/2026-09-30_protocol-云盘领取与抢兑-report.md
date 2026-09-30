# 移动云盘真实抓包与 V3 领取、兑换对照

## 摘要与范围

依据用户提供的《移动云盘领取奖励和真实抢兑.har》修复请求协议和结果确认。授权及网络范围见 [scope.md](har-reward-exchange/scope.md)，过程见 [timeline.md](har-reward-exchange/timeline.md)。报告类型为协议兼容分析，`flavor = null`。

旧代码的兑换端点、方法、设备参数及云朵领奖流程与真实 App 不一致。抓包中的成功兑换商品是「移动云盘100万 tokens 叠加包」，没有 QQ 音乐绿钻月卡的成功样本。会员奖品最终领奖按用户要求留待后续。

## Evidence

原始 HAR 共 1,875 条记录、45,311,998 字节，SHA256：`583de610e5f89a9569fef6485ca68002cc374c9ecb7a17a473c0e0aab275eec4`。原始文件仅在用户本地保留，以下索引从 0 开始。

| E-id | source_ref | 观察 | repro_command | content_hash |
| --- | --- | --- | --- | --- |
| E-01 | HAR entries[1794,1798] | 滑块后 POST exchangeV3；JSON 包含 prizeId、client、clientVersion、puzzleOffset、smsCode、deviceId；isDeviceId=true；回包包含 code=0 和 result.oid | `cd backend && go test ./internal/services -run ExchangeV3` | 上述 HAR SHA256 |
| E-02 | HAR entries[1375,1522,1523] | infoV3.receiveList.recordId 映射为 receiveV3.cloudId；领取后记录消失，余额增加 | `cd backend && go test ./internal/core/api -run ReceiveV3` | 上述 HAR SHA256 |
| E-03 | HAR 的 15 次 receiveV3、entries[1677] | 领取数为 5、6、30、5、60、30、100、5、6、5、6、6、6、5、200，合计 475；待领数量从 475 降至 0 | `cd backend && go test ./internal/core/api -run CapturedFifteen` | 上述 HAR SHA256 |
| E-04 | HAR entries[1370,1854] | V3 任务状态与云朵领取记录分离；兑换后奖品 flag=1，查看详情不代表最终领奖 | `cd backend && go test ./internal/core/tasks -run TaskListRunsV3` | 上述 HAR SHA256 |

测试使用合成记录 ID 和虚构凭据，复现协议结构与状态变化，不包含原始登录数据。

## Findings

| F-id | severity | evidence_ids | confidence | location | status |
| --- | --- | --- | --- | --- | --- |
| F-01 | n/a_re | E-01 | 高 | services/exchange_executor.go | 已修复：GET exchangeV2 改为 POST exchangeV3，参数放入 JSON，补充设备标识与 isDeviceId |
| F-02 | n/a_re | E-02,E-03 | 高 | core/api/caiyun_receive*.go、caiyun.go | 已修复：读取 recordId，逐项领取并确认回包、记录消失和余额 |
| F-03 | n/a_re | E-04 | 高 | core/tasks/tasklist.go | 已修复：V3 领奖不依赖任务按钮 canReceive 或误用 taskId；执行任务前后领取奖励气泡 |
| F-04 | n/a_re | E-02,E-03 | 高 | services/task_runner_extended_tasks.go、task_service_execute.go | 已修复：记录实际到账，保留部分成功收益，避免余额差额重复分配 |
| F-05 | n/a_re | E-01,E-04 | 高 | services/exchange_executor.go | 已修复：缺少业务码/奖品记录或商品不一致时返回「结果待确认」，停止自动重试 |

证据证明请求协议存在差异，不能独立证明 QQ 音乐月卡的全部 `610/GK` 业务拒绝都由端点问题引起。库存、账号资格和服务端限制以部署后的真实回包为准。

## Path

### P-01：兑换调用链，path_type=callflow

同账号 JWT/SSO 和 HTTP 会话 → 获取滑块 → 识别原图坐标 → POST exchangeV3（clientVersion=13.2.2、JSON deviceId、isDeviceId=true）→ 检查业务码、返回商品及奖品记录 → 记录兑换结果。

### P-02：云朵领取调用链，path_type=callflow

准备当前账号会话 → GET infoV3 → 提取 cloudType=0 且有 recordId 的记录 → 每项 POST receiveV3 → 读取实际 receive 与 total → 再 GET infoV3 验证记录消失、余额与回包一致 → 汇总实际到账和剩余待领。

cloudType=2 等没有领取 ID 的汇总项不作为领取请求，下月等其他类型不按本轮即时奖励处理。重复检查重新读取列表，已领取记录不会再次提交。单次最多处理 200 个记录，存在残留时返回错误。

## 验证及部署

新增回归覆盖：15 项/475 云朵、空待领列表、重复执行、多账号隔离、大整数记录 ID、部分领取后恢复、成功但记录仍存在、列表缺失、余额未增加、V3 任务继续执行及领取、不确定兑换回包。

```bash
cd backend
go test ./... -timeout 3m
go vet ./...
```

本轮无需新增 SQL 迁移。API 和 Worker 使用同一版本后端制品并重启；现有待领记录由 `receive` 或任务中心任务重新读取和领取。前端领奖专区继续展示奖品及指引，最终奖品领取后续实现。

旧日志不会被本轮修复改写。验证新版本需查看新执行记录。本轮验证采用本地模拟接口，未消耗真实账号云朵或提交真实兑换。
