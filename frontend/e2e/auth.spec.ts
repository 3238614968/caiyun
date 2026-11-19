import { expect, test } from '@playwright/test'
import { adminUser, authenticateAsAdmin, mockBackend } from './helpers/mockApi'

test('未登录访问受保护页面会跳转到登录页', async ({ page }) => {
  await mockBackend(page)

  await page.goto('/dashboard')

  await expect(page).toHaveURL(/\/login(?:\?redirect=.*)?$/)
  await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible()
  await expect(page.getByPlaceholder('请输入用户名')).toBeVisible()
})

test('用户可登录并进入首页仪表盘', async ({ page }) => {
  await mockBackend(page)

  await page.goto('/login')
  await page.getByPlaceholder('请输入用户名').fill('e2e-admin')
  await page.getByPlaceholder('请输入密码').fill('password')
  await page.getByRole('button', { name: '登录' }).click()

  await expect(page).toHaveURL(/\/dashboard$/)
  await expect(page.getByText('移动云盘').first()).toBeVisible()
  await expect(page.getByText('当前云朵数')).toBeVisible()
  await expect(page.getByText('E2E 公告')).toBeVisible()
  await expect(page.getByText('全局账号云朵排名')).toBeVisible()
})

test('退出断线后刷新仍锁定，恢复连接后可确认退出', async ({ page }) => {
  await mockBackend(page)
  await page.goto('/login')
  await page.getByPlaceholder('请输入用户名').fill('e2e-admin')
  await page.getByPlaceholder('请输入密码').fill('password')
  await page.getByRole('button', { name: '登录' }).click()
  await expect(page).toHaveURL(/\/dashboard$/)
  await page.route('**/api/v1/auth/logout', route => route.abort('failed'), { times: 1 })
  await page.getByRole('button', { name: '打开用户菜单' }).click()
  await page.getByRole('menuitem', { name: '退出登录' }).click()
  await page.getByRole('button', { name: '确定', exact: true }).click()
  await expect(page).toHaveURL(/\/login$/)
  const retry = page.getByRole('button', { name: '重试退出' })
  await expect(retry).toBeVisible()
  await expect(page.getByText('已退出登录', { exact: true })).toHaveCount(0)
  let probes = 0
  page.on('request', request => { if (new URL(request.url()).pathname === '/api/v1/auth/me') probes++ })
  await page.reload()
  await expect(retry).toBeVisible()
  await page.goto('/dashboard')
  await expect(page).toHaveURL(/\/login\?redirect=/)
  await expect(retry).toBeVisible()
  expect(probes).toBe(0)
  await retry.click()
  await expect(page.getByText('已退出登录', { exact: true })).toBeVisible()
  await expect(retry).toHaveCount(0)
  await page.goto('/dashboard')
  await expect(page).toHaveURL(/\/login\?redirect=/)
  await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible()
  expect(probes).toBeGreaterThan(0)
})

test('管理员登录状态可访问主要业务模块', async ({ page }) => {
  await mockBackend(page)
  await authenticateAsAdmin(page)

  await page.goto('/dashboard')
  await expect(page.getByText('当前云朵数')).toBeVisible()

  await page.getByRole('menuitem', { name: /账号/ }).click()
  await expect(page).toHaveURL(/\/accounts$/)
  await expect(page.getByText('账号管理')).toBeVisible()

  await page.getByRole('menuitem', { name: /日志/ }).click()
  await expect(page).toHaveURL(/\/logs$/)
  await expect(page.getByText('运行日志')).toBeVisible()

  await page.getByRole('menuitem', { name: /兑换/ }).click()
  await expect(page).toHaveURL(/\/exchange$/)
  await expect(page.getByText('商品中心')).toBeVisible()

  await page.getByRole('menuitem', { name: /管理/ }).click()
  await expect(page).toHaveURL(/\/admin$/)
  await expect(page.getByText('管理员面板')).toBeVisible()
})

test('普通用户不展示队列监控，也不请求管理员接口', async ({ page }) => {
  await mockBackend(page)
  await authenticateAsAdmin(page)
  await page.route('**/api/v1/auth/me', route => route.fulfill({
    json: { code: 0, message: 'success', data: { ...adminUser, username: 'e2e-user', role: 'user' } }
  }))
  const privateRequests: string[] = []
  page.on('request', request => {
    const path = new URL(request.url()).pathname
    if (path.startsWith('/api/v1/admin/') || path.endsWith('/queue-status')) privateRequests.push(path)
  })

  await page.goto('/dashboard')
  await expect(page.getByTestId('cloud-trend-summary')).toContainText('最新云朵 12,345')
  await expect(page.getByText('任务执行状态', { exact: true })).toHaveCount(0)
  await expect(page.getByText('队列后端', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('menuitem', { name: '管理', exact: true })).toHaveCount(0)
  expect(privateRequests).toEqual([])

  await page.goto('/admin')
  await expect(page).toHaveURL(/\/dashboard$/)
  await expect(page.getByText('任务执行状态', { exact: true })).toHaveCount(0)
  expect(privateRequests).toEqual([])
})
