# 活动与奖励链路补齐开发说明

本文记录「移动云盘活动/任务覆盖度补齐」的开发计划、接口规格与实现边界。

2026-09-30 二次核对已修正重复 `/ycloud` 路径、平台响应解析、多账号配置和领奖流程。复核证据与部署要求见 [实现二次核对](IMPLEMENTATION_RECHECK.md)。账号绑定的配置统一使用 `_手机号` 后缀。

- 需求来源：逆向分析工作区（`中国移动云盘` mCloud 13.2.2）产出的
  `03_协议/全活动协议与参数手册.md`、`03_协议/隐藏活动与任务清单.md`、
  `03_协议/任务覆盖矩阵_A.md` / `_B.md`。
- 落地位置：`backend/internal/core/api/caiyun_*.go`（协议层）与
  `backend/internal/core/tasks/*.go`（任务层），通过
  `backend/internal/services/task_catalog.go` 注册为可配置任务。
- 设计约束：新增能力必须复用现有账号级 HTTP 客户端（Cookie/JWT/userDomainId），
  以保持多账号隔离、熔断、重试与链路追踪语义。

---

## 1. 覆盖度现状

补齐前，六大任务表（`sign_in_3`、`newsign_139mail`、`National_TokenPK`、
`National_MakeWish`、`National_PlayAISpecial`、`National_playAI`）与主要隐藏链路
（`signTask`、`api/prize/query|accept`、`openemailsms/*`、`springgift/*`、
`fivenewcomer/upgradeGifts/*`、`msgPushOn/*`、`backupgift/*`、妙云 `011`、扫描 `204`）
均已实现。

缺口分三类：

| 类别 | 处理方式 |
| --- | --- |
| A. 协议可达且不涉及伪造状态 | 本说明第 3 节，全部实现并注册为任务 |
| B. 需要设备本地真实状态 / 真人外部动作 | 只做「查询 + 如实上报」，写入路径用显式开关关闭（第 4 节） |
| C. 服务端未部署 / 已过期 / 风控闸门 | 实现探测接口，任务内如实报告结论，不视为失败（第 5 节） |

---

## 2. 缺口清单

| # | 缺口 | 上游端点 | 类别 |
| --- | --- | --- | --- |
| 1 | APP 通知已开启上报（551/1021 前提） | `GET /ycloud/signin/page/reportAppNoticeStatus?status=1` | A |
| 2 | 邮箱短信通知开关（1079 前提） | `POST /ycloud/openemailsms-service/openEmailsms/openSmsSwitch`、`GET /ycloud/signin/emailStatus/currEmailStatus` | A |
| 3 | 学生认证福利 `National_StudentPerks` | `/ycloud/simple/studentperks/{user/status,cert/sync,prize/claim,prize/check}` | A/B |
| 4 | 领奖专区 `National_Getprize` | `/ycloud/prize/api/{sendPrizeSms,acceptPrizeV2}`、`/ycloud/prizeApi/checkPrize/{getUserPrizeLogPageV2,receivePrizeDetailsV2,queryAcceptExt}` | A/B |
| 5 | 会员日 `National_MCloudDay` | `/ycloud/mcloudday/{common/activityInfo,gift/list,gift/verify,gift/receive,blindbox/lottery}` | A/C |
| 6 | 美图授权备份 `National_Meitubackup` | `/ycloud/meitu/new/{isRemind,openRemind,prizeCount,prizeList,authStatus,backUpStatus,prize,sendSms}` | B |
| 7 | 红包邀请 `National_Invitingtask` | `/ycloud/redInvite/page/{statMonthInfo,getInviteCode,getInviteState,acceptInvite,risk}` + RSA(PKCS1v15) | A/C |
| 8 | 1T 新礼 `newgifts1T` | `/market/unLoading/{userInfo,getPrizeRecords,getLinksList}`、`/market/sendPrize/<type>` | C |
| 9 | 抽奖码 | `/market/rafflecode/{info,list,getMyPrize,getRafflecodeData,getlotteryRecordV2}`（需 `jwtToken` 头） | C |
| 10 | 家庭圈任务 | `familyCircle/{queryCircleTaskState,queryBackupState}`（body 字段名是 `gruopId`） | B |
| 11 | AI Store 作品保存 / 授权 | `/aitools/uop/user/createUploadTask`、`/api/outer/assistant/accredit/{get,protocol/get,submit}` | A/B |
| 12 | 相册备份开关状态上报 | `POST middle.yun.139.com/openapi/albumAutoBackup/synStatus`（含平台头 + AES-128-CBC/IV 手机号） | B |
| 13 | 邮箱版趣玩AI `National_playAI139mail` | 与 `National_playAI` 共用后端，仅 `register/lottery/invite` | A |
| 14 | funai 邀请助力 | `/ycloud/funai/api/invite/{getcode,accept}` | C |
| 15 | 云盘焕新权益激活（100G 年卡） | `/ycloud/fivenewcomer/upgradeGifts/{getPrizePool,getPrize?smsCode=innerActivation}` | A |

