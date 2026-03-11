# 宝塔面板部署指南

本文档详细介绍如何使用宝塔面板部署移动云盘管理系统。

## 📋 系统要求

- CentOS 7.x+ / Ubuntu 20.04+
- 内存：≥ 2GB
- 硬盘：≥ 10GB
- 宝塔面板：≥ 8.0

## 🚀 安装步骤

### 1. 安装宝塔面板

```bash
# CentOS/RedHat
yum install -y wget && wget -O install.sh http://download.bt.cn/install/install_6.0.sh && sh install.sh ed84833

# Ubuntu/Debian
wget -O install.sh http://download.bt.cn/install/install-ubuntu_6.0.sh && sudo bash install.sh ed84833
```

安装完成后，登录宝塔面板（http://你的服务器 IP:8888）。

### 2. 安装运行环境

在宝塔面板中安装以下软件：

#### 必需组件
- **Nginx** 1.20+ (极速模式)
- **MySQL** 8.0 (或 5.7)
- **PHP** 7.4+ (可选，仅管理数据库用)
- **Redis** 7.0
- **Supervisor** (进程守护)

#### 安装方法
1. 登录宝塔面板
2. 进入【软件商店】
3. 搜索并安装上述组件
4. 选择"快速安装"即可

### 3. 上传项目代码

#### 方法一：使用 Git（推荐）

```bash
# 创建网站目录
mkdir -p /www/wwwroot/caiyun

# 进入目录
cd /www/wwwroot/caiyun

# 克隆项目
git clone <你的仓库地址> .

# 或者上传 zip 包解压
```

#### 方法二：直接上传

1. 在本地打包项目
2. 通过宝塔面板【文件】->【上传】
3. 上传到 `/www/wwwroot/caiyun`
4. 解压压缩包

### 4. 配置数据库

#### 4.1 创建数据库

在宝塔面板中：
1. 进入【数据库】
2. 点击【添加数据库】
3. 填写信息：
   - 数据库名：`caiyun`
   - 用户名：`caiyun_user`
   - 密码：生成强密码（保存好）
   - 权限：所有人

#### 4.2 导入数据

```bash
# 进入 SQL 目录
cd /www/wwwroot/caiyun/backend/scripts

# 执行 SQL 脚本
mysql -u caiyun_user -p caiyun < init_caiyun_database.sql
```

或在宝塔面板中：
1. 进入【数据库】
2. 点击 `caiyun` 对应的【管理】
3. 导入 `init_caiyun_database.sql`

### 5. 编译后端程序

```bash
# 安装 Go（如果未安装）
wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
echo "export PATH=\$PATH:/usr/local/go/bin" >> ~/.bashrc
source ~/.bashrc

# 进入后端目录
cd /www/wwwroot/caiyun/backend

# 下载依赖
go mod download

# 编译 API 服务器
go build -o caiyun-api cmd/api/main.go

# 编译任务执行器
go build -o caiyun-worker cmd/worker/main.go
```

### 6. 配置环境变量

```bash
# 复制配置文件
cp /www/wwwroot/caiyun/backend/configs/.env.example /www/wwwroot/caiyun/backend/configs/.env

# 编辑配置
vi /www/wwwroot/caiyun/backend/configs/.env
```

修改以下配置：

```env
PORT=8080
JWT_SECRET=your-random-secret-key-here-change-in-production
DB_HOST=127.0.0.1
DB_PORT=3306
DB_USER=caiyun_user
DB_PASSWORD=你的数据库密码
DB_NAME=caiyun
REDIS_HOST=127.0.0.1
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0
TASK_CONCURRENCY=10
TASK_SCHEDULE=0 8 * * *
EXCHANGE_CONCURRENCY=10
EXCHANGE_SCHEDULE_TIME_1=10:00
EXCHANGE_SCHEDULE_TIME_2=16:00
```

### 7. 配置 Supervisor 进程守护

#### 7.1 创建 API 服务配置

