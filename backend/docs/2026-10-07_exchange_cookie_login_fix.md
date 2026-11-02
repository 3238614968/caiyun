# App 手动成功与自动兑换差异复核

用户确认“手动正常”指中国移动云盘 App 内手动兑换。重新读取原始 HAR，而非此前的单值请求头摘要，确认并修复以下实现问题。

## 1. 前一轮漏读 HTTP/2 Cookie

HAR entries[1798] 的 `request.headers` 内有九条独立的 `cookie` 字段。此前分析脚本用了 `{header.name: header.value}`，仅保留最后的 JSESSIONID，进而错误删除了会话 Cookie。这是分析和实现错误；原“成功请求只有 JSESSIONID”的结论作废。

实际 Cookie 名称：

- `sajssdk_2015_cross_new_user`
- `cookieToken`
- `cookieTokenKey`
- `hecaiyundata2021jssdkcross`
- `smidV2`
- `.thumbcache_45700955f71be4ef518b0a1af26a3f40`
- `jwtToken`
- `userDomainId`
- `JSESSIONID`

已恢复 JWT、用户域和当前 Basic 凭据对应的 cookieTokenKey，值从当前账号取得，保留账号间隔离。SDK 注册成功后写入同一次请求使用的 smidV2，以及正确名称的设备 Cookie；设备 Cookie 名后缀是组织 ID 的 MD5，而不是手机号或 caiyun。

已直接核对：设备 Cookie 的值经 URL 解码后，等于 entries[1302] 返回的原始 deviceId；兑换 JSON 的 deviceId 恰为这个值加 B 前缀。因此设备 Cookie 必须正确转义 `+`、`/`、`=`，且与本次设备注册结果一致。

未复制 HAR 的旧凭据、Cookie 或设备标识。`cookieToken` 的 App 生成路径和两项统计 Cookie 本轮没有重建，不把当前会话宣称为真实 App 的完整副本。

## 2. App H5 登录不能只换接口地址

HAR entries[1352]：

```text
POST /ycloud/auth-service/auth/tyrzLogin
JSON: token=<当前SSO>, openAccount=0, marketName=sign_in_3, sourceId=1097
```

该接口另需 `x-request-id`、`x-timestamp`、`x-nonce`、`x-signature`。官方 H5 先读取 `/ycloud/api/timestamp`，时间查询失败时使用本地毫秒；两个 ID 使用新 UUID。签名为公开 H5 协议盐、request ID、timestamp、nonce、同一盐的拼接 MD5。公式与 HAR 中的签名匹配，独立固定测试向量也一致。

线上验证过程明确暴露了仅切路由的遗漏：

| 请求 | 结果 |
| --- | --- |
| 新 H5 路由、没有签名 | HTTP 200，code=401，请求签名校验失败；随后回退旧接口 |
| 补齐官方签名和服务器时间 | 新 H5 路由直接 code=0 / success；没有旧接口回退 |

当前优先使用带签名的新接口，保留旧接口兼容回退。新接口的 JWT 与用户域两条 Set-Cookie 均由 HTTP 客户端保留。

## 3. 实测与测试范围

用参考项目当前账号做了登录与设备注册检查，凭据只经标准输入传递，没有写入诊断文件，也未输出 Cookie 值。

- 最终成功登录路径只有 `/ycloud/auth-service/auth/tyrzLogin`。
- 当前 SDK 设备注册成功。
- 会话中包含 `.thumbcache_<组织MD5>`、smidV2、cookieTokenKey、JWT 和用户域；后续活动接口返回的 JSESSIONID 仍由 CookieJar 接收。
- 修正了先前错误地禁止非 JSESSIONID Cookie 的兑换测试，改为验证当前账号的凭据 Cookie、SDK Cookie 和服务端 Cookie 共同保留。
- 新增 `scripts/inspect_exchange_har.py`，按列表保留同名头；对应 Python 回归覆盖九条 HTTP/2 Cookie、多条 Set-Cookie、HTTP/1.1 合并 Cookie，以及输出不包含凭据。
- Go 回归覆盖签名向量、App 登录路径、服务端时间、两条登录 Cookie、设备组织名及转义、账号隔离和不同商品复用兑换流程。

本轮没有扣除云朵或提交真实奖品兑换，因此没有用“登录成功”代替“兑换成功”。完整浏览器指纹及尚未重建的 App Cookie 仍是与 App 的差异，但已确认的漏 Cookie、错误 SDK Cookie 名和缺少登录签名均已修复。

## 部署

替换新版后端并重启 API、Worker，避免继续使用旧进程的会话。前端、数据库结构无需变更。先前 `sdk5-dirty` / `harfix-dirty` 二进制不包含本次更正。

后端 `go test ./...`、`go vet ./...` 和两项 Python HAR 解析回归通过。已编译版本 `2026.10.07-cookieauth-dirty` 的 Linux amd64 制品 `backend/caiyun-linux`，并校验 `backend/SHA256SUMS` 一致。未在当前 Windows 环境运行需要 C 编译器的 race 检查及真实 Redis/MySQL 集成测试。
