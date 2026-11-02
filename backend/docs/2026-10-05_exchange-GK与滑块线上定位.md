# 2026-10-05 抢兑 `610/GK` 与滑块失败的线上定位

核对时间：2026-10-05 15:00（北京，服务端 `Date` 头一致）。
方法：用真实账号凭据直接打线上接口，逐层把错误码打出来对照。**不含任何伪造结论，未证实的都标未证实。**

## 1. 先把错误码分层（全部为线上实测）

| 服务端回包 | 含义 | 复现条件 |
| --- | --- | --- |
| `{"code":501,"msg":"滑块验证参数不能为空"}` | 没带 `puzzleOffset` | 识别失败时传了 `null` |
| `{"code":514,"msg":"验证失败"}` | **滑块坐标不对** | 同一奖品把 `puzzleOffset` 传 `0` |
| `{"code":407,"msg":"非移动用户不可领奖"}` | **账号号段不满足奖品范围** | A 账号（`133` 电信）+ 移动专属奖品，且滑块坐标正确 |
| `{"code":610,"msg":"活动异常，请稍后重试！Error Code：GK"}` | 上游通用拒绝，**App 自己也只能笼统提示** | 见第 3 节 |

**样本提示部分坐标可能已通过校验。** 据这份探测记录，同一奖品、同一账号更换坐标后，
响应从 `514` 变成 `407`/`610`，与先验滑块再查资格的流程相符。
没有服务端实现或独立的验证成功回包，不能仅凭错误码变化确认所有坐标正确、固定的校验顺序或 GK 的具体原因。

App 侧对照（`03_协议/h5/js/newsignin.75080fd5.js`，即 `exchangePrize` 的调用点）：

```js
if (0 === t)            → 兑换成功
if (514 == t)           → toast "验证失败"，500ms 后重试
var n = { 610: "兑换失败，请稍后再试或联系客服~",
          407: "您非目标用户，无法兑换",
          420: "该奖品每日限兑，您今日份额已用完，明天再来吧～",
          421: "该奖品每月限兑，您本月份额已用完，下月再来吧～",
          614/615: "兑换失败，请稍后再试或看看其他奖品吧" }
```

H5 对 `610` 只显示通用失败提示，没有给出 `GK` 的具体拒绝条件。不能由此排除请求协议、设备身份、活动资格或时间窗口等问题。

## 2. 兑换请求体：我们发的是对的（H5 源码逐字对照）

H5 里 `exchangePrize` 的 body：

```js
n = { prizeId: ..., client: D.value ? "mini" : B.value ? "app" : "out",
      clientVersion: Object(d["c"])(), puzzleOffset: t, smsCode: "" }
```

**body 里没有 `deviceId`** —— 它由请求拦截器按 `isDeviceId: true` 注入
（`139newsignin~…~promotion~test.2ef1f7cf.js`）：

```js
var a = (e.headers.isDeviceId === true);
var c = sessionStorage.getItem("deviceId");      // 来自 window.SMSdk.getDeviceId()（数美 SDK）
if (c && a) { if (e.method === "post") { u = JSON.parse(e.data); u.deviceId = c; e.data = JSON.stringify(u); } ... }
```

并与抓包证据一致（`docs/2026-09-30_protocol-云盘领取与抢兑-report.md` E-01：
`POST exchangeV3`，JSON 含 `prizeId/client/clientVersion/puzzleOffset/smsCode/deviceId`，`isDeviceId=true`）。
**现有实现已经对齐**，不需要再改请求形状。

线上验证过三个"是不是漏了字段"的猜想，全部仍然 `610/GK`：

| 变体 | 结果 |
| --- | --- |
| `activityid: sign_in_3`（现状） | 610/GK |
| `activityid: sign_in_3_ex`（奖品自身的 marketName） | 610/GK |
| body 里不带 `deviceId` | 610/GK |

## 3. `610/GK` 到底是什么：两条硬线索

### 3.1 有一道"资格校验"我们此前完全没做

App 在发兑换前会先调：

```
GET /ycloud/signin/page/receivePrizeDetails?marketId=<marketName>&prizeId=<id>
  -> result.exchangeCode
```

`exchangeCode != 0` 时 **H5 直接把按钮拦下，根本不发兑换请求**。线上实测（`marketId=sign_in_3_ex`）：

| 账号 | QQ音乐绿钻会员月卡 | 哔哩哔哩会员月卡 | 菜鸟裹裹5元寄件券 |
| --- | --- | --- | --- |
| A `133****1213`（**电信**） | **2305** | **2305** | 0 |
| B `157****9115`（**移动**） | 0 | 0 | 0 |

奖品 `memo` 明写「该奖品**中国移动用户**可兑换」/「仅限**本月成功备份**的用户兑换」。
`2305` = 非目标用户；`2303` = 需开启自动备份且本月成功备份照片；`2302` = 当月第三方兑换额度用尽。

⇒ **A 账号对移动专属奖品是注定失败的**（这也是 A 那次 `407` 的来源）。
此前我们把这类请求真的发了出去，既浪费次数，也在上游风控那里累积失败样本。