---

## 3. 接口规格（类别 A）

以下按 `core/api` 文件组织。除特别标注外，均复用 `buildMarketHeaders` /
`buildActivityHeaders` 的鉴权头集合（`jwttoken` + `jwtToken` + `Referer`）。

### 3.1 `caiyun_notice.go`

| 方法 | 常量 | 说明 |
| --- | --- | --- |
| `ReportAppNoticeStatus(status)` | `MobileMarketURL/signin/page/reportAppNoticeStatus` | 按该账号提供的实际状态上报，`0` 关闭、`1` 开启；未提供实际值时跳过 |
| `GetEmailSmsSwitchStatus(ctx)` | `MobileMarketURL/signin/emailStatus/currEmailStatus` | 查询邮箱短信通知开关 |
| `OpenEmailSmsSwitch(ctx)` | `MobileMarketURL/openemailsms-service/openEmailsms/openSmsSwitch` | 开启邮箱短信通知开关（无 body） |
| `GetCloudNum(ctx)` | `MobileMarketURL/signin/page/getCloudNum` | 当前云朵数（只读） |

> `openSmsSwitch` 与既有 `ClaimOpenEmailSMSReward` 是**两个不同动作**：
> 老实现只领奖、没开开关；补齐后两者都调用，才能推动 1079 的连续天数。

### 3.2 `caiyun_studentperks.go`

```
GET  /ycloud/simple/studentperks/user/status
POST /ycloud/simple/studentperks/cert/sync      body {}
POST /ycloud/simple/studentperks/prize/claim    body {"smsCode": md5(明文短验码)}
GET  /ycloud/simple/studentperks/prize/check?openid=<openid>
```

状态字段：`certStatus`（0 未认证 / 1 已认证待领 / 2 已领取）、`certStatusDesc`、
`canCert`、`canClaim`、`prizeStatus`、`stockStatus`。

`cert/sync` 的语义是「从权威源（微信）同步已完成的认证结果」，请求体为空，
**无法凭空造出学生身份**；未认证账号同步后仍是 `certStatus=0`，属正常结果。

### 3.3 `caiyun_getprize.go`（领奖专区）

```
GET  /ycloud/prizeApi/checkPrize/getUserPrizeLogPageV2?currPage=&pageSize=
GET  /ycloud/prizeApi/checkPrize/receivePrizeDetailsV2?marketId=&prizeId=&drawRecode=
GET  /ycloud/prizeApi/checkPrize/queryAcceptExt?oid=
POST /ycloud/prize/api/sendPrizeSms   {"oid":..,"app":0,"puzzleOffset":<滑块偏移>}
POST /ycloud/prize/api/acceptPrizeV2  {"oid":..,"app":0,"sendDefaultSms":true,"smsCode":<MD5>}
```

`flag`：1 未领取 / 2 已领取 / 10 已失效。发码前必须过滑块（`puzzleOffset`），
服务端另有每日短信上限（`1002 今日短信发送已达上限`）。

### 3.4 `caiyun_mcloudday.go`（会员日）

```
GET  /ycloud/mcloudday/common/activityInfo
GET  /ycloud/mcloudday/gift/list
POST /ycloud/mcloudday/gift/verify
POST /ycloud/mcloudday/gift/receive
POST /ycloud/mcloudday/blindbox/lottery
```

`activityInfo` 返回 `online` / `extGiftOnline` / `blindboxOnline`，以业务接口为准。

### 3.5 `caiyun_redinvite.go`（红包邀请）

```
GET  /ycloud/redInvite/page/statMonthInfo
GET  /ycloud/redInvite/page/risk?marketName=National_Invitingtask
POST /ycloud/redInvite/page/getInviteCode  {"marketName":..,"op":"getInviteCode"}
POST /ycloud/redInvite/page/getInviteState
POST /ycloud/redInvite/page/acceptInvite
     {"op":"inviteNewUser","month":true,"inviteNumber":"<码>",
      "data": base64(RSA_PKCS1v15(JSON{"marketName":..,"encryptTime":<毫秒>}))}
```

RSA 公钥为 App 活动容器的 1024-bit 公钥，PKCS#1 v1.5，见 `caiyun_rsa.go`。
业务码：`3008` 疑似异常行为、`3009` 被邀请方 30 天内登录过 App、
`3010` 风控闸门；`risk` 接口回 `success:false, body:999` 时前端连 `acceptInvite` 都不会发。

### 3.6 `caiyun_aistore.go`（AI Store）

