import { expect, test } from '@playwright/test'

const adminUsername = process.env.E2E_ADMIN_USERNAME || 'e2e-admin'
const adminPassword = process.env.E2E_ADMIN_PASSWORD || 'E2eAdminPass123!'
const apiHealthURL = process.env.API_HEALTH_URL || 'http://backend-api:8080/health'
const workerHealthURL = process.env.WORKER_HEALTH_URL || 'http://backend-worker:8081/health'

test('真实 Compose 服务健康检查通过', async ({ request }) => {
  const [apiHealth, workerHealth] = await Promise.all([
    request.get(apiHealthURL),
    request.get(workerHealthURL)
  ])

  expect(apiHealth.ok()).toBe(true)
  expect(workerHealth.ok()).toBe(true)
})

test('真实前后端链路可登录并访问核心页面', async ({ page }) => {
  await page.goto('/dashboard')
  await expect(page).toHaveURL(/\/login(?:\?redirect=.*)?$/)
  await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible()

  await page.getByPlaceholder('请输入用户名').fill(adminUsername)
  await page.getByPlaceholder('请输入密码').fill(adminPassword)
  await page.getByRole('button', { name: '登录' }).click()

  await expect(page).toHaveURL(/\/(?:dashboard)?$/)
  await expect(page.getByText('移动云盘').first()).toBeVisible()
  await expect(page.getByText('当前云朵数')).toBeVisible()

  await page.goto('/accounts')
  await expect(page.getByText('账号管理')).toBeVisible()
  await expect(page.getByText('E2E 主账号')).toBeVisible()

  await page.goto('/logs')
  await expect(page.getByText('运行日志')).toBeVisible()
  const logsTable = page.locator('.task-logs-container .el-table')
  await expect(logsTable.getByText('13900009001', { exact: true })).toBeVisible()
  await expect(logsTable.getByText('+6', { exact: true })).toBeVisible()

  await page.goto('/exchange')
  await expect(page.getByText('商品中心')).toBeVisible()
  await expect(page.getByText('E2E 视频会员')).toBeVisible()

  await page.goto('/admin')
  await expect(page.getByText('管理员面板')).toBeVisible()
})
