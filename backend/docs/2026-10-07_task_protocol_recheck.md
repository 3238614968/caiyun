# 任务失败复核与请求头修复

本轮从 2026-10-06 的失败日志开始，对照用户提供的 `D:/Project/Python/逆向/app/中国移动云盘`，于 2026-10-07 整理。保留此前未提交的修复。下表明确区分线上接口结果、本地回归和仍待确认的问题。

## 已复现的海报根因

公共 Go HTTP 客户端用原始 map 同时发送 `jwtToken` 和 `jwttoken`，以及其他仅大小写不同的字段。HTTP 字段名不区分大小写，传输后这些会成为同一字段的多个值；自定义设备头也可能没有覆盖默认头。

用参考项目同一账号新取的会话，对同一上传预约端点做最小对照，凭据只保留在内存中：

| 请求 | 结果 |
| --- | --- |
| 参考 Python 的 JWT 请求头 | `code=0`，有上传地址 |
| Python 请求额外携带 Basic 头 | `code=0`，有上传地址 |
| 修改前的 Go 客户端 | `code=505`，权限不足 |
| 仅将 Go 请求头按大小写不敏感合并 | `code=0`，有上传地址 |
| 修复后的原始 Go 客户端，再次验证 | `code=0`，有上传地址 |

因此这次有证据支持的根因是重复请求头，不能继续归因于海报活动名或 Basic 头。修复放在公共客户端，默认头先合并、自定义头再覆盖，最后以标准 Header.Set 发送。新增真实 HTTP/1.1 与 HTTP/2 测试确认单值鉴权头及覆盖行为；旧模拟传输只检查第一个匹配值，未发现实际发出了重复字段。

此次线上对照仅预约上传地址，没有宣称已经拿到海报活动奖励；上传、完成回调和活动完成位仍有本地协议回归。

## 固定人像与写真

用户明确授权将参考项目 `FACE_B64` 人像上传并固定用于后续相关任务。该 JPEG 原样内置到 `internal/core/api/assets/ai-task-reference-face.jpg`，供 AI 写真及趣玩 AI 图像算法共用，不再为写真逐张下载、上传广告素材。

使用参考协议对该固定图完成一次上传与主图校验，返回 `code=0000`，同时得到 `masterId`、`taskId`。该次测试图片已回收。此证据只证明主图校验，不等于写真生成或所有账号的活动完成位已验证。

写真流程补齐 `recordUnfinishFileId` 的错误检查、`getUnfinishFileId` 和 `queryModelList`。生成使用服务端返回的 `modelType`、`poseId`，停止写死模型 1001。目录登记、主图校验、建形象和模型查询的业务错误保留端点与 code；下游异常不再笼统写成“没有通过正脸校验的模板图”，也不连续上传十二张其他人像。

## 分享与待领云朵

- 邀请好友任务原来查询失败就默认还可分享 20 次，连续发出事件后也没有复查流水，并夹带两次全账户云朵领取。现在查询错误立即返回；每次分享后复查 `fxnrplus5` 奖励记录，未更新则停止本轮并标为待确认，不伪报完成。
- 分享表单改用 URL 编码，保留 base64 的 `+`、`=`；事件使用当前 SSO，不再生成假的 SSO 字符串。
- 待领云朵由现有领取任务统一处理，邀请日志不再承载无关奖励的领取失败。领取异常保留具体奖励记录、业务 code 和消息；最终待领数量重新查询，保留已确认的部分到账。
- 设备注册失败时，领取任务不再拿构造器里的静态设备 ID 继续提交。

这几项修复不代表日志中的“20 云朵”已经实际到账。本轮只读查询了参考账号的待领类型，没有领取该账号的云朵；新日志用于区分业务拒绝、设备认证失败和奖励状态未更新。

## 兑换 610/GK

后续用户提供成功抢兑 HAR 后，又核对出活动日志引起的 Cookie 更新及官方 SDK `encode=5/compress=2` 与当前简化注册的差异。最新证据和追加修复见 [成功抢兑 HAR 再核对](2026-10-07_exchange_har_recheck.md)。

