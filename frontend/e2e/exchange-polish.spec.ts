import { expect, test } from '@playwright/test'
import path from 'node:path'
import fs from 'node:fs'
import { authenticateAsAdmin, mockBackend } from './helpers/mockApi'

test.beforeEach(async ({ page }) => {
  await mockBackend(page)
  await authenticateAsAdmin(page)
  await page.route('**/api/v1/exchange/prizes**', async route => {
    const url = new URL(route.request().url())
    const account = Number(url.searchParams.get('account_id'))
    if (url.searchParams.get('refresh') === '1' && account === 1) await new Promise(resolve => setTimeout(resolve, 300))
    const count = account === 1 ? 20 : 1
    const prizes = Array.from({ length: count }, (_, index) => ({
      oId: `${account}-${index}`, prizeName: account === 1 ? `E2E奖品 ${String(index + 1).padStart(2, '0')}` : '账号二待领奖品', prizeId: `${index}`, marketid: 'National_NewLoginGif', marketname: 'National_NewLoginGif', expireTime: '2030-09-30T23:59:59', flag: 1, verifycode: 1, type: 'space'
    }))
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ code: 0, message: 'success', data: { account_id: account, prizes, total: count, record_count: count + 5, fetched_at: '2026-09-30T09:00:00Z' } }) })
  })
})

test('领奖专区显示全部奖品并隔离账号切换后的旧响应', async ({ page }) => {
  await page.goto('/exchange')
  await page.getByRole('tab', { name: '领奖专区' }).click()
  await expect(page.getByText('E2E奖品 01', { exact: true })).toBeVisible()
  await expect(page.locator('.prize-summary')).toContainText('20')
  await page.locator('.prize-pagination .btn-next').click()
  await expect(page.getByText('E2E奖品 20', { exact: true })).toBeVisible()
  await page.getByPlaceholder('搜索奖品或活动').fill('E2E奖品 03')
  await expect(page.locator('.prize-card')).toHaveCount(1)
  await page.getByRole('button', { name: '领取指引' }).click()
  await expect(page.getByRole('dialog', { name: '领取奖品' })).toContainText('此奖品对应的短信验证码')
  await page.getByRole('button', { name: '我知道了' }).click()
  await page.getByPlaceholder('搜索奖品或活动').fill('')
  await page.getByRole('button', { name: '刷新奖品' }).click()
  await page.locator('.prize-account').click()
  await page.getByRole('option', { name: /13900000002/ }).click()
  await expect(page.getByText('账号二待领奖品', { exact: true })).toBeVisible()
  await page.waitForTimeout(400)
  await expect(page.getByText('E2E奖品 01', { exact: true })).toHaveCount(0)
  await expect(page.locator('.prize-error')).toHaveCount(0)
})

test('领奖专区桌面与手机布局无横向溢出', async ({ page }) => {
  const artifacts = path.resolve(process.cwd(), '../.local/review')
  fs.mkdirSync(artifacts, { recursive: true })
  await page.goto('/exchange')
  await page.getByRole('tab', { name: '领奖专区' }).click()
  await expect(page.getByText('E2E奖品 01', { exact: true })).toBeVisible()
  await page.waitForTimeout(150)
  await page.screenshot({ path: path.join(artifacts, 'exchange-rewards-desktop.png'), fullPage: true })
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(page.getByText('E2E奖品 01', { exact: true })).toBeVisible()
  await expect(page.locator('.mobile-page-title')).toBeVisible()
  await expect(page.locator('.sidebar-menu .el-menu-item').last()).toBeInViewport()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await page.screenshot({ path: path.join(artifacts, 'exchange-rewards-mobile.png'), fullPage: true })
})

test('领奖专区展示凭据迁移错误并在恢复后重新加载', async ({ page }) => {
  const message = '账号凭据无法解密，请管理员核对数据加密密钥并迁移旧版密文后重试'
  let broken = true
  await page.route('**/api/v1/exchange/prizes**', async route => {
    if (broken) {
      await route.fulfill({ status: 409, contentType: 'application/json', body: JSON.stringify({ code: 409, message }) })
    } else {
      await route.fallback()
    }
  })
  await page.goto('/exchange')
  await page.getByRole('tab', { name: '领奖专区' }).click()
  await expect(page.locator('.prize-error')).toContainText(message)
  await expect(page.locator('.prize-card')).toHaveCount(0)
  broken = false
  await page.getByRole('button', { name: '刷新奖品' }).click()
  await expect(page.getByText('E2E奖品 01', { exact: true })).toBeVisible()
  await expect(page.locator('.prize-error')).toHaveCount(0)
})

test('预定任务修改指定时间后不会被旧补货时间覆盖', async ({ page }) => {
  await page.goto('/exchange')
  await page.locator('.product-card').filter({ hasText: 'E2E售罄券' }).getByRole('button', { name: '预定' }).click()
  const dialog = page.getByRole('dialog', { name: '创建抢兑任务' })
  await dialog.getByPlaceholder('选择抢兑时间').fill('17:51')
  await dialog.getByPlaceholder('选择抢兑时间').press('Tab')
  const submitted = page.waitForRequest(request => new URL(request.url()).pathname === '/api/v1/exchange/tasks' && request.method() === 'POST')
  await dialog.getByRole('button', { name: '确定', exact: true }).click()
  const payload = (await submitted).postDataJSON()
  expect(payload.scheduled_exchange_time).toBe('17:51:00')
  expect(payload.restock_times).toEqual(['17:51:00'])
})

test('管理员首页不会混入个人趋势并保持缺失日期的断点', async ({ page }) => {
  let personalLoads = 0
  page.on('request', request => { if (new URL(request.url()).pathname === '/api/v1/stats/dashboard') personalLoads++ })
  await page.route('**/api/v1/stats/trend**', route => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ code: 0, message: 'success', data: { trend_data: [
    { date: '2026-09-28', cloud_count: 12000, cloud_diff: 0, has_data: true, comparable: false, sampled_accounts: 3, account_count: 3 },
    { date: '2026-09-29', cloud_count: 2000, cloud_diff: 0, has_data: false, comparable: false, sampled_accounts: 1, account_count: 3 },
    { date: '2026-09-30', cloud_count: 12345, cloud_diff: 0, has_data: true, comparable: false, sampled_accounts: 3, account_count: 3 }
  ] } }) }))
  await page.goto('/dashboard')
  await expect(page.getByTestId('cloud-trend-summary')).toContainText('12,345')
  await expect(page.locator('.trend-note')).toContainText('保留断点')
  await expect(page.locator('.point-hit')).toHaveCount(2)
  expect(personalLoads).toBe(0)
})