### 3.2 放货窗口：服务端时间 15:00，而这批奖品的放货时间是 **16:00**

兑换列表里每个商品都带 `hour` / `minute`（本批几乎全是 `16:00`）。
H5 的逻辑是：当前小时还没到放货点、且按钮文案是 `"16:00开抢"` 时，**不会**进入「立即兑换」状态。

> **未证实**：本轮在 16:00 前做的所有尝试都回 `610/GK`，与"未到窗口"高度吻合，
> 但**没有做 16:00 之后的对照实验**，所以不能断言 `610/GK == 未到放货窗口`。
> 验证方法：在 16:00 后（且商品 `dailyRemainderCount > 0` 时）用同样方式重放一次，
> 看是 `code=0` 还是仍然 `610/GK`。

`dailyRemainderCount=0` 只描述查询时的当日剩余库存；后续是否补货需重新查询。
明确的「奖品单日已耗尽」应按库存不足处理，不能用一个历史库存快照判定全天都不可兑换。

## 4. 滑块（NCC）本身的问题：不是语义错，是**稳定性**

`puzzleOffset` 就应该是 NCC 在**原图坐标系**里的匹配 x（`picWidth=680`），
参考实现 `sms_login_app/captcha/SliderCaptcha.js` 返回的也是 `bestX`。线上核对：
识别成功且置信度高时，服务端**不再回 514**，说明语义是对的。

实测同一接口不同次抽样的置信度：`0.9492 / 0.9160 / 0.9070 / 0.9030 / 0.864 / 0.8488 / 0.8278 / 0.8162 / 0.8144 / 0.7474 / 0.6973 / 0.5613`。

2026-10-05 源码复核：上述 `0.70` 拒绝阈值属于临时 Python `test_slide.py` 探测脚本。核对的 Node `SliderCaptcha.js` 与 `/solve` 路由没有这一阈值，会把有效坐标及其置信度返回给调用方。

当前 Go 后端以 `CAIYUN_EXCHANGE_SLIDE_MIN_CONFIDENCE=0.75` 为默认阈值，低于阈值重新取题，最多三题；不会把识别失败的 `null` 坐标提交到兑换接口。`501` 的探测结果不能视为当前 Go 后端必然存在的行为。

后续优化已将多特征 NCC 识别直接内置到 Go 后端，默认不再调用远端识别服务；增加 JPEG 平滑、暗色纹理保留和候选歧义拒绝。参考图片与模拟服务的结果见 [内置滑块识别](2026-10-05_builtin_slider.md)，这些本地结果不改变本报告对 GK 根因尚未证实的判断。

这 12 个样本中有 3 个低于 0.75，样本量不足以估计长期失败率。相关系数映射出的置信度也不等于服务端验证通过概率。重新取题及不提交低置信度坐标的策略已实现，并有模拟服务回归覆盖。

## 5. 本次已落地的代码改动

`backend/internal/services/exchange_eligibility.go`（新增）+ `exchange_executor.go`（接入）：

1. 兑换前先调 `receivePrizeDetails` 取 `exchangeCode`；
   * 非 0 → **直接终止本次兑换**（`stop=true`），日志写明 `exchangeCode` 与对应文案，不再白发请求；
   * 0 或读不到 → 不拦截（保持原行为）。
2. 失败日志附加 `资格 exchangeCode=…`，成功日志同样带出，便于事后归因。
3. `isExchangeTerminalMessage` 增加 `兑换资格校验未通过` / `非目标用户` 两个终止词。
4. 同步更新两处测试 mock（`exchange_executor_http_test.go`、`exchange_validation_test.go`）以适配新端点。

验证：`go build ./internal/services/` 通过、`go vet` 干净、`gofmt` 无差异。

> **初次集成时的测试记录（已修复）**：`go test ./internal/services -run Exchange` 曾有两项断言不匹配：
> * `TestExecuteExchangeOnceSuccessWithMockServer` —— 断言成功文案的标签是 `captcha`，与分类器矛盾；
> * `TestExecuteExchangeOncePropagatesSlideErrors` —— 断言 `滑块验证码识别失败…` 的标签是 `other`，同样矛盾。
>
> 最终合并已修正成功消息为 `other`、滑块识别失败为 `captcha`，补齐预检模拟响应。
> 后端全量 `go test ./...`、`go vet ./...` 与 `go build ./...` 已通过；新增资格不满足时不取滑块、不提交兑换的回归用例。

## 6. 给使用方的操作结论

1. 按商品和所配置场次触发兑换；当前记录的 `16:00` 是该次查询的时间信息，不能推广成所有商品和所有场次的唯一有效时间，也不能认定提前请求必然返回 GK。
2. **先过滤**：用 `receivePrizeDetails.exchangeCode==0` 筛掉注定失败的奖品（A 账号尤其明显：
   移动专属奖品一律 `2305`）；再用兑换列表的 `dailyRemainderCount>0` 筛库存。
3. **滑块失败就换一张**，不要在同一张图上反复猜坐标。
4. `610/GK` 的根因仍未确认；需要在有效场次结合资格、库存、设备身份、挑战会话和实际回包核对。已完成的本地修复不能作为线上兑换成功的证明。
