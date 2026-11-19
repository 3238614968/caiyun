# 使用咪咕音乐活动任务

咪咕音乐沿用当前项目的账号、任务目录、开关、批次、结果、日志和通知格式。新增 9 个任务项默认启用并加入批次；已有任务的定义与管理员开关保持原状。AI 音符使用 `availableNum` 核对真实余额，奖励提示数不作为累计资产，`cloud_gained` 保持 0。

## 已确认音符收益

运行日志的“已确认收益”分别显示云朵、音符，任务明细、手机端和结果通知同样保留两种单位。音符使用独立的 `migu_notes_gained` 字段持久化，按当天所有执行的确认收益求和；重试后的零收益结果不会覆盖已经到账的收益，也不会重复累计。

听歌仅统计本次领取接口返回金额、且复查为已领取的 SONG 档位。登录礼、签到、新人礼、社区、移动 APP、饭点及晚安打卡在实际提交前后查询稳定余额，只有两次查询均有效且余额增加才记录收益；任一次查询失败时不把当前余额当成本次收入。已经完成的任务不会再次记账。资产查询的余额和奖励提示均不计入新增收益。

升级时先停止 API、Worker，使用新二进制和服务相同的数据库配置执行：

```bash
./caiyun-linux migrate
./caiyun-linux migrate --validate-only
```

两条命令成功后再启动 API、Worker，并更新前端静态文件。迁移 `025_migu_note_rewards.sql` 为在线日志、归档日志增加音符字段，并补入旧听歌日志中明确记录的“本次AI音符”确认金额。例如“本次AI音符40”补记 40 音符；“AI音符余额134，本次奖励提示25”保留为资产信息，不将 134 或 25 当作新增收益。既有云朵字段、任务编码、开关和时间计划保持原状。

实现使用本地参考项目的客户端、接口契约和验证记录移植，并通过本地模拟接口测试。2026-11-15 实测核对了换票、任务状态、奖励余额与会话缓存，并修复了新人资格判断和奖励提示数统计。参考项目中兑换依赖真实风控验证，视频、评论、收藏、彩铃任务未打通，当前实现保留这些边界。

## 启用任务

1. 更新后端和前端。使用新版后端运行迁移，同步新增任务定义：

   ```bash
   cd backend
   go run ./cmd/caiyun migrate
   ```

   Compose 部署继续使用 `docker compose up --build -d`，由现有 `backend-migrate` 服务完成同步。

2. 重启 API 和 Worker，在管理页面检查下表任务的启用状态。现有账号的完整云盘 Authorization 自动用于出票，无需另行填咪咕密码或短信验证码。
3. 使用原有执行入口或等待每日批次。Worker 为饭点和晚安分别注册独立定时，只为已启用且加入批次的任务提交持久化操作；按账号、任务和北京时间日期去重。窗口内每 20 分钟补查派发，兼容 Worker 中途重启。排队超过窗口时，任务记录为待完成，不补交窗口外打卡。

| 任务编码 | 显示名称 | 执行与确认方式 |
| --- | --- | --- |
| `migu_login` | 咪咕登录即送 | 登录接入后复查完成次数；行为奖励自动结算，`rceiveTime` 展示字段不作为独立领奖门槛 |
| `migu_signin` | 咪咕每日签到 | 先查询新人礼状态的 `signTask`；为 `false` 时使用新人礼签到，为 `true` 时使用常规签到，均先查今日状态再提交一次并复查 |
| `migu_new_user` | 咪咕新人礼 | 条件签到；`signTask=true` 表示常规签到账号，不适用新人礼；`200013` 同样不再提交领取 |
| `migu_community` | 咪咕浏览社区 | 仅提交当前列表中存在且未完成的 `qsq`，再复查次数 |
| `migu_mobile` | 咪咕前往移动APP | 同上，使用 `zyd-sign` |
| `migu_noon` | 咪咕饭点打卡 | 北京时间 12:00–13:00 提交 `noon-check`，窗口外待完成 |
| `migu_night` | 咪咕晚安打卡 | 北京时间 22:00–23:00 提交 `night-check`，窗口外待完成 |
| `migu_song` | 咪咕听歌与领奖 | 查询已记账时长，按配置补报差额，复查后领取 `status=1` 的所有 SONG 档位，再复查 `status=2` |
| `migu_assets` | 咪咕金币与音符 | 核对 AI 音符余额、读取奖励提示数；提示读取后可能清零，不作为余额或累计收益；重复批次仍刷新 |

每日签到使用新人礼通道时保留 `migu_signin` 原任务编码。随后运行 `migu_new_user` 会查到今日已领，音符不会重复累计。读取参与条件失败时保留错误结果，上游明确拒绝参与时记录拒绝原因。可见行为任务若明确返回 `200013 / 不在任务时间范围内`，记录为待开放，本次不计收益。

接口受理但服务端状态未变化时使用现有 `pending` 状态，不把“提交成功”作为完成。普通行为任务自动结算，不调用 SONG 专用领奖接口。缺少有效状态、发生网络异常或上游拒绝时，不盲目重复提交。

`music-bean-accumulate.num` 对应 H5 的 `queryBeansCount` 奖励提示；`ai-bean/query-available.availableNum` 对应 `queryBeansTotal` 的稳定余额。实测前者从 13 变为 0 时，后者仍是 58，不表示资产减少或新收益为负。需要对账时使用 `NoteBalance()`，避免消耗奖励提示。

## 饭点与晚安定时

