# 移动云盘自动任务与兑换系统

基于 Go 后端 + Vue 前端的主系统，可选接入 Python 滑块识别和辅助脚本，支持移动云盘账号管理、自动任务执行、奖品兑换等功能。

## 系统架构

```
┌─────────────────────────────────────────────────────────────┐
│                        前端 (Vue.js)                         │
│                   Element Plus + TypeScript                  │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼ HTTPS
┌─────────────────────────────────────────────────────────────┐
│                      Nginx 反向代理                          │
└─────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┴───────────────┐
              ▼                               ▼
┌─────────────────────────┐      ┌─────────────────────────┐
│    Go API 服务          │      │    Go Worker 服务       │
│    (Gin Framework)      │      │    (任务调度/执行)       │
│    - 用户认证           │      │    - 定时任务           │
│    - 账号管理           │      │    - 队列处理           │
│    - 任务管理           │      │    - 兑换执行           │
│    - 兑换中心           │      │                         │
└─────────────────────────┘      └─────────────────────────┘
              │                               │
              └───────────────┬───────────────┘
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                      MySQL 数据库                            │
│              账号、任务、兑换记录、配置等                       │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼ HTTP API (可选)
┌─────────────────────────────────────────────────────────────┐
│              Python 滑块验证码识别服务                        │
│                  (OpenCV + Flask)                            │
│              端口: 5000 (本地开发使用)                        │
└─────────────────────────────────────────────────────────────┘
```

## 非生产/辅助组件
- 根目录 `main.go`：演示/调试用兑换脚本，默认 JWT 为占位值，填入临时凭证后才可运行，不属于生产部署。
- 根目录 Python 脚本（如 `solver_service.py`、`slider_captcha.py` 等）：仅供本地或实验使用。
- `兑换/` 目录：独立的 FastAPI 兑换示例，非主线，如需使用请单独部署并自管依赖。
- 抓图/批处理脚本（如 `download_product_images.py` 等）：运维辅助，非核心服务。

## 功能特性
### 账号管理
- ✅ 多账号管理（支持同一手机号多用户）
- ✅ Token 自动刷新
- ✅ JWT 获取失败自动重试（3次后自动禁用账号）
- ✅ 账号状态监控

### 自动任务
- ✅ 每日签到
- ✅ 微信打卡
- ✅ 摇一摇
- ✅ 任务列表
- ✅ 备份礼包
- ✅ 邀请好友
- ✅ 今日云朵
- ✅ 消息推送
- ✅ 我的花园
- ✅ 红包任务
- ✅ AI 云盘
- ✅ 云端 battle
- ✅ 盲盒任务
- ✅ 云手机

### 兑换中心
- ✅ 商品自动更新
- ✅ 自定义兑换时间
- ✅ 多账号并发兑换
- ✅ 滑块验证码自动识别
- ✅ 兑换记录管理

### 管理后台
- ✅ 用户管理
- ✅ 账号管理
- ✅ 任务配置
- ✅ 系统公告
- ✅ 运行日志
- ✅ 实时监控

## 技术栈

| 层级 | 技术 | 说明 |
|------|------|------|
| 前端 | Vue 3 + TypeScript + Element Plus | 管理界面 |
| 后端 API | Go + Gin | RESTful API |
| 后端 Worker | Go | 任务调度与执行 |
| 数据库 | MySQL 8.0 | 数据存储 |
| 缓存 | Redis | 任务队列、Token缓存 |
| 识别服务 | Python + OpenCV | 滑块验证码识别 |

## 快速开始

### 环境要求
- Go 1.21+
- Node.js 18+
- MySQL 8.0+
- Redis 6.0+
- Python 3.11+ (可选，用于滑块识别)

### 1. 数据库初始化

```bash
# 创建数据库并导入初始结构
mysql -u root -p < backend/migrations/init.sql
```

### 2. 后端配置

```bash
cd backend

# 复制环境变量配置
cp configs/.env.example .env

# 编辑 .env 文件，配置数据库和Redis连接
vim .env
```

### 3. 启动 API 服务

```bash
cd backend
go run cmd/api/main.go

# 或编译后运行
go build -o caiyun-api cmd/api/main.go
./caiyun-api
```

### 4. 启动 Worker 服务