```
POST /aitools/uop/user/createUploadTask  {"module":<n>,"taskId":"","imageBase64":"data:image/jpeg;base64,.."}
POST /aitools/uop/user/createUploadTaskV2
POST /api/outer/assistant/accredit/get       {"sourceBusiness":1,"module":<n>}
POST /api/outer/assistant/accredit/protocol/get
POST /api/outer/assistant/accredit/submit
```

实测：`module` 1–14 的保存路径可用；`module=15`（朋友圈9图）恒回
`30101 云盘目录创建失败`，要求 App WebView JSBridge 上下文。

### 3.7 `caiyun_upgradegift.go`（焕新权益）

```
GET  /ycloud/fivenewcomer/upgradeGifts/getPrizePool
POST /ycloud/fivenewcomer/upgradeGifts/getPrize?smsCode=innerActivation
```

`innerActivation` 是活动侧提供的免验证码官方通道；无领奖记录时回
`404 未找到领奖记录`（不在白名单），属正常结果。

---

## 4. 类别 B：只读 + 显式开关

| 项 | 只读部分 | 写入部分 | 开关 |
| --- | --- | --- | --- |
| 学生认证领奖 | `user/status`、`cert/sync` | `prize/claim` | `CAIYUN_STUDENT_SMS_CODE`（未设置则跳过） |
| 领奖专区领奖 | 奖品清单、详情、`queryAcceptExt` | `acceptPrizeV2` | 按账号提供 `CAIYUN_PRIZE_OID` + 对应 `CAIYUN_PRIZE_SMS_CODE`；先经官方流程收到验证码，任务不重新发码 |
| 美图备份领奖 | `authStatus`、`backUpStatus`、`prizeCount`、`prizeList` | `prize` | 仅当两个前置状态均为已满足时调用 |
| 相册备份上报 | — | `albumAutoBackup/synStatus` | `CAIYUN_ALBUM_BACKUP_STATUS=0\|1`（未设置则只报告、不上报） |
| AI Store 保存 | `accredit/get` | `createUploadTask` | `CAIYUN_AISTORE_SAVE_MODULE`（未设置则只探测授权） |
| 家庭圈 | `queryBackupState`、`queryCircleTaskState` | — | 无（无群组时如实报告功能门槛） |

设计原则：**不向活动方提交与设备真实状态不符的数值，也不自动绕过滑块验证**。
这与逆向工作区对 `albumAutoBackup/synStatus`、`receiveV3` 的处理结论一致。
上述开关默认关闭，需要账号持有人明确提供真实值时才执行写入。

---

## 5. 类别 C：探测并如实报告

以下活动在逆向实测中已被判定为「服务端未部署 / 已过期 / 风控闸门」。
实现方式为**调用探测接口并把服务端回执写进任务消息**，不伪造成功、
也不因为服务端不可用而把整个任务批次判为失败：

| 项 | 预期回执 |
| --- | --- |
| 1T 新礼 `unload_once` | `503 远程调用失败` |
| 抽奖码 `rafflecodeConfigList` | 空数组（当前无场次） |
| 红包邀请 `risk` | `success:false, body:999` |
| funai 邀请助力 | `513 不满足助力条件` |
| 会员日盲盒 / 礼品 | `blindboxOnline:false`、`hasStock:false` |

---

## 6. 实现计划与文件清单

### 协议层 `internal/core/api/`

| 文件 | 内容 |
| --- | --- |
| `caiyun_notice.go` | 通知/开关类端点（3.1） |
| `caiyun_studentperks.go` | 学生认证族（3.2） |
| `caiyun_getprize.go` | 领奖专区（3.3） |
| `caiyun_mcloudday.go` | 会员日（3.4） |
| `caiyun_redinvite.go` | 红包邀请（3.5） |
| `caiyun_rsa.go` | App 活动容器 RSA 公钥签名（PKCS#1 v1.5） |
| `caiyun_aistore.go` | AI Store 授权/保存（3.6） |
| `caiyun_upgradegift.go` | 焕新权益奖池/激活（3.7） |
| `caiyun_unloading.go` | 1T 新礼探测 |
| `caiyun_rafflecode.go` | 抽奖码（`/market/*` 需 `jwtToken` 头） |
| `caiyun_familycircle.go` | 家庭圈任务状态 |
| `caiyun_meitu.go` | 美图授权备份 |
| `caiyun_openapi.go` | `middle.yun.139.com/openapi` 平台头 + AES-128-CBC/IV 加密 |
| `caiyun_helpers.go` | 活动层通用请求/解析辅助（既有 `hiddenRewardRequest` 复用同一实现） |

