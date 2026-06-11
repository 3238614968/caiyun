# 前端 E2E 测试

本项目使用 Playwright 覆盖关键浏览器路径，测试位于 `frontend/e2e`。

## 覆盖范围

- 未登录访问受保护路由时跳转到登录页。
- 登录页提交后进入首页仪表盘。
- 管理员登录态下访问账号、日志、兑换、管理等核心模块。
- 账号管理 CRUD。
- 抢兑任务创建。
- 管理员抢兑配置保存和任务配置切换。
- 抢兑历史记录筛选、详情查看、导出触发。
- 管理员用户管理：角色修改、密码重置、删除用户。
- 管理员公告管理：发布、查看、编辑、删除公告。
- 仪表盘队列监控：队列后端、健康状态、关键队列指标。

测试默认通过 Playwright `page.route` Mock `/api/**` 请求，并在浏览器上下文中替换 WebSocket，避免依赖真实后端服务，适合在 CI 中稳定运行。

## 常用命令

```bash
npm run e2e
npm run e2e:ui
npm run e2e:install
```

如需连接已有前端服务，可设置：

```bash
PLAYWRIGHT_SKIP_WEBSERVER=1 PLAYWRIGHT_BASE_URL=http://127.0.0.1:5173 npm run e2e
```

如需调整内置 preview 端口：

```bash
PLAYWRIGHT_PORT=4180 npm run e2e
```
