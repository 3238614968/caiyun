# 移动云盘自动任务与兑换系统

![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white)
![Vue](https://img.shields.io/badge/Vue-3-42b883?logo=vue.js&logoColor=white)
![TypeScript](https://img.shields.io/badge/TypeScript-5.x-3178C6?logo=typescript&logoColor=white)
![MySQL](https://img.shields.io/badge/MySQL-8.0+-4479A1?logo=mysql&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green.svg)

基于 **Go 后端 + Vue 3 前端** 的移动云盘自动化平台，提供账号管理、日常任务执行、兑换调度、商品同步、日志审计与管理后台能力。

---

## 目录

- [项目简介](#项目简介)
- [核心能力](#核心能力)
- [本次新增与修复](#本次新增与修复)
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
- Auth / JWT 自动刷新
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
- 兑换日志与执行结果记录

### 5) 管理后台

- 用户管理、账号管理、任务配置、公告管理
- 商品中心、兑换账号、抢兑任务、领奖专区
- 系统日志与任务日志查看
- 响应式布局与移动端适配

---

## 本次新增与修复

> 以下内容已纳入当前代码与构建产物。

### 新增功能

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

### 修复与优化

- **修复签到上下文构造不一致**
  - 补齐签到中心预热请求
  - 补齐 `clientVersion / User-Agent / sourceid / deviceId / userDomainId` 等上下文
- **修复签到状态判断不一致**
  - 新增对 `cal[].s` 的兜底判断，避免仅依赖旧字段导致状态误判
- **修复 JWT / SSO 上下文错配**
  - 统一复用同一次获取的 `ssoToken` 与其对应 JWT
  - 避免签到预热页与 JWT 来源不一致导致 `infoV3 / startSignIn / taskListV2` 失败
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
              ┌───────────────┴───────────────┐
              ▼                               ▼
┌─────────────────────────┐      ┌─────────────────────────┐
│       Go API 服务       │      │      Go Worker 服务     │
│  - 用户认证 / 接口层    │      │  - 定时任务 / 调度执行  │
│  - 账号 / 任务 / 商品   │      │  - 自动任务 / 抢兑执行  │
│  - 日志 / 管理后台接口  │      │  - 日志记录 / 状态同步  │
└─────────────────────────┘      └─────────────────────────┘
              │                               │
              └───────────────┬───────────────┘
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
| 后端 API | Go + Gin | RESTful API |
| 后端 Worker | Go | 任务调度、自动执行、抢兑执行 |
| 数据库 | MySQL 8.0+ | 持久化存储 |
| 缓存 | Redis | 队列、缓存、状态管理 |

---

## 目录结构

```text
.
├── backend/                       # Go 后端
│   ├── cmd/
│   │   ├── api/                   # API 服务入口
│   │   └── worker/                # Worker 服务入口
│   ├── configs/                   # 配置模板
│   ├── docs/                      # 项目文档
│   ├── internal/
│   │   ├── core/                  # HTTP / Auth / API / Task 核心能力
│   │   ├── handlers/              # HTTP 处理器
│   │   ├── middleware/            # 中间件
│   │   ├── models/                # 数据模型
│   │   ├── repository/            # 数据访问层
│   │   └── services/              # 业务服务层
│   ├── migrations/                # 数据库初始化脚本
│   ├── api-linux                  # Linux amd64 API 构建产物
│   └── worker-linux               # Linux amd64 Worker 构建产物
├── frontend/                      # Vue 前端
│   ├── src/
│   └── dist/                      # 前端生产构建产物
├── nginx-server.conf              # Nginx 配置示例
├── docker-compose.yml             # Docker Compose 示例
└── README.md
```

---

## 快速开始

### 环境要求

- Go 1.21+
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
go run cmd/api/main.go
```

### 4. 启动 Worker

```bash
cd backend
go run cmd/worker/main.go
```

### 5. 启动前端

```bash
cd frontend
npm install
npm run dev
```

## 构建与部署

### 本地前端构建

```bash
cd frontend
npm install
npm run build
```

前端构建输出目录：

```text
frontend/dist
```

### 后端 Linux amd64 编译

> 当前项目默认以后端 **Linux / amd64** 为目标构建平台。

```bash
cd backend
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o api-linux ./cmd/api
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o worker-linux ./cmd/worker
```

构建输出文件：

```text
backend/api-linux
backend/worker-linux
```

### Docker 部署

首次启动前建议在项目根目录创建 `.env`，至少填写以下变量：

```bash
MYSQL_ROOT_PASSWORD=replace_with_strong_root_password
MYSQL_USER=caiyun_app
MYSQL_PASSWORD=replace_with_strong_app_password
REDIS_PASSWORD=replace_with_strong_redis_password
JWT_SECRET=replace_with_32_chars_random_secret
WORKER_MONITOR_TOKEN=replace_with_random_monitor_token
GRAFANA_ADMIN_PASSWORD=replace_with_strong_grafana_password
# 逗号分隔。必须包含浏览器实际访问前端的 Origin，例如域名、局域网 IP 或非 80 端口。
ALLOWED_ORIGINS=http://localhost,http://127.0.0.1,http://your-domain.com

# 可选：启用密码找回邮件
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USERNAME=your_smtp_username
SMTP_PASSWORD=your_smtp_password_or_app_password
SMTP_FROM=no-reply@example.com
SMTP_FROM_NAME=移动云盘
SMTP_USE_TLS=false
```

Compose 模式下前端 Nginx 会统一代理 `/api` 与 `/ws` 到 `backend-api:8080`，后端端口仅绑定到宿主机 `127.0.0.1`。如果通过 `http://192.168.x.x`、`https://example.com` 或 `http://localhost:8088` 访问前端，请同步把这些完整 Origin 加入 `ALLOWED_ORIGINS`，否则跨域 API 或 WebSocket Origin 校验会拒绝连接。

```bash
docker-compose build
docker-compose up -d
```

Compose 会启动以下核心服务：

- `mysql`：初始化并持久化业务数据库
- `redis`：启用 `requirepass`，用于队列、缓存和密码找回验证码
- `backend-api`：仅绑定到宿主机 `127.0.0.1:8080`
- `backend-worker`：仅绑定到宿主机 `127.0.0.1:8081`，监控接口需要 `WORKER_MONITOR_TOKEN`
- `frontend`：对外暴露 Web 页面，并代理 `/api`、`/ws`
- `grafana`：示例监控面板，仅绑定到宿主机 `127.0.0.1:3000`

### Kubernetes 部署示例

项目提供单文件示例清单：

```text
k8s/caiyun.yaml
```

部署前必须先创建业务 Secret，不要使用弱口令：

```bash
kubectl apply -f k8s/caiyun.yaml --dry-run=client

kubectl create namespace caiyun
kubectl -n caiyun create secret generic caiyun-secrets \
  --from-literal=mysql-root-password='<强随机 root 密码>' \
  --from-literal=mysql-password='<强随机应用数据库密码>' \
  --from-literal=redis-password='<强随机 Redis 密码>' \
  --from-literal=jwt-secret='<至少 32 字符随机 JWT 密钥>' \
  --from-literal=worker-monitor-token='<强随机 Worker 监控 Token>' \
  --from-literal=smtp-password='<SMTP 密码，可为空>'

kubectl apply -f k8s/caiyun.yaml
```

说明：

- `k8s/caiyun.yaml` 已内置 `caiyun-mysql-init-sql` ConfigMap，首次启动 MySQL 空 PVC 时会自动执行初始化 SQL。
- MySQL 使用 `mysql-data` PVC，请根据集群 StorageClass 调整容量、回收策略和备份方案。
- 示例镜像名固定为 `caiyun-api:2.1.0`、`caiyun-worker:2.1.0`、`caiyun-frontend:2.1.0`，实际部署前请替换为你的镜像仓库地址、不可变版本号或镜像 digest。
- `ALLOWED_ORIGINS` 默认是占位域名，请改为浏览器实际访问前端的完整 Origin。
- API、Worker、Frontend 示例已配置非 root、只读根文件系统、能力收敛、资源 requests/limits 和健康探针；如镜像运行用户变化，请同步调整 `securityContext`。
- 如果使用外部 MySQL，可移除清单中的 MySQL Deployment/PVC/Service，并确保外部库已执行初始化 SQL。

### 宝塔面板部署

详见：[`backend/docs/DEPLOY_BT_PANEL.md`](backend/docs/DEPLOY_BT_PANEL.md)

### 手动部署示例

```bash
# 1) 编译后端
cd backend
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o api-linux ./cmd/api
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o worker-linux ./cmd/worker

# 2) 编译前端
cd ../frontend
npm install
npm run build

# 3) 上传构建产物
scp -r ../backend/api-linux ../backend/worker-linux root@server:/www/wwwroot/caiyun/
scp -r dist root@server:/www/wwwroot/caiyun/frontend/
```

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
```

当前安全基线包括：

- JWT 带 `token_version`，用户改密、找回密码、管理员重置密码后旧会话立即失效
- Cookie 登录态启用 CSRF 校验
- 用户维度限流在认证后执行，兑换和导出等接口按用户隔离限流
- 外部 HTTP 响应体读取设置大小上限，短信认证响应日志只记录摘要
- Nginx 示例包含 HTTPS 跳转、HSTS、CSP、`X-Content-Type-Options`、`X-Frame-Options`、`Referrer-Policy` 等安全头
- K8s 示例固定镜像版本，并配置 `securityContext`、资源限制和健康探针

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
- JWT 获取失败支持重试
- 连续失败可自动禁用账号
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

生产部署建议只发布：

- `backend/cmd/api`
- `backend/cmd/worker`
- `frontend`

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