> 邮箱版趣玩AI 未单独建文件：`caiyun_funai.go` 已改为活动标识可传参
> （`FunaiPageModuleForMarket` / `FunaiRegisterForMarket` / `FunaiLotteryForMarket`），
> 并新增 `FunaiInviteCode` / `FunaiAcceptInvite`。

### 任务层 `internal/core/tasks/`

| 文件 | 任务码 | 默认批次 | 说明 |
| --- | --- | --- | --- |
| `notice_switch.go` | `notice_switch` | 是 | 有实际账号状态时上报 APP 通知 + 开邮箱短信开关（1、2） |
| `studentperks.go` | `student_perks` | 是（只读+同步） | 学生认证状态（3） |
| `getprize.go` | `prize_center` | 是（盘点） | 领奖专区清单（4） |
| `mcloudday.go` | `mcloud_day` | 否 | 会员日（5） |
| `meitu_backup.go` | `meitu_backup` | 否 | 美图备份（6） |
| `redinvite.go` | `red_invite` | 否 | 红包邀请（7） |
| `unloading.go` | `unloading_1t` | 否 | 1T 新礼（8） |
| `rafflecode.go` | `rafflecode` | 否 | 抽奖码（9） |
| `familycircle.go` | `family_circle` | 否 | 家庭圈（10） |
| `aistore.go` | `ai_store` | 否 | AI Store 保存/授权（11） |
| `album_backup_report.go` | `album_backup_report` | 否 | 相册备份上报（12） |
| `funai_mail.go` | `fun_ai_mail` | 否 | 邮箱版趣玩AI（13） |
| `upgradegift.go` | `upgrade_gift` | 否 | 焕新权益奖池/激活（15，`hidden_rewards` 已含激活，默认不重复执行） |

`task_runner_completion_tasks.go` 提供对应的 `runXxxTask()`；`task_catalog.go`
在 `defs` 中注册（含中文名、排序、别名、默认启停与是否进批次）。
单元测试见 `internal/core/api/caiyun_*_test.go`、
`internal/core/tasks/completion_tasks_test.go`、
`internal/services/task_catalog_completion_test.go`。

### 配置项

| 环境变量 | 默认 | 用途 |
| --- | --- | --- |
| `CAIYUN_APP_NOTICE_STATUS_手机号` | 空 | 该账号 APP 通知实际开关，严格 `0`/`1` |
| `CAIYUN_STUDENT_SMS_CODE_手机号` | 空 | 该账号学生认证领奖验证码（明文，内部取 MD5） |
| `CAIYUN_PRIZE_OID_手机号` | 空 | 要领取的具体奖品 OID |
| `CAIYUN_PRIZE_SMS_CODE_手机号` | 空 | 同一账号、同一 OID 的已收到验证码 |
| `CAIYUN_ALBUM_BACKUP_STATUS_手机号` | 空 | 该账号相册备份真实开关，严格 `0`/`1` |
| `CAIYUN_AISTORE_SAVE_MODULE_手机号` | 空 | 该账号需要保存作品的 AI Store 模块号 |
| `CAIYUN_REDINVITE_CODE_手机号` | 空 | 该账号要接受的红包邀请码 |
| `CAIYUN_UNLOAD_CODE_手机号` | 空 | 该账号 1T 新礼兑换码 |
| `CAIYUN_UNLOAD_TYPE_手机号` | `1` | 该账号 1T 新礼奖励类型 |
| `CAIYUN_FAMILY_GROUP_ID_手机号` | 空 | 该账号家庭圈群组 ID |

---

## 7. 验收

```bash
cd backend
gofmt -l ./internal/core ./internal/services
go build ./...
go vet ./...
go test ./internal/core/... ./internal/services/...
```

单元测试覆盖纯逻辑：RSA 签名格式、AES-128-CBC/IV 加密可解密、
`middle.yun.139.com` 平台头完整性、奖品排序与状态映射、学生认证状态映射。

真实链路验证需要账号凭据，按 `deploy/` 中既有流程在测试环境执行；
本说明涉及的写操作默认关闭，验证时通过上述环境变量显式开启。

---

## 8. 明确不做的项

| 项 | 原因 |
| --- | --- |
| 自动绕过滑块（`receiveV3`、`exchangeV3`、领奖专区发码） | 反自动化控制，绕过等同规避风控 |
| 伪造相册备份开关为 `1` | 该端点语义是上报设备真实状态，伪造即向活动方提交虚假状态 |
| 伪造学生认证 | `cert/sync` 从权威源同步，无真实认证无法产生结果 |
| 伪造 `548/550` 备份/通知完成 | 需要真实设备动作 |
| `makewish 1008` 状态翻牌 | 服务端不以下单作为完成条件（保留兑换本身） |
| 收费订购类接口（`openVip`、`CaiyunViporderPop`） | 产生真实扣费 |
