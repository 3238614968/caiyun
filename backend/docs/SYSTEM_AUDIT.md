# 系统整体审计报告（2026-03-11）

## 审计范围
- 后端（Go API / Worker）
- 前端（Vue3 + Vite）
- 构建与发布流程
- 依赖与基础安全项

## 已执行检查
1. 后端单元/编译检查：`cd backend && go test ./...`
2. 后端静态检查：`cd backend && go vet ./...`
3. 前端类型检查：`cd frontend && npm run typecheck`
4. 前端代码规范检查：`cd frontend && npm run lint`
5. 前端依赖漏洞检查：`cd frontend && npm audit --audit-level=moderate --json`
6. 发布包构建校验：`bash scripts/build_release.sh`

## 主要发现

### P1（高优先级）
1. **前端依赖存在高危漏洞**
   - `axios` 命中 DoS 漏洞（GHSA-43fc-jf86-j433）。
   - `rollup` 命中路径遍历任意写文件风险（GHSA-mw96-cpmx-2vgc）。
   - 建议：尽快升级到无漏洞版本并做一次回归测试。

2. **WebSocket Origin 校验过宽**
   - 目前 `CheckOrigin` 直接 `return true`，生产环境存在跨站连接风险。
   - 建议：基于白名单域名做严格校验，与 HTTP CORS 策略保持一致。

### P2（中优先级）
1. **前端质量门禁未通过**
   - `typecheck` 出现多处 TS 类型不匹配（如 `label` 缺失、类型约束不一致）。
   - `lint` 出现大量 warning 和 error（本次扫描统计 22 error / 689 warning）。
   - 建议：
     - 先修复所有 error（阻断 CI）；
     - 按模块逐步清理 warning（可分批完成）。

2. **代码仓库存在历史备份污染（已本次清理）**
   - 多个 `*.go.<数字>` 备份文件被纳入版本控制，增加审计与维护成本。
   - 本次已删除并在 `.gitignore` 增加忽略规则。

### P3（低优先级）
1. **配置热加载实现存在互斥锁拷贝风险（已本次修复）**
   - 原 `Reload` 采用结构体整体赋值，触发 `go vet` 的“copy lock value”问题。
   - 本次改为字段级拷贝，保留原互斥锁实例。

## 本次已完成修复
1. 修复 `Config.Reload` 的锁拷贝问题。
2. 删除仓库中所有已跟踪的 `*.go.<数字>` 备份文件。
3. 在 `.gitignore` 中新增 `*.go.[0-9]*`，防止同类文件再次进入版本控制。

## 后续建议（按优先级）
1. **立即**：升级前端高危依赖（`axios`、`rollup`/`vite` 链路）。
2. **立即**：收紧 WebSocket `CheckOrigin` 白名单策略。
3. **本周内**：完成前端 `typecheck` 错误清零。
4. **两周内**：建立 CI 质量门禁（`go test`、`go vet`、`npm run typecheck`、`npm run lint`）。
5. **持续**：引入依赖漏洞周期扫描（如每周执行 `npm audit` / `govulncheck`）。
