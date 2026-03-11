# 移动云盘管理系统

![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)
![Vue](https://img.shields.io/badge/Vue-3.0+-4FC08D?style=flat&logo=vue.js)
![TypeScript](https://img.shields.io/badge/TypeScript-5.0+-3178C6?style=flat&logo=typescript)
![License](https://img.shields.io/badge/License-MIT-green.svg)
![Status](https://img.shields.io/badge/Status-Production%20Ready-brightgreen)

> 基于 Go + Vue 3 的企业级前后端分离项目，用于管理移动云盘账号、执行自动化任务、统计云朵数据，并提供兑换中心（商品同步、抢兑任务、记录导出、管理配置等）能力。

## ✨ 功能特性

### 🎯 核心功能
- **17+ 自动化任务** - 签到、抽奖、摇一摇、盲盒、红包、AI云朵等
- **兑换中心** - 商品搜索、分类、抢兑任务、记录导出
- **账号管理** - 增删改查、短信验证码登录、Token自动刷新
- **任务调度** - Cron定时任务、批量触发、任务状态监控
- **数据统计** - 云朵统计、趋势分析、仪表盘可视化

### 🚀 企业级特性
- **微服务架构** - API服务 + Worker服务分离
- **Redis缓存** - 高性能缓存与队列状态管理
- **Prometheus监控** - 完整的监控指标暴露
- **WebSocket实时通知** - 任务状态实时推送
- **审计日志** - 关键操作完整记录
- **JWT鉴权** - 安全的用户认证与授权

### 📦 部署支持
- **Docker部署** - 一键启动完整环境
- **宝塔面板部署** - 完整的宝塔部署文档
- **本地开发** - 完善的开发环境配置
- **Windows构建Linux** - 跨平台编译支持

## 📊 项目完成度

| 模块 | 完成度 | 说明 |
|------|--------|------|
| 任务模块 | 100% | 17个任务全部实现 |
| API集成 | 100% | 40+ API全部实现 |
| 兑换中心 | 100% | 完整的兑换中心功能 |
| 管理员功能 | 100% | 完整的管理后台 |
| 监控运维 | 100% | Prometheus + WebSocket |
| 统计功能 | 100% | 多维度数据统计 |
| **整体完成度** | **95%** | **生产环境就绪** |

## 🏗️ 架构优势

- **微服务架构** - API服务和Worker服务分离，提高可扩展性
- **企业级功能** - 完整的兑换中心、管理员后台、审计日志
- **监控完善** - Prometheus监控指标、WebSocket实时通知
- **数据库升级** - MySQL + GORM，支持更大数据量
- **部署灵活** - 支持Docker、宝塔面板、本地开发多种方式

当前仓库包含：
- 后端 API 服务（`backend/cmd/api`）
- 后端 Worker 服务（`backend/cmd/worker`）
- 前端管理界面（`frontend`）
- 数据库初始化/迁移脚本（`backend/scripts`、`backend/migrations`）

## 功能概览

### 用户侧功能
- 用户注册、登录、JWT 鉴权与刷新
- 云盘账号管理（增删改查、短信验证码登录、Token 刷新）
- 手动触发单账号任务 / 批量触发任务
- 任务执行日志、队列状态、任务状态查询
- 云朵统计、趋势统计、仪表盘数据

### 兑换中心功能
- 商品搜索、分类、商品列表更新
- 兑换账号管理（独立于普通账号的抢兑配置）
- 抢兑任务管理（创建、编辑、删除、立即执行、批量执行）
- 兑换记录查询与导出（CSV/JSON）
- 管理员兑换配置、月卡兑换执行

### 管理员功能
- 用户/账号总览与状态管理
- 任务配置管理
- 系统统计概览
- 审计日志记录（关键管理员操作）
- 兑换配置管理与月卡兑换执行

### 系统能力
- WebSocket 实时通知
- Redis 缓存与队列状态统计
- 定时任务调度（cron）
- Prometheus 指标暴露（API 与 Worker）
- Worker 健康检查与状态接口

## 项目结构（简版）

```text
移动云盘/
├─ backend/
│  ├─ cmd/
│  │  ├─ api/                  # API 服务入口
│  │  └─ worker/               # Worker 服务入口
│  ├─ configs/                 # 配置示例、监控面板配置
│  ├─ docs/                    # 部署/接口/改进文档
│  ├─ internal/
│  │  ├─ handlers/             # HTTP 处理器
│  │  ├─ services/             # 业务服务
│  │  ├─ repository/           # 数据访问层
│  │  ├─ models/               # 数据模型
│  │  ├─ core/                 # 核心 API/任务逻辑/SMS/配置等
│  │  ├─ middleware/           # 中间件（鉴权/审计等）
│  │  ├─ scheduler/            # 定时任务调度
│  │  ├─ queue/                # 队列能力
│  │  ├─ concurrency/          # 并发执行控制
│  │  ├─ monitor/              # Prometheus/运行监控
│  │  ├─ cache/                # Redis 缓存
│  │  └─ ws/                   # WebSocket 推送
│  ├─ migrations/              # Docker 初始化迁移脚本（历史/增量）
│  └─ scripts/                 # 初始化脚本（含整合版 init_caiyun_database.sql）
├─ frontend/
│  ├─ src/
│  │  ├─ views/                # 页面（登录、仪表盘、兑换中心、记录、管理后台等）
│  │  ├─ components/           # 公共组件
│  │  ├─ api/                  # 前端 API 封装
│  │  ├─ store/                # Pinia 状态管理
│  │  └─ router/               # 路由
│  └─ public/
├─ scripts/                    # 宝塔部署脚本等
├─ docker-compose.yml          # Docker 编排
└─ README.md
```

## 技术栈

### 后端
- Go 1.21（`backend/go.mod`）
- Gin（HTTP 框架）
- GORM + MySQL
- Redis（缓存 / 队列状态）
- JWT（鉴权）
- robfig/cron（定时任务）
- Prometheus client（监控指标）
- Gorilla WebSocket（实时通知）

### 前端
- Vue 3 + TypeScript
- Vite
- Pinia
- Vue Router
- Element Plus
- Axios
- ECharts

## 快速开始

### 方式一：宝塔面板部署（推荐）
- 详细文档：`backend/docs/DEPLOY_BT_PANEL.md`
- 快速部署：`backend/docs/DEPLOY_BT_PANEL_QUICK.md`

```bash
bash scripts/bt_deploy.sh
```

### 方式二：Docker 部署

```bash
docker-compose up -d
docker-compose logs -f
docker-compose down
```

说明：
- 当前 `docker-compose.yml` 已默认挂载 `backend/scripts/init_caiyun_database.sql` 作为 MySQL 初始化脚本。
- 本仓库已补充 `backend/Dockerfile.api`、`backend/Dockerfile.worker`、`frontend/Dockerfile`，并为前端提供 SPA 路由回退配置（`frontend/nginx.conf`）。

### 方式三：本地开发

#### 后端（API + Worker）

```bash
cd backend

go mod download

# 复制环境变量示例（程序默认读取 backend/.env）
cp configs/.env.example .env

# 启动 API 服务
go run cmd/api/main.go

# 启动 Worker 服务
go run cmd/worker/main.go

# 可选：运行后端测试
go test ./...
```

#### 前端

```bash
cd frontend

npm install
npm run dev

# 类型检查 / 代码检查 / 构建
npm run typecheck
npm run lint
npm run build
```

前端本地代理配置见 `frontend/vite.config.ts`（默认代理到 `http://localhost:8080`）。

### Windows 构建 Linux 二进制（后端）

```powershell
.\build-linux.ps1
```

### 一键构建前后端并打包（Linux amd64）

```bash
bash scripts/build_release.sh
```

构建完成后会在 `build/caiyun-linux-amd64.tar.gz` 生成可下载发布包，包含：
- 前端静态文件（`frontend/`）
- 后端 Linux amd64 二进制（`backend/caiyun-api`、`backend/caiyun-worker`）
- `.env.example` 与 `README.md`

## 环境变量配置（后端）

后端运行时默认从 `backend/.env` 读取配置。完整示例见：`backend/configs/.env.example`。

关键配置示例：

```env
PORT=8080
WORKER_MONITOR_PORT=8081
GIN_MODE=release

DB_HOST=localhost
DB_PORT=3306
DB_USER=root
DB_PASSWORD=your_password_here
DB_NAME=caiyun

REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0

JWT_SECRET=your_32_character_random_secret_here
TASK_CONCURRENCY=5
TASK_SCHEDULE=0 2 * * *

EXCHANGE_CONCURRENCY=5
EXCHANGE_SCHEDULE_TIME_1=10:00
EXCHANGE_SCHEDULE_TIME_2=20:00
EXCHANGE_AUTO_UPDATE_PRODUCTS=true

# 短信服务（可选）
CAIYUN_SMS_API_BASE_URL=https://caiyun.feixin.10086.cn
CAIYUN_SMS_INSECURE_SKIP_VERIFY=false

# 当 YAML 配置文件未提供 auth 时回退读取
CAIYUN_AUTH=
```

说明：
- `WORKER_MONITOR_PORT` 为 Worker 监控 HTTP 服务端口（`/health`、`/status`、`/metrics`）。
- `CAIYUN_SMS_INSECURE_SKIP_VERIFY` 仅建议调试环境使用，生产环境保持 `false`。
- `.env.example` 中保留了 `WORKER_PORT` 兼容旧配置名，当前主程序不读取。

## 数据库初始化（以 `init_caiyun_database.sql` 为主）

推荐使用整合脚本：`backend/scripts/init_caiyun_database.sql`

```bash
mysql -u root -p < backend/scripts/init_caiyun_database.sql
```

脚本特点：
- 包含库创建与表结构初始化（`CREATE DATABASE IF NOT EXISTS caiyun`）
- 包含兑换中心相关表、审计日志表、WebSocket 消息表、任务历史表
- 包含默认系统配置/任务配置初始化
- 包含默认管理员账号初始化（默认账号 `admin`，默认密码 `admin123`）
- 带补字段逻辑，适合重复执行（幂等化补丁风格）

主要表（节选）：
- `users`
- `accounts`
- `task_logs`
- `cloud_stats`
- `system_configs`
- `task_configs`
- `products`
- `exchange_accounts`
- `exchange_tasks`
- `exchange_records`
- `exchange_task_history`
- `audit_logs`
- `web_socket_messages`

安全提示：
- 首次登录后请立即修改默认管理员密码。

## API 接口概览（以 `backend/cmd/api/main.go` 为准）

### 认证相关
- `POST /api/auth/register` - 用户注册
- `POST /api/auth/login` - 用户登录
- `POST /api/auth/refresh` - 刷新 Token
- `GET /api/auth/me` - 获取当前用户信息

### 账号管理
- `GET /api/accounts` - 获取账号列表
- `POST /api/accounts` - 添加账号
- `GET /api/accounts/:id` - 获取单个账号
- `PUT /api/accounts/:id` - 更新账号
- `DELETE /api/accounts/:id` - 删除账号
- `PUT /api/accounts/:id/status` - 更新账号启用状态
- `POST /api/accounts/:id/refresh` - 刷新账号 Token
- `POST /api/accounts/:id/trigger` - 手动触发账号任务
- `POST /api/accounts/sms/send` - 发送短信验证码
- `GET /api/accounts/sms/status/:phone` - 查询短信验证码状态
- `POST /api/accounts/sms/verify` - 短信验证码登录

### 任务与统计
- `GET /api/tasks/logs` - 获取任务日志
- `POST /api/tasks/trigger-all` - 触发所有任务
- `GET /api/tasks/queue-status` - 获取任务队列状态
- `GET /api/tasks/status` - 获取任务状态汇总
- `GET /api/stats/dashboard` - 仪表盘数据
- `GET /api/stats/cloud` - 云朵统计
- `GET /api/stats/trend` - 趋势数据
- `POST /api/stats/calculate` - 计算统计
- `GET /api/stats/total-cloud` - 总云朵数

### 兑换中心
- `GET /api/products/search` - 搜索商品列表
- `GET /api/products/categories` - 商品分类
- `POST /api/products/update` - 手动更新商品（需账号 ID）
- `GET /api/exchange/accounts` - 兑换账号列表
- `POST /api/exchange/accounts` - 添加兑换账号
- `PUT /api/exchange/accounts/:id` - 更新兑换账号
- `DELETE /api/exchange/accounts/:id` - 删除兑换账号
- `GET /api/exchange/tasks` - 抢兑任务列表
- `POST /api/exchange/tasks` - 创建抢兑任务
- `PUT /api/exchange/tasks/:id` - 更新抢兑任务
- `DELETE /api/exchange/tasks/:id` - 删除抢兑任务
- `POST /api/exchange/tasks/:id/execute` - 立即执行抢兑任务
- `POST /api/exchange/tasks/batch-execute` - 批量执行抢兑任务
- `GET /api/exchange/records` - 兑换记录列表
- `GET /api/exchange/records/export` - 导出兑换记录（CSV/JSON）

### 管理员
- `GET /api/admin/users` - 用户列表
- `DELETE /api/admin/users/:id` - 删除用户
- `PUT /api/admin/users/:id/role` - 更新用户角色
- `GET /api/admin/accounts` - 账号列表（管理员视角）
- `GET /api/admin/accounts/summaries` - 账号汇总
- `PUT /api/admin/accounts/:id/status` - 更新账号状态
- `DELETE /api/admin/accounts/:id` - 删除账号
- `GET /api/admin/dashboard` - 管理员仪表盘
- `GET /api/admin/stats/overview` - 统计概览
- `GET /api/admin/task-configs` - 任务配置列表
- `PUT /api/admin/task-configs/:task_type` - 更新任务配置
- `GET /api/admin/exchange/config` - 获取兑换配置
- `PUT /api/admin/exchange/config` - 更新兑换配置
- `POST /api/admin/exchange/execute-monthly` - 立即执行月卡兑换

### WebSocket 与监控
- `WS /ws` - WebSocket 实时通知
- `GET /health` - API 健康检查
- `GET /metrics` - API Prometheus 指标
- `GET :WORKER_MONITOR_PORT/health` - Worker 健康检查（默认 `8081`）
- `GET :WORKER_MONITOR_PORT/status` - Worker 状态摘要
- `GET :WORKER_MONITOR_PORT/metrics` - Worker Prometheus 指标

## 自动化任务清单（当前服务层执行列表）

后端任务服务当前按顺序执行以下 17 项任务（见 `backend/internal/services/task_service.go`）：

| 任务标识 | 说明（中文） |
|---|---|
| `signin` | 每日签到 |
| `tasklist` | 获取任务列表/状态 |
| `wechat` | 微信相关任务 |
| `wxdraw` | 微信抽奖任务 |
| `todaycloud` | 今日云朵领取 |
| `invitefriends` | 邀请好友任务 |
| `shake` | 摇一摇 |
| `receive` | 奖励领取 |
| `messagepush` | 消息推送奖励 |
| `backupgift` | 备份礼包 |
| `blindbox` | 盲盒任务 |
| `redpacket` | 红包任务 |
| `aicloud` | AI 云朵任务 |
| `garden` | 果园任务 |
| `cloudphone` | 云手机任务 |
| `cloudbattle` | 云朵大作战 |
| `exchange` | 兑换相关任务 |

说明：具体是否执行还会受到任务配置开关和账号状态影响。

## 监控与运维建议

- API 和 Worker 都已支持 Prometheus 指标，建议配合 Grafana 使用（可参考 `backend/configs/grafana_dashboard.json`）。
- 建议反向代理层限制管理接口访问来源，并启用 HTTPS。
- 定期检查 `task_logs`、`audit_logs`、`exchange_records` 的数据增长，必要时做归档或清理策略。

## 常见问题

### 1. `.env.example` 是否可以直接用？
可以作为模板使用，但必须至少修改以下配置：
- `DB_*`
- `REDIS_*`（如果有密码）
- `JWT_SECRET`
- `CAIYUN_AUTH`（按你的运行方式决定是否使用）

### 2. 为什么 Docker 启动后数据库结构与 `init_caiyun_database.sql` 不完全一致？
如果你之前已经初始化过 MySQL 数据卷（`mysql-data`），Docker 不会再次执行初始化脚本，因此可能看到旧结构。可在确认数据可清理后执行 `docker-compose down -v` 重新初始化，或手动导入 `backend/scripts/init_caiyun_database.sql`。

### 3. 短信接口 TLS 校验是否默认开启？
是。`CAIYUN_SMS_INSECURE_SKIP_VERIFY` 默认应为 `false`，仅在调试环境临时排查时才建议设为 `true`。

## 文档索引

### 核心文档
- `README.md` - 项目主文档（本文档）
- `功能完成度分析报告.md` - 详细的功能完成度分析报告
- `caiyun.md` - 项目审计与改进建议
- `AI开发框架.md` - AI开发框架说明
- `ARCHITECTURE.md` - 系统架构文档

### 后端文档
- `backend/docs/api.md` - 后端接口说明（历史文档）
- `backend/docs/exchange_api.md` - 兑换中心接口说明
- `backend/docs/DEPLOY_BT_PANEL.md` - 宝塔部署指南
- `backend/docs/IMPROVEMENTS.md` - 改进项记录
- `backend/docs/real-integration-checklist.md` - 真实联调检查清单

### 部署文档
- `scripts/bt_deploy.sh` - 宝塔部署脚本
- `docker-compose.yml` - Docker编排配置
- `backend/Dockerfile.api` - API服务Dockerfile
- `backend/Dockerfile.worker` - Worker服务Dockerfile
- `frontend/Dockerfile` - 前端Dockerfile

### API文档
- `移动云盘SMS_APIv1.1.md` - 移动云盘短信API文档

## 🤝 贡献指南

欢迎贡献代码、报告问题或提出改进建议！

### 贡献方式

1. **Fork 本仓库**
   ```bash
   # 点击 GitHub 页面右上角的 Fork 按钮
   ```

2. **克隆你的 Fork**
   ```bash
   git clone https://github.com/your-username/移动云盘.git
   cd 移动云盘
   ```

3. **创建特性分支**
   ```bash
   git checkout -b feature/your-feature-name
   ```

4. **提交更改**
   ```bash
   git add .
   git commit -m "Add some feature"
   ```

5. **推送到你的 Fork**
   ```bash
   git push origin feature/your-feature-name
   ```

6. **创建 Pull Request**
   - 访问 GitHub 上的原始仓库
   - 点击 "Pull Request" 按钮
   - 填写 PR 描述并提交

### 代码规范

#### Go 代码规范
- 遵循 [Effective Go](https://golang.org/doc/effective_go) 指南
- 使用 `gofmt` 格式化代码
- 添加必要的注释和文档
- 编写单元测试

#### Vue/TypeScript 代码规范
- 遵循 [Vue 3 风格指南](https://vuejs.org/style-guide/)
- 使用 TypeScript 类型检查
- 组件命名使用 PascalCase
- 添加必要的注释

### 提交信息规范

遵循 [Conventional Commits](https://www.conventionalcommits.org/) 规范：

```
<type>(<scope>): <subject>

<body>

<footer>
```

类型（type）：
- `feat`: 新功能
- `fix`: 修复bug
- `docs`: 文档更新
- `style`: 代码格式调整
- `refactor`: 重构
- `test`: 测试相关
- `chore`: 构建/工具相关

示例：
```
feat(exchange): add batch exchange task execution

- Add batch execution API endpoint
- Implement concurrent task execution
- Add progress tracking
```

### 问题报告

报告问题时，请提供以下信息：

1. **环境信息**
   - 操作系统版本
   - Go/Node.js 版本
   - 数据库版本

2. **问题描述**
   - 清晰描述问题
   - 复现步骤
   - 预期行为
   - 实际行为

3. **日志信息**
   - 相关的错误日志
   - 控制台输出

4. **截图/录屏**（如适用）

## 📄 License

MIT License

Copyright (c) 2024 移动云盘管理系统

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

## ⚠️ 免责声明

本项目仅供学习与技术交流使用，请勿用于违反平台规则或法律法规的用途。

使用本软件所产生的一切后果由使用者自行承担，开发者不承担任何责任。

## 📞 联系方式

- **问题反馈**: [GitHub Issues](https://github.com/your-username/移动云盘/issues)
- **功能建议**: [GitHub Discussions](https://github.com/your-username/移动云盘/discussions)

---

<div align="center">

**如果这个项目对你有帮助，请给一个 ⭐️ Star**

Made with ❤️ by 移动云盘管理系统团队

</div>
