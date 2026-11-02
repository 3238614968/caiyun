# 成功抢兑 HAR 再核对

来源：用户指定的 `D:/Hunzi/Documents/移动云盘领取奖励和真实抢兑.har`。共 1875 条记录，SHA256 为 `583de610e5f89a9569fef6485ca68002cc374c9ecb7a17a473c0e0aab275eec4`，与此前 2026-09-30 报告使用的文件一致。本次重新核对完整会话和设备注册，未执行其中的脚本或重放凭据，未提交真实兑换。

## 可以复用的兑换协议

成功请求为 entries[1798]，`POST /ycloud/signin/page/exchangeV3`。JSON 共六个字段：`prizeId`、`client=app`、`clientVersion=13.2.2`、`puzzleOffset`、空 `smsCode`、`deviceId`。业务回包 `code=0`，包含 `result.oid` 和对应 `prizeId`。

因此不同商品共用此协议，`prizeId` 从当前任务取值，无需为 QQ 音乐等商品单独实现另一套提交接口。不过每次挑战坐标、账号 JWT、设备标识和服务端会话均须取当前值；商品资格也需要当前接口确认。HAR 成功商品是“移动云盘100万tokens叠加包”，另一商品“酷狗音乐会员周卡”在 entries[1745] 的资格码为 2305。

## 会话与请求头差异及修复

| 顺序 | HAR 索引 | 行为 |
| --- | --- | --- |
| 1 | 1792 | `receivePrizeDetails`，资格码 0 |
| 2 | 1793 | POST `visitlog/journaling`，`optkeyword=newsignin_index_exchange_times`，`module=uservisit`、`sourceid=1097`、`marketName=sign_in_3`；响应通过 Set-Cookie 更新 JSESSIONID |
| 3 | 1794 | POST `auth-service/slide/getSlide`，取得 680×400 背景及 96×400 滑块 |
| 4 | 1798 | POST `exchangeV3`，使用日志接口更新后的 JSESSIONID；返回兑换成功 |

App 的日志与取题请求在时间上有重叠，取题仍携带日志响应前的 Cookie；兑换携带更新后的 Cookie。不能由此推断滑块必须与旧 JSESSIONID 字符串绑定。实现采用顺序执行日志、取题、提交，沿用同一 CookieJar，避免自身的 Cookie 轮换竞争。

已补齐兑换前的活动日志请求，最多等待 2 秒；日志请求失败会记录诊断并继续原有兑换流程，不因可选日志服务异常重复兑换。获取挑战前清除上轮的可选 trace 值。

**更正：此前“Cookie 只有 JSESSIONID”的结论错误，相关删 Cookie 的改动已撤销。** 原分析将 HTTP/2 的同名 Cookie 头转换为单值字典，覆盖了前八条。实际成功请求有九条 Cookie，其中包括 `jwtToken`、`userDomainId`、`cookieTokenKey`、`smidV2` 和 `.thumbcache_<组织MD5>`。现在恢复当前账号和当前 SDK 会话对应的 Cookie，并保留 CookieJar 接收的 JSESSIONID；详细证据见 [Cookie 与签名登录修复](2026-10-07_exchange_cookie_login_fix.md)。额外 `deviceId` 请求头仍不发送，设备 ID 保留在兑换 JSON。

## 成功验证码回归

仅将 entries[1794] 的两张验证码图片加入测试，不复制原始账号、Cookie、JWT、设备 ID 或订单。

- HAR 实际成功提交偏移：546。
- 内置识别输出：546，置信度 0.8889，峰值差 0.5598。
- 回归同时检查 260630012 和虚构的其他商品 ID，确认资格查询、日志、内置匹配和提交顺序，以及日志更新后的 Cookie 被提交采用。
- 日志 HTTP 503 时的兼容分支也有覆盖。

此样本说明当前内置算法与这次真实成功坐标相符，不能据一张图推导所有挑战准确率。

## 仍未对齐的设备 SDK

**后续实现更新：** 下表记录核对时的旧实现。正常 `5/2` 编码现已原生 Go 接入，并完成一次线上设备注册验证，详见 [设备 SDK 编码链移植](2026-10-07_device_sdk_codec.md)。完整浏览器数据采集与实际兑换结果仍需分别看待。

| 项目 | HAR entries[1302] | 当前 Go 简化实现 |
| --- | --- | --- |
| deviceprofile 端点 | `/deviceprofile/v4` | 相同 |
| organization / appId / os | 同一组织 / default / web | 相同 |
| encode / compress | **5 / 2** | **1 / 0** |
| data | 官方 SDK 编码后的数据，无法直接 base64 解码成 JSON | base64(JSON) |

成功兑换的 `deviceId` 恰为 entries[1302] 返回 ID 加 `B` 前缀，不能复制此旧值给其他会话。HAR 也含官方 `fp.min.js`，但一份已编码请求不能证明简化设备注册具有相同效果，更不能只将 1/0 改写为 5/2 就当作 SDK 对齐。

当前 **610/GK 仍未通过真实兑换验证消除**。前一轮修复的 User-Agent 一致性与本轮请求顺序、Cookie 修复均有依据，但不能将任何一项直接认定为 GK 的唯一原因。官方设备 SDK 与简化注册之间的差异应保留为后续定位重点。

## 验证与部署

模拟接口使用虚构凭据，验证不同商品复用流程、服务端 Cookie 更新、设备 ID 仅放 JSON、固定成功图片识别，以及日志失败不阻断兑换。部署需更新后端并重启 API、Worker；本轮不涉及前端或数据库变更。

`go test ./...`、`go vet ./...` 均通过。已生成版本标记 `2026.10.07-harfix-dirty` 的 Linux amd64 二进制 `backend/caiyun-linux`，并核对 `backend/SHA256SUMS`。当前制品 SHA256：`605e3823ac90a5303ccc159dc4eefa20074db4f7ecc6bc6c5d82577a778aba67`。尚未部署或提交真实兑换。