```bash
cd backend
go run cmd/worker/main.go

# 或编译后运行
go build -o caiyun-worker cmd/worker/main.go
./caiyun-worker
```

### 5. 启动前端

```bash
cd frontend
npm install
npm run dev

# 生产构建
npm run build
```

### 6. 启动滑块识别服务（可选）

```bash
pip install flask opencv-python-headless numpy
python solver_service.py
```

## 部署指南

### Docker 部署

```bash
# 构建镜像
docker-compose build

# 启动服务
docker-compose up -d
```

### 宝塔面板部署

详见 [docs/DEPLOY_BT_PANEL.md](backend/docs/DEPLOY_BT_PANEL.md)

### 手动部署

```bash
# 1. 编译后端（Linux）
cd backend
GOOS=linux GOARCH=amd64 go build -o caiyun-api cmd/api/main.go
GOOS=linux GOARCH=amd64 go build -o caiyun-worker cmd/worker/main.go

# 2. 编译前端
cd frontend
npm install
npm run build

# 3. 上传到服务器
scp -r backend/caiyun-api backend/caiyun-worker backend/migrations root@server:/www/wwwroot/caiyun/
scp -r frontend/dist root@server:/www/wwwroot/caiyun/

# 4. 配置 Nginx（参考 nginx-server.conf）

# 5. 启动服务
nohup ./caiyun-api > logs/api.log 2>&1 &
nohup ./caiyun-worker > logs/worker.log 2>&1 &
```

## 目录结构

```
.
├── backend/                 # Go 后端
│   ├── cmd/
│   │   ├── api/            # API 服务入口
│   │   └── worker/         # Worker 服务入口
│   ├── internal/
│   │   ├── core/           # 核心逻辑（HTTP、Auth、Tasks）
│   │   ├── handlers/       # HTTP 处理器
│   │   ├── services/       # 业务逻辑
│   │   ├── repository/     # 数据访问层
│   │   ├── models/         # 数据模型
│   │   └── middleware/     # 中间件
│   ├── migrations/         # 数据库迁移
│   └── docs/               # 文档
├── frontend/               # Vue.js 前端
│   ├── src/
│   │   ├── views/          # 页面组件
│   │   ├── components/     # 通用组件
│   │   ├── api/            # API 接口
│   │   └── store/          # 状态管理
│   └── dist/               # 构建输出
├── solver_service.py       # 滑块识别服务
└── README.md
```

## API 文档

详见 [docs/api.md](backend/docs/api.md)

## 核心功能说明

### JWT 自动刷新机制

系统会在以下情况自动刷新 JWT Token：
1. Token 即将过期（默认30天有效期）
2. 任务执行前检查
3. 兑换操作前检查

### 任务执行流程

```
1. 从队列获取待执行任务
2. 检查账号 Token 是否有效
3. 获取/刷新 JWT Token（失败3次后禁用账号）
4. 执行各项任务
5. 记录执行日志
6. 更新账号状态
```

### 兑换流程

```
1. 到达设定时间，创建兑换任务
2. 多账号并发执行
3. 获取滑块验证码
4. 调用 Python 服务识别滑块位置
5. 提交兑换请求
6. 记录兑换结果
```

## 注意事项

1. **安全性**: 生产环境务必修改 JWT_SECRET 和数据库密码
2. **并发控制**: 兑换任务建议控制并发数，避免触发风控
3. **Token 有效期**: 账号 Token 有效期约30天，系统会自动刷新
4. **滑块识别**: 识别准确率约85%，失败时会自动重试
5. **日志清理**: 定期清理 task_logs 表，避免数据过大

## 更新日志

### 2025-03-15
- 新增 JWT 获取失败重试机制（3次后自动禁用账号）
- 优化账号添加逻辑（存在则更新，支持同一手机号多用户）
- 修复分页限制问题
- 修复 authMgr nil pointer 问题

## 重要说明与安全提示
- 数据库仅使用 `backend/migrations/init.sql` 初始化/升级；历史 `001-004` SQL 已移除，请勿混用。
- 根目录 `main.go` 的 JWT 为占位符，运行前请填入临时凭证，勿提交真实凭证。
- 部署时仅构建/发布 `backend/cmd/api`、`backend/cmd/worker`、`frontend`，不要打包根目录辅助脚本。
- 滑块识别服务可选，不启用时主系统正常工作。

## 许可证

MIT License