| 系统定时 | 原任务编码 | 北京时间计划 | Cron |
| --- | --- | --- | --- |
| `migu_noon_checkin` | `migu_noon` | 每天 12:00、12:20、12:40 补查 | `CRON_TZ=Asia/Shanghai */20 12 * * *` |
| `migu_night_checkin` | `migu_night` | 每天 22:00、22:20、22:40 补查 | `CRON_TZ=Asia/Shanghai */20 22 * * *` |

两个定时各用独立调度锁，沿用现有持久化操作和账号队列。重复补查使用同一账号、任务、北京时间日期的去重键；已经派发的操作不会因下一个二十分钟刻度再次执行。关闭对应任务或取消加入批次后，定时不再派发该项。

Cron 显式指定 `Asia/Shanghai`，宿主机使用 UTC 或其他时区时仍按北京时间触发。原有每日批次、任务名称、日志和结果字段继续使用。两个定时本身不需要额外 SQL；包含独立音符收益的新版本仍须先完成上文的 `migrate` 再重启 Worker。

## 配置活动和听歌行为

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `CAIYUN_MIGU_ACTIVITY_ID` | `MAC_KF_CBH_JBZX` | 由请求头传入；数字埋点 ID 不作为活动 ID |
| `CAIYUN_MIGU_SONG_MINUTES` | `30` | 0–30；0 仅领取账号已达标奖励，不上报时长 |
| `CAIYUN_MIGU_DEVICE_ID_手机号` | 按账号稳定生成 | 可选，16 位十六进制 Android ID；跨批次保持一致 |
| `CAIYUN_MIGU_IP_手机号` | `127.0.0.1` | 可选，写入 ce 的 IP 字段；接受 IPv4/IPv6 |

例如直接运行 Worker 时只领取已达标奖励：

```powershell
$env:CAIYUN_MIGU_SONG_MINUTES = '0'
go run ./cmd/caiyun worker
```

Compose 已传递前两个全局变量；按账号覆盖设备/IP时，需在 API/Worker 的环境映射中注入对应变量。Kubernetes 使用现有 `caiyun-config` ConfigMap。不要把真实登录票、会话凭证写入 `.env.example` 或提交到仓库。

## 会话换票与隔离

云盘 Basic Authorization → `querySpecToken(toSourceId=012008)` → 一次性票加 `ANDT` 前缀 → `/user/h5/token-validate/v3.0?sourceId=220029&type=2` → 响应头或数据中的 pacmtoken，以及 uid/msisdn。

会话放在账号任务存储中，按账号、云盘凭据摘要、设备 ID、IP 校验，最多复用 10 分钟。复用前使用 H5 用户信息接口探活，已失效时重新出票；凭据变更和缓存过期时重新换票。同一批次共享一次换票结果，出票失败不为每个子任务反复登录。执行中会话失效会清理缓存，下一次执行再换票，不重放已经开始的写操作。

咪咕请求单独构造请求头，禁用云盘默认头、Cookie Jar、重试和跳转，不覆盖云盘客户端的 Basic/JWT 或设备信息。票据只消费一次，不写入日志。登录后的 ce 使用账号原始手机号，听歌上报、查询和领奖保持同一 ce，不使用脱敏手机号或参考项目的固定账号。

## 参考依据和本地验证

接入时以参考项目中较晚的验证结论为准，出票和会话隔离方式见上节。

| 参考证据 | 接入结论 | 当前实现 |
| --- | --- | --- |
| `08_验证记录/验证总账.md` K1、L1 | 出票 `012008`、消费 `220029`、ANDT 前缀 | `core/api/migu.go` 的 `Login` |
| `04_脚本/protocol/mg_sign.py`、`02_分析/apk_cert_v2.hex` | V005 签名、证书前 32 个 hex 字符、公用 ce 编码 | `core/api/migu_sign.go`，9 个 Python 参考向量对照 |
| `07_任务福利分析/金币中心接口契约.md` §8–9、`09_完整客户端/migu_client.py` | 当前活动编码、任务 ID、时长差额与 SONG 领奖 | `core/tasks/migu.go` |
| `07_任务福利分析/h5_gold_coin/index.0f89b06a.js` | 饭点 12:00–13:00、晚安 22:00–23:00 | `core/tasks/migu.go`、`app/worker/migu_jobs.go` |
| `08_验证记录/验证总账.md` H、M | App 行为任务和风控兑换的现有边界 | 未接入自动兑换、视频、评论、收藏和彩铃 |

在后端目录运行：

```bash
go test ./internal/core/api ./internal/core/tasks ./internal/services ./internal/app/worker -run Migu -count=1
```

在前端目录运行：

```bash
npm run test:unit -- src/utils/task-types.test.ts
npm run typecheck
npm run lint
```

模拟接口覆盖签名边界、完整凭据换票、跨账号隔离、缓存更新、签到/新人资格、任务次数复查、北京时间窗、听歌差额/幂等领奖、独立资产统计及定时操作去重。线上是否到账以咪咕服务端记录为准。

显式线上验证使用 `TestMiguExistingAccountLive`，普通测试默认跳过。它读取 `CAIYUN_MIGU_LIVE_AUTH_FILE` 指定的现有 Basic 凭据文件，或含 `authorization` / `auth` 的 JSON 文件；要求显式代理 `CAIYUN_MIGU_LIVE_PROXY` 和绝对路径的脱敏报告 `CAIYUN_MIGU_LIVE_REPORT`。模式 `CAIYUN_MIGU_LIVE_MODE` 默认 `snapshot`，完成换票及加入活动后查询状态；设为 `execute` 才执行任务，记录原有结果字段、完成位、真实余额差和缓存复用结果。报告不保存票据、Cookie 或会话凭证。
