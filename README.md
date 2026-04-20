# 移动云盘自动任务与兑换系统

![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white)
![Vue](https://img.shields.io/badge/Vue-3-42b883?logo=vue.js&logoColor=white)
![TypeScript](https://img.shields.io/badge/TypeScript-5.x-3178C6?logo=typescript&logoColor=white)
![MySQL](https://img.shields.io/badge/MySQL-8.0+-4479A1?logo=mysql&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green.svg)

基于 **Go 后端 + Vue 3 前端** 的移动云盘自动化平台，提供账号管理、日常任务执行、兑换调度、商品同步、日志审计与管理后台能力。项目同时支持可选的 Python 滑块识别服务，用于高频兑换场景中的验证码辅助处理。

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
- [文档索引](#文档索引)
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

### 1) 账号管理

- 多账号管理，支持同一手机号被不同用户分别绑定
- Auth / JWT 自动刷新
- JWT 获取失败自动重试，连续失败可自动禁用账号
- 账号健康检查、状态监控、过期账号隔离

### 2) 自动任务

- 每日签到
- 微信签到 / 微信抽奖
- 摇一摇
- 任务中心任务
- 备份礼包 / 膨胀奖励
- 邀请好友
- 消息推送奖励
- 今日云朵、盲盒、云朵大作战等扩展任务
- 复活卡奖励任务

### 3) 兑换中心

- 商品自动更新 / 手动同步
- 自定义兑换时间
- 多账号并发兑换
- 滑块验证码辅助识别
- 兑换日志与执行结果记录

### 4) 管理后台

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
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                Python 滑块识别服务（可选）                  │
│                   Flask + OpenCV + NumPy                   │
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
| 可选组件 | Python + OpenCV + Flask | 滑块验证码辅助识别 |

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
├── solver_service.py              # 可选滑块识别服务
└── README.md
```

---

## 快速开始

### 环境要求

- Go 1.21+
- Node.js 18+
- MySQL 8.0+
- Redis 6.0+
- Python 3.11+（可选，仅滑块识别需要）

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

### 6. 启动滑块识别服务（可选）

```bash
pip install flask opencv-python-headless numpy
python solver_service.py
```

---

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

```bash
docker-compose build
docker-compose up -d
```

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

## 文档索引

- API 文档：[`backend/docs/api.md`](backend/docs/api.md)
- 兑换接口说明：[`backend/docs/exchange_api.md`](backend/docs/exchange_api.md)
- 宝塔部署文档：[`backend/docs/DEPLOY_BT_PANEL.md`](backend/docs/DEPLOY_BT_PANEL.md)
- 历史改进记录：[`backend/docs/IMPROVEMENTS.md`](backend/docs/IMPROVEMENTS.md)

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
3. 多账号并发执行抢兑
4. 如有需要调用滑块识别
5. 提交兑换请求并记录结果
6. 同步日志、状态与统计数据
```

### Token 处理机制

- 任务执行前优先校验账号可用性
- JWT 获取失败支持重试
- 连续失败可自动禁用账号
- 签到中心请求使用与 JWT 对应的同一 `ssoToken` 预热上下文

---

## 仓库说明

以下内容属于辅助组件或运维脚本，不属于主系统核心部署链路：

- 根目录调试脚本 / 实验脚本
- 可选 Python 辅助识别脚本
- 兑换相关独立样例或抓图脚本

生产部署建议只发布：

- `backend/cmd/api`
- `backend/cmd/worker`
- `frontend`
- 可选 `solver_service.py`

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

# 前端
cd ../frontend
npm run build
```

---

## 许可证

本项目基于 [MIT License](LICENSE) 开源。
