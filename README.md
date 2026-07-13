# 移动云盘自动任务与兑换管理系统

![Go](https://img.shields.io/badge/Go-1.25.11-00ADD8?logo=go&logoColor=white)
![Vue](https://img.shields.io/badge/Vue-3-42b883?logo=vue.js&logoColor=white)
![TypeScript](https://img.shields.io/badge/TypeScript-5.x-3178C6?logo=typescript&logoColor=white)
![MySQL](https://img.shields.io/badge/MySQL-8.0+-4479A1?logo=mysql&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green.svg)

基于 **Go 后端 + Vue 3 前端** 的移动云盘自动化管理平台，提供账号托管、日常任务执行、兑换调度、商品同步、日志审计、监控与管理后台能力。

---

## 目录

- [项目简介](#项目简介)
- [核心能力](#核心能力)
- [版本更新摘要](#版本更新摘要)
- [系统架构](#系统架构)
- [技术栈](#技术栈)
- [目录结构](#目录结构)
- [快速开始](#快速开始)
- [构建与部署](#构建与部署)
- [安全与质量检查](#安全与质量检查)
- [运行机制](#运行机制)
- [仓库说明](#仓库说明)
- [Roadmap](#roadmap)
- [贡献](#贡献)
- [许可证](#许可证)

---

## 项目简介

本项目面向移动云盘自动化管理场景，包含以下两部分：

- **管理后台**：统一管理账号、任务、兑换账户、商品库、系统日志与公告。
- **任务/兑换执行层**：负责定时任务、奖励领取、商品同步、并发抢兑、状态记录与异常重试。

支持的典型场景：

- 多账号集中托管
- 日常任务自动执行
- 奖励自动领取
- 商品自动同步与定时兑换
- 管理员后台统一监控与日志排查

---

## 核心能力

### 1) 用户认证

- 登录、注册、HttpOnly Cookie 会话
- 用户可通过“用户名 + 注册邮箱 + 邮箱验证码”自助重置密码
- 管理员可在后台为用户重置密码，用于未绑定邮箱账号的兜底恢复
- 密码变更会递增会话版本，旧 JWT / Cookie 会话会立即失效

### 2) 账号管理

- 多账号管理，支持同一手机号被不同用户分别绑定
- Authorization / JWT 自动刷新，支持新版 APP refreshToken 链路
- JWT 获取失败自动重试，连续失败可自动禁用账号
- 账号健康检查、状态监控、过期账号隔离

### 3) 自动任务

- 每日签到
- 备份翻倍奖励
- 微信签到 / 微信抽奖
- 摇一摇
- 任务中心巡检
- 备份礼包 / 膨胀奖励
- 邀请好友
- 消息推送奖励
- 领取云朵 / 今日云朵统计
- 云朵大作战 / 云手机红包
- 复活卡奖励任务
- 收尾清理任务
- 果园 / 盲盒 / AI 红包 / AI 云朵等历史任务保留在注册表中，但默认不参与批量执行

### 4) 兑换中心

- 商品自动更新 / 手动同步
- 自定义兑换时间
- 多账号并发兑换
- 滑块验证码接口识别
- 同账号同月同分类商品重复兑换保护
- 兑换日志与执行结果记录

### 5) 管理后台

- 用户管理、账号管理、任务配置、公告管理
- 商品中心、兑换账号、抢兑任务、领奖专区
- 系统日志与任务日志查看
- 响应式布局与移动端适配

---

## 版本更新摘要

> 以下内容已纳入当前代码与构建产物。

### 新增功能

- **新版 Authorization 刷新机制**
  - 接入 APP 新版 `user/auth/refreshToken` 链路
  - 使用 AES-192-CBC 加密请求与解密响应
  - 刷新成功后重新组装 `Basic base64(mobile:phone:token)`，并通过 `querySpecToken` 验证可用后才写入数据库
  - 主账号刷新成功后会同步更新对应抢兑账号的 `auth / token / jwt_token`
- **抢兑滑块验证码接口识别**
  - 抢兑流程自动获取滑块验证码图片
  - 仅保留远端识别接口调用，不内置本地 NCC / PCL 图片识别实现
  - 识别成功后自动带入 `puzzleOffset` 调用新版 `exchangeV2`
- **月度同系列兑换保护**
  - 识别“兑换成功 / 重复兑奖 / 本月已兑换”等结果
  - 同账号同月同分类商品不再重复执行，避免音乐、流量等系列重复提交
- **签到中心 / 任务中心 V2 适配**
  - 已接入新版 `taskListV2`
  - 支持按 `cloudEmail / time / day / month` 分组拉取任务
- **新增任务能力**
  - 分享文件任务
  - 上传文件任务
  - 月上传补传任务
  - AI 相机任务
  - 复活卡奖励任务
- **后台与展示增强**
  - 任务类型与执行结果统一为更短、更规整的中文展示
  - 前端管理页、兑换页、日志页完成自适应优化
  - 管理页与日志页在手机端支持卡片视图
  - 引入前端 E2E 用例覆盖登录、账号、兑换、管理员配置等核心路径

### 修复与优化

- **修复签到上下文构造不一致**
  - 补齐签到中心预热请求
  - 补齐 `clientVersion / User-Agent / sourceid / deviceId / userDomainId` 等上下文
- **修复签到状态判断不一致**
  - 新增对 `cal[].s` 的兜底判断，避免仅依赖旧字段导致状态误判
- **修复 JWT / SSO 上下文错配**
  - 统一复用同一次获取的 `ssoToken` 与其对应 JWT
  - 避免签到预热页与 JWT 来源不一致导致 `infoV3 / startSignIn / taskListV2` 失败
- **修复 JWT 非规范编码绕过校验**
  - 校验 JWT 三段 Base64URL 是否为规范编码
  - 避免篡改但解码等价的 token 被误判为有效
- **修复临时文件清理误删风险**
  - 临时上传/分享文件仅清理本次任务记录的文件 ID，不再按文件名前缀全盘扫描删除
- **修复兑换与账号相关流程细节**
  - 同手机号跨用户添加场景下的账号查重与创建逻辑更稳定
  - 失效账号在商品更新选择列表中不再参与展示
- **增强密码找回与会话安全**
  - 密码找回改为邮箱验证码流程，验证码存入 Redis 并设置 TTL
  - 发送验证码、重置密码的错误响应做统一处理，降低账号邮箱枚举风险
  - Cookie 会话场景补充 CSRF 校验，登录态异常时前端会同步清理本地状态
  - 密码重置、管理员重置密码会递增会话版本，已签发 JWT 立即失效
  - 用户维度限流移动到认证后执行，避免退化为 IP 限流
  - 登录失败锁定优先使用 Redis，多副本共享；Redis 异常时可降级到 `login_fail_locks` 数据库表
  - 外部 HTTP 响应体读取增加大小限制，并避免短信认证响应明文落日志
- **优化首页公告与统计展示**
  - 首页展示公告列表，不再新增用户侧公告菜单
  - 仅置顶弹窗公告会弹出，其余公告仅展示在列表中
  - 修复公告已读状态本地缓存异常导致首页加载中断的问题
- **修复抢兑请求上下文**
  - 抢兑接口切换到新版 `exchangeV2`
  - 补齐 `jwttoken / activityid / deviceid / appversion / referer` 等移动端上下文
  - 单账号只提交一次抢兑；成功后继续下一个账号，商品无库存/已兑完/已下架时停止当前商品后续账号
- **统一初始化材料与运行时定义**
  - 已同步 `README`、`backend/migrations/init.sql`、`backend/scripts/init_caiyun_database.sql`
  - 默认任务清单、启用状态、排序顺序与代码注册表保持一致
- **增强部署与监控配置**
  - Docker Compose 使用 Redis 密码、独立数据库应用用户，并补充 Grafana 示例服务
  - 新增 K8s 示例清单，内置 MySQL 初始化 SQL ConfigMap 与 MySQL PVC
  - API / Worker / 前端容器以非 root 用户运行
  - Nginx 补充安全响应头与 HTTP 到 HTTPS 跳转
  - K8s 应用镜像固定版本，并补充 securityContext、资源限制与健康探针
- **统一默认调度配置**
  - `backend/configs/.env.example` 中的 `TASK_SCHEDULE` 已调整为 `0 8 * * *`
  - 与 Worker 内部默认回退值保持一致，避免环境配置漂移
- **清理历史遗留调度逻辑**
  - 已移除 Worker 中未使用的 `RunScheduledTask()` ticker 调度实现
  - 清理 `Stop()` 中重复 `cancel()` 的历史遗留代码

---

## 系统架构

```text
┌─────────────────────────────────────────────────────────────┐
│                        Frontend (Vue 3)                    │
│               Element Plus + TypeScript + ECharts          │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                      Nginx / Reverse Proxy                 │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│             统一后端制品 backend/caiyun-linux             │
│  api：用户认证 / REST / WebSocket / 管理接口               │
│  worker：定时任务 / 队列消费 / 自动任务 / 抢兑执行          │
│  migrate / reencrypt：发布迁移与字段密钥轮换                │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                        MySQL / Redis                       │
│           账号、任务、商品、兑换记录、配置、缓存等           │
└─────────────────────────────────────────────────────────────┘
```

---

## 技术栈

| 层级 | 技术 | 说明 |
|------|------|------|
| 前端 | Vue 3 + TypeScript + Element Plus | 管理后台与业务界面 |
| 图表 | ECharts | 首页趋势图与统计展示 |
| 后端统一制品 | Go + Gin | 同一二进制按 `api` / `worker` 等子命令运行 |
| 数据库 | MySQL 8.0+ | 持久化存储 |
| 缓存 | Redis | 队列、缓存、状态管理 |

---

## 目录结构

```text
.
├── backend/                       # Go 后端
│   ├── cmd/
│   │   └── caiyun/                # 统一后端入口与子命令分发
│   ├── configs/                   # 配置模板
│   ├── docs/                      # 项目文档
│   ├── internal/
│   │   ├── app/                   # api/worker/migrator/reencrypt 运行模块
│   │   ├── core/                  # HTTP / Auth / API / Task 核心能力
│   │   ├── handlers/              # HTTP 处理器
│   │   ├── middleware/            # 中间件
│   │   ├── models/                # 数据模型
│   │   ├── repository/            # 数据访问层
│   │   └── services/              # 业务服务层
│   ├── migrations/                # 数据库初始化脚本
│   └── scripts/                   # 数据库初始化脚本（Compose 入口）
├── frontend/                      # Vue 前端
│   ├── src/
│   └── dist/                      # 前端生产构建产物
├── deploy/                        # systemd / logrotate / 部署说明模板
├── scripts/                       # 本地 CI、部署、回滚、健康检查脚本
├── Makefile                       # 本地构建/测试/审计等价脚本
├── nginx-server.conf              # Nginx 配置示例
├── docker-compose.yml             # Docker Compose 示例
└── README.md
```

> 后端只发布一个构建产物 `backend/caiyun-linux`。API、Worker、迁移器和重加密工具是该二进制的不同子命令，不再分别维护多个后端制品。

---

## 快速开始

### 环境要求

- Go 1.25.11（以 `backend/go.mod` 为准；本地开发建议启用 `GOTOOLCHAIN=auto`）
- Node.js 18+
- MySQL 8.0+
- Redis 6.0+

### 1. 初始化数据库

```bash
mysql -u root -p < backend/migrations/init.sql
```

### 2. 配置后端环境变量

```bash
cd backend
cp configs/.env.example .env
```

根据实际环境填写数据库、Redis、JWT、端口等配置。

多副本部署建议显式启用 Redis 限流后端，避免不同 API 副本各自维护本地窗口：

```bash
RATE_LIMIT_BACKEND=redis
RATE_LIMIT_REDIS_WINDOW=1s
```

如需启用用户自助找回密码，还需要配置 SMTP：

```bash
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USERNAME=your_smtp_username
SMTP_PASSWORD=your_smtp_password_or_app_password
SMTP_FROM=no-reply@example.com
SMTP_FROM_NAME=移动云盘
# 465 端口通常设为 true；587 端口通常设为 false 并使用 STARTTLS
SMTP_USE_TLS=false
```

密码找回验证码会写入 Redis，并按 TTL 自动过期；因此生产环境请务必配置 `REDIS_PASSWORD`。

### 3. 启动后端 API

```bash
cd backend
go run ./cmd/caiyun api
```

### 4. 启动 Worker

```bash
cd backend
go run ./cmd/caiyun worker
```

### 5. 启动前端

```bash
cd frontend
npm install
npm run dev
```

## 构建与部署

### 前端构建

```bash
cd frontend
npm install
npm run build
```

输出目录为 `frontend/dist/`。

### 唯一后端制品

```bash
make backend-build
# 等价命令：
cd backend
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o caiyun-linux ./cmd/caiyun
```

唯一后端构建输出为 `backend/caiyun-linux`，通过子命令按需运行：

```text
caiyun-linux api
caiyun-linux worker
caiyun-linux migrate [--validate-only]
caiyun-linux reencrypt [--apply]
caiyun-linux all
caiyun-linux version
```

生产环境建议使用同一个制品分别运行 API 和 Worker；`all` 是单机便捷监督模式，不替代容器或 systemd 的进程隔离。

### Docker Compose

根目录 `.env` 至少配置：

```env
APP_ENV=production
MYSQL_ROOT_PASSWORD=<强随机 root 密码>
MYSQL_USER=caiyun_app
MYSQL_PASSWORD=<强随机应用数据库密码>
REDIS_PASSWORD=<强随机 Redis 密码>
JWT_SECRET=<至少 32 字符随机 JWT 密钥>
DATA_ENCRYPTION_KEYS=v1=<32 字节随机数据密钥>
DATA_ENCRYPTION_CURRENT_VERSION=v1
DB_AUTO_MIGRATE=false
WORKER_MONITOR_TOKEN=<强随机监控 Token>
GRAFANA_ADMIN_PASSWORD=<强随机 Grafana 密码>
ALLOWED_ORIGINS=https://your-domain.example
# 直连为 none；反代时仅填写实际 Nginx/Ingress IP 或 CIDR。
TRUSTED_PROXIES=none
```

```bash
docker compose build
docker compose up -d
```

`backend-migrate`、`backend-api`、`backend-worker` 使用同一个镜像，分别执行 `migrate`、`api`、`worker`；迁移成功后业务进程才启动。后端端口默认只绑定宿主机回环地址。

### Kubernetes

部署前创建 Secret：

```bash
kubectl create namespace caiyun
kubectl -n caiyun create secret generic caiyun-secrets \
  --from-literal=mysql-root-password='<强随机 root 密码>' \
  --from-literal=mysql-password='<强随机应用数据库密码>' \
  --from-literal=redis-password='<强随机 Redis 密码>' \
  --from-literal=jwt-secret='<至少 32 字符随机 JWT 密钥>' \
  --from-literal=data-encryption-keys='v1=<32 字节随机数据密钥>' \
  --from-literal=worker-monitor-token='<强随机 Worker 监控 Token>' \
  --from-literal=smtp-password='<SMTP 密码，可为空>'
```

每次发布必须重建固定名称迁移 Job；已完成的 Job 不会因 `kubectl apply` 自动重跑，PodTemplate 变化还会触发 immutable 校验。使用：

```bash
bash scripts/deploy-k8s.sh
```

脚本会删除旧 `caiyun-migrate`、应用 `k8s/caiyun.yaml`、等待迁移完成，再等待 API、Worker、Frontend rollout。示例后端镜像统一为 `caiyun-backend:2.1.0`，生产请替换为不可变 tag 或 digest，并把 `TRUSTED_PROXIES` 改为真实 Ingress 网段。

### 裸机 / 宝塔发布

生产发布顺序：

1. 备份数据库和当前发布目录。
2. 使用新制品执行 `caiyun-linux migrate`。
3. 执行 `caiyun-linux migrate --validate-only`。
4. 灰度 API。
5. 发布 Worker。
6. 检查 `/startupz`、`/readyz`、`/livez`。

构建与打包：

```powershell
.\scripts\build-linux.ps1 -Version local-unified
cd frontend
npm install
npm run build
cd ..
.\scripts\package-release.ps1 -Version local-unified
.\scripts\release-smoke-test.ps1 -Version local-unified
```

服务器部署和回滚：

```bash
bash scripts/deploy-linux.sh --target /www/wwwroot/caiyun --health-check
bash scripts/rollback-linux.sh --target /www/wwwroot/caiyun
```

部署脚本默认运行同版本 `migrate`；只有外部流水线已经完成迁移时才使用 `--skip-migrations`。迁移失败时不会替换文件，并会尝试恢复原 API/Worker。生产 `.env` 由服务器独立维护，不要用示例文件覆盖。

发布目录包含：

- `caiyun-linux`
- `caiyun-frontend-<version>.tar.gz`
- `caiyun-migrations-<version>.tar.gz`
- 监控、日历、SBOM 和 `SHA256SUMS`
- 部署、回滚、健康检查、归档与密钥轮换脚本

探针语义：`/livez` 只表示进程存活；`/readyz` 校验 MySQL、Redis 和队列；`/startupz` 表示配置、依赖及初始化已完成。

---
## 安全与质量检查

提交或部署前建议执行以下本地检查：

```bash
# 后端
cd backend
go test ./...
go vet ./...

# 前端
cd ../frontend
npm run typecheck
npm run lint -- --quiet
npm run build
npm run e2e
# 可选：生成 dist/bundle-report.json，分析前端资源体积
npm run analyze
```

Windows/PowerShell 环境可直接使用本地 CI 等价脚本。脚本会执行后端测试、覆盖率编译、`go vet`、前端 typecheck/lint/unit/build/E2E；`npm audit` 需要向 npm registry 发送依赖信息，默认不执行，可通过 `-WithAudit` 显式开启：

```powershell
.\scripts\ci-local.ps1 -SkipInstall
# 如需同时执行 npm audit：
.\scripts\ci-local.ps1 -SkipInstall -WithAudit
```

如需同时验证真实 Redis 队列集成测试：

```powershell
.\scripts\ci-local.ps1 -SkipInstall -WithRedisIntegration -RedisAddr 127.0.0.1:6379 -RedisDB 15
```

如需验证真实 MySQL 的版本化迁移与 Schema 校验集成测试，可在具备 MySQL 8 的环境执行：

```bash
cd backend
CAIYUN_MYSQL_INTEGRATION=1 CAIYUN_TEST_MYSQL_ADDR=127.0.0.1:3306 CAIYUN_TEST_MYSQL_USER=root CAIYUN_TEST_MYSQL_PASSWORD=your_root_password go test ./internal/dbmigrate -run MySQLIntegration -count=1
```

可选真实 Redis 队列集成测试默认跳过，如需验证生产 Redis 行为：

```bash
cd backend
CAIYUN_REDIS_INTEGRATION=1 \
CAIYUN_TEST_REDIS_ADDR=127.0.0.1:6379 \
CAIYUN_TEST_REDIS_DB=15 \
go test ./internal/queue -run RedisIntegration -count=1
```

更多说明：

- 前端 E2E：[`frontend/docs/E2E_TESTING.md`](frontend/docs/E2E_TESTING.md)
- Redis 队列集成测试：[`backend/docs/REDIS_QUEUE_INTEGRATION_TESTS.md`](backend/docs/REDIS_QUEUE_INTEGRATION_TESTS.md)
- Redis Streams 迁移评估：[`backend/docs/REDIS_STREAMS_QUEUE_MIGRATION.md`](backend/docs/REDIS_STREAMS_QUEUE_MIGRATION.md)
- 生产运行与观测清单：[`backend/docs/OPERATIONS_CHECKLIST.md`](backend/docs/OPERATIONS_CHECKLIST.md)
- 部署模板说明：[`deploy/README.md`](deploy/README.md)

当前安全基线包括：

- JWT 带 `token_version`，用户改密、找回密码、管理员重置密码、角色变更或删除后旧会话立即失效（认证快照缓存主动失效）
- Cookie 登录态启用 CSRF 校验，WebSocket 升级强制校验 Origin
- 用户维度限流在认证后执行，兑换和导出等接口按用户隔离限流
- 登录失败超过阈值后优先在 Redis 共享锁定；Redis 异常时降级到 `login_fail_locks` 表继续计数
- 前端已开启 TypeScript `strict`、`noUnusedLocals`、`noUnusedParameters`、`noImplicitReturns`
- 监控接口 Token 使用常量时间比较，防时序攻击
- AI 用户 ID 加密密钥支持通过 `CAIYUN_AI_USERID_KEY` 环境变量注入，`release` 模式不再降级为明文或 Base64
- 外部 HTTP 响应体读取设置大小上限，错误链路截断响应体，短信认证响应日志只记录摘要
- 非 AppError 内部错误一律返回通用消息，详情只入审计日志
- 数据库 schema 启动期只校验不自动变更，与 DBA 流程解耦
- Nginx 示例包含 HTTPS 跳转、HSTS、CSP（`connect-src` 收紧到本域 WSS）、`X-Content-Type-Options`、`X-Frame-Options`、`Referrer-Policy` 等安全头
- K8s 示例固定镜像版本，并配置 `securityContext`、资源限制和健康探针
- K8s 示例中 MySQL/Redis 使用 StatefulSet；API/Frontend 使用 2 副本并配置 PDB
- Docker Compose 服务全部配置 `healthcheck`、`restart: unless-stopped` 与资源 limits

---

## 运行机制

### 自动任务执行流程

```text
1. 调度器拉起待执行任务
2. 校验账号状态与 Token
3. 获取并刷新 JWT / ssoToken 上下文
4. 调用签到、任务中心、奖励领取等接口
5. 写入任务日志与系统日志
6. 更新账号与任务状态
```

### 兑换执行流程

```text
1. 到达设定兑换时间
2. 预加载待执行抢兑队列
3. 按管理员后台配置的并发数执行多个账号
4. 每个账号只提交一次抢兑请求并记录结果
5. 成功后继续下一个账号；商品无库存/已兑完/已下架时停止当前商品后续账号
6. 同步日志、状态与统计数据
```

### Token 处理机制

- 任务执行前优先校验账号可用性
- Authorization 距离过期不足 5 天时会尝试预刷新；刷新失败但旧票据仍未过期时，不阻断普通任务执行
- 新版 refreshToken 成功后必须通过 `querySpecToken` 验证，验证成功才更新数据库
- 刷新成功后同步更新主账号与对应抢兑账号的鉴权信息
- JWT 获取失败支持重试，连续失败可自动禁用账号
- 签到中心请求使用与 JWT 对应的同一 `ssoToken` 预热上下文

### 任务注册表约定

- **代码注册表是任务定义的唯一真源**
- 任务定义位于：`backend/internal/services/task_catalog.go`
- 数据库初始化脚本中的 `task_configs` 仅负责提供首批种子数据
- API / Worker 启动时会通过 `SyncDefinitions` 自动对数据库中的任务定义做同步兜底

当前默认任务种子已与注册表保持一致，包括：

- `signin`
- `task_expansion_reward`
- `wechat`
- `wxdraw`
- `tasklist`
- `invitefriends`
- `shake`
- `receive`
- `messagepush`
- `revivalreward`
- `backupgift`
- `garden`（默认禁用）
- `redpacket`（默认禁用）
- `aicloud`（默认禁用）
- `cloudbattle`
- `blindbox`（默认禁用）
- `cloudphone`
- `todaycloud`
- `after_task`

---

## 仓库说明

以下内容属于辅助组件或运维脚本，不属于主系统核心部署链路：

- 根目录调试脚本 / 实验脚本
- 兑换相关独立样例或抓图脚本

生产部署建议优先发布版本化产物：

- `release/<version>-linux-amd64/caiyun-linux`
- `release/<version>-linux-amd64/caiyun-frontend-<version>.tar.gz`
- `release/<version>-linux-amd64/SHA256SUMS`

---

## Roadmap

- [x] 多账号统一托管
- [x] 自动任务中心
- [x] 商品同步与定时兑换
- [x] 任务中心 V2 适配
- [x] 分享文件 / 上传文件 / AI 相机等扩展任务
- [x] 移动端适配与响应式管理后台
- [ ] 更多任务编排策略
- [ ] 更细粒度的监控告警
- [ ] 更完善的自动化测试

---

## 贡献

欢迎通过以下方式参与改进：

1. 提交 Issue 反馈问题或需求
2. 提交 Pull Request 改进代码或文档
3. 补充任务适配、部署文档与测试用例

建议在提交前完成：

```bash
# 后端
cd backend
go test ./...
go vet ./...

# 前端
cd ../frontend
npm run typecheck
npm run lint -- --quiet
npm run build
```

---

## 许可证

本项目基于 [MIT License](LICENSE) 开源。