在宝塔面板中：
1. 进入【软件商店】->【Supervisor】
2. 点击【添加守护进程】
3. 填写信息：
   - 名称：`caiyun-api`
   - 运行目录：`/www/wwwroot/caiyun/backend`
   - 启动命令：`/www/wwwroot/caiyun/backend/caiyun-api`
   - 用户：`www`
   - 实例数：`1`

#### 7.2 创建任务执行器配置

重复上述步骤，添加第二个守护进程：
- 名称：`caiyun-worker`
- 运行目录：`/www/wwwroot/caiyun/backend`
- 启动命令：`/www/wwwroot/caiyun/backend/caiyun-worker`
- 用户：`www`
- 实例数：`1`

#### 7.3 手动配置（可选）

也可以通过配置文件：

```bash
# API 服务
cat > /etc/supervisor/conf.d/caiyun-api.conf << EOF
[program:caiyun-api]
command=/www/wwwroot/caiyun/backend/caiyun-api
directory=/www/wwwroot/caiyun/backend
user=www
autostart=true
autorestart=true
redirect_stderr=true
stdout_logfile=/var/log/supervisor/caiyun-api.out.log
stderr_logfile=/var/log/supervisor/caiyun-api.err.log
EOF

# 任务执行器
cat > /etc/supervisor/conf.d/caiyun-worker.conf << EOF
[program:caiyun-worker]
command=/www/wwwroot/caiyun/backend/caiyun-worker
directory=/www/wwwroot/caiyun/backend
user=www
autostart=true
autorestart=true
redirect_stderr=true
stdout_logfile=/var/log/supervisor/caiyun-worker.out.log
stderr_logfile=/var/log/supervisor/caiyun-worker.err.log
EOF

# 重新加载配置
supervisorctl reread
supervisorctl update
supervisorctl start all
```

### 8. 配置 Nginx 反向代理

#### 8.1 添加网站

在宝塔面板中：
1. 进入【网站】->【添加站点】
2. 填写信息：
   - 域名：`caiyun.yourdomain.com`（或使用 IP）
   - 根目录：`/www/wwwroot/caiyun/frontend/dist`
   - PHP 版本：纯静态，选择 纯静态
   - 数据库：无需创建

#### 8.2 配置反向代理

1. 进入刚创建的网站【设置】
2. 选择【反向代理】
3. 添加反向代理：
   - 代理名称：`api`
   - 目标 URL：`http://127.0.0.1:8080`
   - 发送域名：`$host`
   - 代理目录：`/api`

#### 8.3 修改 Nginx 配置

在网站设置中，点击【配置文件】，修改为：

```nginx
server {
    listen 80;
    server_name caiyun.yourdomain.com;
    
    root /www/wwwroot/caiyun/frontend/dist;
    index index.html;
    
    # API 反向代理
    location /api {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # WebSocket 支持
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
    
    # 前端静态文件
    location / {
        try_files $uri $uri/ /index.html;
    }
    
    # 缓存静态资源
    location ~* \.(jpg|jpeg|png|gif|ico|css|js|svg|woff|woff2|ttf|eot)$ {
        expires 1y;
        add_header Cache-Control "public, immutable";
    }
    
    # Gzip 压缩
    gzip on;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml application/xml+rss text/javascript;
    gzip_min_length 1000;
}
```

### 9. 配置防火墙

#### 宝塔面板配置

1. 进入【安全】
2. 放行端口：
   - `80` (HTTP)
   - `443` (HTTPS，如果需要)
   - `8888` (宝塔面板，可选)

#### 云服务器安全组

如果使用云服务器（阿里云、腾讯云等），还需在控制台配置安全组：
- 添加入站规则
- 放行 80、443 端口

### 10. 验证部署

#### 10.1 检查服务状态

```bash
# 检查 Supervisor 进程
supervisorctl status

# 应该看到：
# caiyun-api                    RUNNING
# caiyun-worker                 RUNNING
```

#### 10.2 查看日志

```bash
# API 日志
tail -f /var/log/supervisor/caiyun-api.out.log

# Worker 日志
tail -f /var/log/supervisor/caiyun-worker.out.log
```

#### 10.3 访问测试

