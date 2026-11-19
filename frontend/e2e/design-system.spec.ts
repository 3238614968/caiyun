import { expect, test } from '@playwright/test'
import { authenticateAsAdmin, mockBackend } from './helpers/mockApi'

test.beforeEach(async ({ page }) => {
  await mockBackend(page)
  await authenticateAsAdmin(page)
})

test('快捷导航支持键盘搜索，并能直接打开新增账号', async ({ page }) => {
  await page.goto('/dashboard')
  await expect(page.getByTestId('cloud-trend-summary')).toContainText('最新云朵 12,345')
  await page.keyboard.press('Control+k')
  const search = page.getByRole('combobox', { name: '搜索页面、功能与操作' })
  await expect(search).toBeFocused()
  await search.fill('账号管理')
  await search.press('Enter')
  await expect(page.getByRole('heading', { name: '账号管理' })).toBeVisible()
  await page.keyboard.press('Control+k')
  await search.fill('添加账号')
  await search.press('Enter')
  await expect(page.getByRole('dialog', { name: '添加账号' })).toBeVisible()
  await expect(page.getByRole('dialog').getByPlaceholder('请输入手机号')).toHaveValue('')
})

test('批量选择提交真实账号任务，并展示后台状态', async ({ page }) => {
  await page.goto('/accounts')
  await page.locator('.el-table__body .el-table__row').first().locator('.el-checkbox').click()
  const request = page.waitForRequest(req => req.method() === 'POST' && /\/accounts\/1\/trigger$/.test(req.url()))
  await page.getByRole('button', { name: '执行所选任务' }).click()
  await request
  await expect(page.getByText('1 个账号的任务已加入队列')).toBeVisible()
  await expect(page.getByLabel('后台任务状态')).toBeVisible()
})

test('快捷导航打开管理员面板内的公告管理，支持页面刷新', async ({ page }) => {
  await page.goto('/dashboard')
  await page.getByRole('button', { name: '打开快捷导航' }).click()
  const search = page.getByRole('combobox', { name: '搜索页面、功能与操作' })
  await search.fill('公告管理')
  await search.press('Enter')
  await expect(page).toHaveURL(/\/admin\?tab=announcements$/)
  await expect(page.getByRole('tab', { name: '公告管理' })).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByRole('button', { name: '发布公告' })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('tab', { name: '公告管理' })).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByRole('row', { name: /E2E 公告/ })).toBeVisible()
})

test('切换到失败日志会退出收益过滤，避免筛选结果被隐藏', async ({ page }) => {
  await page.goto('/logs')
  await page.getByRole('button', { name: '任务明细', exact: true }).click()
  const rewards = page.getByRole('switch', { name: '仅看确认收益' })
  await page.getByText('仅看确认收益', { exact: true }).click()
  await expect(rewards).toBeChecked()
  const request = page.waitForRequest(req => {
    const url = new URL(req.url())
    return url.pathname === '/api/v1/tasks/logs' && url.searchParams.get('status') === 'failed' && !url.searchParams.has('has_reward')
  })
  await page.getByRole('group', { name: '日志状态' }).getByRole('button', { name: '失败', exact: true }).click()
  await request
  await expect(rewards).not.toBeChecked()
})

test('日志筛选栏在窄窗口完整显示状态，操作按钮不会重叠', async ({ page }) => {
  await page.goto('/logs')
  await page.getByRole('button', { name: '任务明细', exact: true }).click()
  const status = page.getByRole('group', { name: '日志状态' })
  const running = status.getByRole('button', { name: '待确认', exact: true })
  const search = page.getByRole('button', { name: '搜索', exact: true })
  for (const width of [1440, 1100, 820, 390, 320]) {
    await page.setViewportSize({ width, height: 900 })
    await expect(running).toBeVisible()
    const group = await status.boundingBox()
    const button = await running.boundingBox()
    const action = await search.boundingBox()
    expect(group).not.toBeNull()
    expect(button).not.toBeNull()
    expect(action).not.toBeNull()
    expect(button!.x).toBeGreaterThanOrEqual(group!.x)
    expect(button!.x + button!.width).toBeLessThanOrEqual(group!.x + group!.width + 1)
    const overlapWidth = Math.min(button!.x + button!.width, action!.x + action!.width) - Math.max(button!.x, action!.x)
    const overlapHeight = Math.min(button!.y + button!.height, action!.y + action!.height) - Math.max(button!.y, action!.y)
    expect(overlapWidth <= 0 || overlapHeight <= 0).toBe(true)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  }
})

test('移动端账号以卡片展示，页面没有横向溢出', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/accounts')
  await expect(page.locator('.account-mobile-card')).toHaveCount(2)
  await expect(page.locator('.account-mobile-card').first().getByRole('button', { name: '执行任务', exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
})
