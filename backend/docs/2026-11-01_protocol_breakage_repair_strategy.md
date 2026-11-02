# 2026-11-01 服务端协议变更 · 失效诊断与修复策略

> 背景:2026-11-01 全量实测(参考账号 182****7646,新鲜 SMS 登录凭据),发现移动云盘服务端
> 在 13.2.2→13.2.5 过渡期做了一批**网关迁移与请求形态变更**,本平台多条主链路正中变更点。
> 本文只给**诊断与修复策略**(含已验证的黄金样本),不给实现;修复由后续任务执行。
> 所有结论均有当日实测证据,佐证材料见姊妹项目
> `D:\Project\Python\逆向\app\中国移动云盘\05_归档\版本差分_13.2.5_20261101.md`。

---

## 一、失效根因总览(按优先级)

| # | 服务端变更(2026-11-01 实测) | 本平台受影响面 | 优先级 |
|---|---|---|---|
| R1 | **auth-service tyrzLogin 迁移**:`m.mcloud.139.com/ycloud/auth-service/auth/tyrzLogin` → **404**;同路径在 **gp.mcloud.139.com 存活**(需 MD5 签名头,盐不变 `sekaMdYYLIZfbCfm`;body 的 `openAccount` 必须是**数字 0**) | `internal/core/auth/h5_session.go:50`、`internal/core/auth/app_refresh.go:244` —— **JWT 获取主链,全部任务/统计/领奖随之一并停摆** | **P0** |
| R2 | **caiyun `/portal/auth/*` 整族 502**(整条路由后端故障/下线;同域 `:7071/market/*` **仍活**,勿误伤) | `internal/core/auth/auth.go:179 QuerySpecToken(.action)`、`app_refresh.go` 的 PC 回退链、`blindbox.go`/`cloudbattle.go` 内引用 | **P0** |
| R3 | **`chat/v2/add` 的 Accept 改为组合值** `text/event-stream, application/json`(单发 event-stream → **HTTP 406 + 01000002**) | `internal/core/api/caiyun_ai.go:37`、`internal/core/tasks/redpacket.go:265` —— AI 对话族(灵犀/带图问AI/知识库对话)全哑 | **P1** |
| R4 | **RCS 凭据单活轮换**:每次 thirdlogin 成功即作废上一枚 cyToken | 多账号托管的取 CK 流程:若"刷新/登录"与"在用"并发,在用凭据会被轮换死(错误码 `05050006`,客户端常量 `TOKEN_EXPIRES_ERROR_IN_USER_DOMAIN`) | **P1(运营约束)** |
| R5 | **云手机红包派对迁移**:`cpactiv.buy.139.com` → `cloud.139.com` 独立 SPA;旧 `tokenValidate` 回 `401 应用未授权`;新登录参数 `{version:"1.2", pintype:5}`(实测仍 401,新链未通) | `internal/core/api/caiyun_cloudphone.go` 整组 | P2(先禁用,择期逆向) |
| R6 | taskListV3 顶层数组信封 | `decodeTaskListV3` **本来就按 result 数组解析,不受影响**(已核对) | 无需修 |
| R7 | 待领豆 615 设备锁 / 抢兑 GK 610 | `receiveV3`/`exchangeV3` | **非本次新增失效**,沿用既有档案结论(见 §4) |

---

## 二、逐项修复策略

### P0-1 · JWT 链切换到 gp 网关(一行级修复,收益最大)

- **改法**:`h5_session.go:50` 与 `app_refresh.go:244` 的 URL 域名 `m.mcloud.139.com` → `gp.mcloud.139.com`,其余全部不动:
  - 签名公式 `MD5(salt + requestId + timestamp + nonce + queryString + salt)` **未变**;
  - 盐 `sekaMdYYLIZfbCfm` **未变**(现行服务端 bundle charCode 数组原样,已解出比对);
  - body `{token, openAccount:0(数字), marketName:"sign_in_3", sourceId:"1097"}`——平台自用 sourceId 1097 建议保留(实测 001005 亦通,判定在 Authorization 层)。
- **稳妥形态**:按"m 先试、404/失败回落 gp"的双端点兜底(参考姊妹项目 06/07 青龙脚本已落地的
  `Account.login()` 补丁),防服务端回滚。
- **黄金用例(2026-11-01 19:00 实测通过)**:
  `POST gp.mcloud.139.com/ycloud/auth-service/auth/tyrzLogin`,签名头齐全 →
  `{"code":0,"msg":"success","result":{"token":"eyJhbGciOiJIUzI1NiJ9…"(905字符),"userDomainId":…}}`。
- **验收**:出票(orches)→ gp tyrzLogin → JWT → taskListV3(带真实 state)→ infoV3 全链 200。

### P0-2 · PC `.action` 链退役,orches 为主

- `auth.go QuerySpecToken`(`querySpecToken.action?phone=`)属 502 死族:**降级为最后回退或直接删除**;
  主路径统一走 `orches.yun.139.com/orchestration/auth-rebuild/token/v1.0/querySpecToken`
  (POST `{account, toSourceId}`,app_refresh.go 已有实现,实测仍活)。
- **注意**:`aas.caiyun.feixin.10086.cn/tellin/authTokenRefresh.do`(auth.go:134)当日实测**仍活**
  (回 4006 判据可用),可保留作凭据死活探针。