兑换使用独立的 HTTP 会话，原本通过 Header.Set 发送，因此不能把海报的重复头根因直接套用到 GK。

已修复的另一组明确不一致：注册数据使用随机 User-Agent A，注册 HTTP 请求重新随机为 B，后续兑换却固定为 C；注册数据的 `dg` 还重复拼接 `5.0`。现在注册数据、注册请求和当前活动会话使用同一个 User-Agent，并补齐活动请求的 Origin。

**没有提交真实奖品兑换，尚未确认 610/GK 已消除。** `exchangeCode=0` 只证明资格预检通过，滑块分数只说明本地匹配质量。服务端最终拒绝不能被改成成功，后续仍需结合有效场次、库存、设备会话和实际业务结果核对。

## 参考主流程覆盖

核对 `06_青龙面板/移动云盘_全任务.py` 的 28 个 steps，加单独调用的 mutual，共 29 个执行步骤。以下是代码覆盖关系，不是所有账号的完成承诺。

| 参考步骤 | 当前实现 |
| --- | --- |
| signin | signin |
| upload、pcupload100 | tasklist 文件/照片/PC 上传与月度上传 |
| ai、aitools | fun_ai 图像任务 003/004/006/007/008/009/031/034/3Drenou |
| camera、chat | tasklist、fun_ai、活动公共动作 |
| note | tasklist 云笔记、fun_ai 笔记登记 |
| email | mail_mutual 邮箱登录、真实互发、MQ 上报与转存 |
| aiworkbench | tasklist 1057/1028，渠道 10175 |
| knowledge | tasklist 547 知识库创建；不把它等同于 funai 的知识库完成位 |
| timed | fun_ai 限时模块录音转写 |
| poster、playai | poster_activity 上传、相机、步骤推进及复查 |
| tokenpk | token_pk 动作、登记、领奖、预约 |
| makewish | make_wish 动作及状态复查 |
| backup | 活动公共相册备份；本轮备份翻倍补做也改用照片和快照流程 |
| rewards | task_expansion_reward、cloud_multiple、messagepush、backupgift、notice_switch 等 |
| miaoyun、scan | fun_ai 写真 011、扫描文档 204 |
| lottery | fun_ai 模块抽奖 |
| hidden | hidden_rewards 隐藏任务、月度福袋和活动奖励 |
| cloudphone | cloudphone 红包派对 |
| lantern | lantern 猜谜 |
| shake | shake 抽抽乐 |
| liveflower | liveflower 首次赠花 |
| share | tasklist 434 真实文件分享及每日进度 |
| exchange | makewish_exchange，300 AI 豆兑换入口，保留显式启用 |
| mutual | mutual_assist 多账号互助 |

参考目录也记录了时间、账号资格和需要 App 交互的限制。连续通知/跨月预约按服务端天数推进；学生认证、公众号绑定、短信领奖等保留实际资格要求；朋友圈九图、趣玩 AI 知识库等不使用“请求发出”替代服务端完成位。

## 验证与交付

回归包括真实 HTTP 请求头传输、写真固定图输入与动态模型、下游错误保留、分享失败停止与奖励确认、备份照片及快照、领取部分到账、设备注册失败停止领取，以及设备注册的 User-Agent 一致性。

最终 `go test ./...`、`go vet ./...` 通过。已使用 `scripts/build-linux.ps1 -Version 2026.10.07-taskfix-dirty` 生成 `backend/caiyun-linux` 和 `backend/SHA256SUMS`，确认 Linux amd64 ELF 格式与校验和一致。制品包含当前工作区的修复，尚未部署到线上。

常规测试不运行线上兑换；当前环境没有 C 编译器，race 与需要真实 Redis/MySQL 的集成验证未运行。新版后端需要重新部署并重启 API、Worker；本轮没有前端源代码或数据库结构改动。