浏览器访问：`http://你的服务器 IP` 或 `http://caiyun.yourdomain.com`

默认管理员账号：
- 用户名：`admin`
- 密码：`admin123`

⚠️ **首次登录后请立即修改密码！**

## 🔧 常见问题

### 问题 1：Supervisor 启动失败

**解决方案**：
```bash
# 查看详细错误
supervisorctl tail caiyun-api stderr

# 检查二进制文件是否有执行权限
chmod +x /www/wwwroot/caiyun/backend/caiyun-api
chmod +x /www/wwwroot/caiyun/backend/caiyun-worker

# 重启服务
supervisorctl restart all
```

### 问题 2：数据库连接失败

**解决方案**：
1. 检查 MySQL 是否运行
2. 验证 `.env` 文件中的数据库配置
3. 确认数据库用户权限
4. 检查防火墙是否阻止 3306 端口

### 问题 3：Redis 连接失败

**解决方案**：
```bash
# 检查 Redis 状态
systemctl status redis

# 启动 Redis
systemctl start redis

# 测试连接
redis-cli ping
# 应返回：PONG
```

### 问题 4：Nginx 反向代理不工作

**解决方案**：
1. 检查 Nginx 配置是否正确
2. 测试配置：`nginx -t`
3. 重载 Nginx：`systemctl reload nginx`
4. 检查后端服务是否运行

### 问题 5：前端页面空白

**解决方案**：
1. 打开浏览器开发者工具查看错误
2. 检查 API 地址配置
3. 确认跨域设置
4. 清除浏览器缓存

## 🔒 安全加固建议

### 1. 修改默认密码

首次登录后立即修改管理员密码。

### 2. 配置 HTTPS

```bash
# 在宝塔面板中
1. 进入【网站】
2. 点击对应网站的【设置】
3. 选择【SSL】
4. 申请免费 Let's Encrypt 证书
```

### 3. 限制数据库访问

```sql
-- 只允许本地访问
GRANT ALL PRIVILEGES ON caiyun.* TO 'caiyun_user'@'localhost' IDENTIFIED BY 'password';
FLUSH PRIVIVLEGES;
```

### 4. 定期备份

使用宝塔面板的自动备份功能：
- 数据库备份（每天）
- 网站文件备份（每周）

### 5. 更新系统

```bash
# 定期更新系统包
yum update -y  # CentOS
apt update && apt upgrade -y  # Ubuntu
```

## 📊 监控与维护

### 查看实时日志

```bash
# API 日志
tail -f /var/log/supervisor/caiyun-api.out.log

# Worker 日志
tail -f /var/log/supervisor/caiyun-worker.out.log

# Nginx 日志
tail -f /var/log/nginx/access.log
tail -f /var/log/nginx/error.log
```

### 性能监控

使用宝塔面板的【监控】功能：
- CPU 使用率
- 内存使用情况
- 磁盘空间
- 网络流量

### 日志清理

```bash
# 清理旧日志（保留最近 7 天）
find /var/log/supervisor -name "*.log" -mtime +7 -delete
```

## 🎯 优化建议

### 1. 调整并发数

根据服务器性能调整：

```env
# 小内存服务器（2GB）
TASK_CONCURRENCY=5
EXCHANGE_CONCURRENCY=5

# 中等服务器（4GB）
TASK_CONCURRENCY=10
EXCHANGE_CONCURRENCY=10

# 高配服务器（8GB+）
TASK_CONCURRENCY=20
EXCHANGE_CONCURRENCY=20
```

### 2. 启用 Redis 持久化

编辑 Redis 配置：
```bash
vi /etc/redis.conf

# 启用 RDB 持久化
save 900 1
save 300 10
save 60 10000
```

### 3. 配置 MySQL 优化

使用宝塔面板的 MySQL 优化建议，根据内存选择合适的配置方案。

## 📞 技术支持

如遇到问题，请提供以下信息：

1. 服务器系统版本
2. 宝塔面板版本
3. 错误日志内容
4. 问题复现步骤

---

**祝您部署顺利！** 🎉