- **误伤提醒**:`:7071/market/*`(msgPushOn/signTask/backupgift)实测**活着**,不要连带改动。

### P1-1 · AI chat Accept 组合值

- `caiyun_ai.go:37`、`redpacket.go:265`:`"text/event-stream"` → `"text/event-stream, application/json"`。
- **附带审查**:成功判据若存在 `"0000" in body` 类**子串匹配**,会被 `01000002/01000001` 误命中
  (姊妹项目刚因此掩盖了多轮 406 失败)——统一改为解析 JSON 的 `code` 字段精确比对。
- 验收:纯文本 `chat/v2/add` 回 200 且 SSE 流含 `"success":true`。

### P1-2 · 凭据单活轮换的运营约束

- 触发面:平台"添加/更新账号凭据"若走 SMS 登录,**每成功一次即作废该账号当前在用 CK**。
- 策略:
  1. 登录编排加**互斥**:某账号存在"活跃且未过期"凭据时,禁止再触发该账号的新登录(除非显式 force);
  2. 错误码 `05050006` 纳入"凭据已死"判定清单(服务端文案"暂无权限"是误导,客户端常量表证实其义为
     TOKEN_EXPIRES,App 收到即踢回登录页);
  3. 凭据到期前**不主动轮换**,只在使用失败后再走换新(减少无谓作废)。

### P2 · 云手机(cloudphone)整组处置

- 短期:任务组**默认禁用**(避免每次批次的无效尝试污染日志/风控计数)。
- 择期逆向新链,要点已踩好:
  - 新 SPA:`cloud.139.com`(独立「移动云手机」Web 版,主包 `app.6d14d906.js` 已落盘于姊妹项目
    `work/cloud139/`,可直接取用);
  - 新登录参数:`tokenValidate {version:"1.2", pintype:5}`(**实测仍 401 应用未授权**,说明登录链
    还有前置——大概率是它自己的 sso 获取方式变了,需要从该 SPA 的登录流逆向);
  - 新数美 org:`b62YpFMesYwnWYqEKr9M`(与云盘的 `FXlyfmWg2AzwbrxDKSv5` 不同,apiHost 同为
    `slw.h5cmpassport.com:9090`);
  - 活动中心仍挂「云手机红包派对」入口(channelSrc=red-mcloudapp-001),活动本身未下线。

### P3 · 验证与收尾项(非失效,趁修复顺手做)

- 版本串 bump:`appVersion 13.2.2.0 → 13.2.5.0`、`clientVersion 13.2.2 → 13.2.5`、UA `MCloudApp/13.2.5`
  (服务端宽松,非紧急,建议与修复同批);
- MakeWish 进阶组死门(1002"开启自动备份"判号码库白名单,参考账号 exist=false)与 549 同源,
  不必投入;待领豆 615 锁见 §4;
- 新面登记(无任务影响):`msg.mcloud.139.com/msg-api/msg/list`(Spring,入参 `{rsaPhone,userId}`,
  rsaPhone 形态待解)、`ai.yun.139.com/api/openclaw/*`(智能体平台,头要全套 algo 形态,
  `rightStatus=0` 无实例)。

---

## 三、修复顺序与验收基线

```
第一批(P0):R1 域名切换 + R2 主路径统一        → 验收:全链 JWT 黄金用例过
第二批(P1):R3 Accept + 判据审计 + R4 登录互斥  → 验收:AI 任务恢复 + 双账号无互踩
第三批(P2/P3):cloudphone 禁用 + 版本串 bump    → 验收:批次日志无新增 404/406/401 族
每批照惯例:Go 全量测试 + vet + 前端构建 + 双进程(API/Worker)同版本发布 + build 标记核对
```

## 四、既有难题的最新边界(避免重复投入)

- **待领豆 receiveV3 615**:当日又排除 4 个新角度(gp 通道自有清单 / client=out / client=email /
  组合头),与 App 原生数美会话绑定的结论不变。自动化唯一现实路径=真机 UI 驱动
  (姊妹项目 `app_daily.py::do_cloud_center` 已实现"点气泡领取",前提是该账号在真机登录态)。
- **抢兑 GK 610**:维持 `2026-10-12_GK_single_trial.md` 结论——会话/滑块/资格全过仍被拒,
  差异在 App 注入上下文,无新证据前不重复真实提交。

## 五、可复用资产(姊妹项目已就绪)

| 资产 | 位置 |
|---|---|
| gp tyrzLogin 完整可运行实现(Python,含双端点兜底) | `中国移动云盘/06_青龙面板/移动云盘_全任务.py::Account.login` |
| 一键全链验证脚本(出票→JWT→任务表→infoV3→文件层→新面) | `中国移动云盘/04_脚本/verify_1325_live.py` |
| 13.2.5 静态差分全档(端点/域名/密钥/头体系/H5) | `中国移动云盘/05_归档/版本差分_13.2.5_20261101.md` |
| cloud.139.com 云手机 SPA 主包 | `中国移动云盘/work/cloud139/app.6d14d906.js` |
| 现行云朵 bundle 快照(chunk/签名函数/盐) | `中国移动云盘/02_分析/差分_1325/h5_live/` |
