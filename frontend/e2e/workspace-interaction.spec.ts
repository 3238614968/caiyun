import { expect, test } from '@playwright/test'
import path from 'node:path'
import fs from 'node:fs'
import { authenticateAsAdmin, mockBackend } from './helpers/mockApi'

test.beforeEach(async ({ page }) => {
  await mockBackend(page)
  await authenticateAsAdmin(page)
})

test('导航高亮跟随页面，搜索可用 Enter，保留蓝色主题', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  await page.goto('/dashboard')
  await page.getByRole('menuitem', { name: '账号', exact: true }).click()
  await expect(page).toHaveURL(/\/accounts$/)
  await expect(page.locator('.navigation-indicator')).toHaveCSS('opacity', '1')
  await expect.poll(async () => {
    const active = await page.locator('.sidebar-menu .is-active').boundingBox()
    const indicator = await page.locator('.navigation-indicator').boundingBox()
    return active && indicator ? Math.abs(active.y - indicator.y) : 999
  }).toBeLessThan(2)
  await page.getByPlaceholder('请输入手机号', { exact: true }).fill('13900000002')
  const searched = page.waitForRequest(request => request.url().includes('/accounts') && request.url().includes('13900000002'))
  await page.getByPlaceholder('请输入手机号', { exact: true }).press('Enter')
  await searched
  expect(await page.locator('.layout-container').evaluate(el => getComputedStyle(el).getPropertyValue('--el-color-primary').trim())).toBe('#2563eb')
  expect(errors).toEqual([])
})

test('页面进入动画结束后保留用户已聚焦的输入框', async ({ page }) => {
  await page.goto('/dashboard')
  await page.addStyleTag({ content: '.layout-container .page-transition-enter-active { transition-duration: 1s !important; }' })
  await page.getByRole('menuitem', { name: '账号', exact: true }).click()
  const input = page.getByPlaceholder('请输入手机号', { exact: true })
  await input.fill('13900000002')
  await expect(page.locator('.account-manage-container')).not.toHaveClass(/page-transition-enter-active/)
  await expect(input).toBeFocused()
  const searched = page.waitForRequest(request => request.url().includes('/accounts') && request.url().includes('13900000002'))
  await input.press('Enter')
  await searched
})

test('慢请求显示骨架，空筛选可恢复，移动导航不会继承桌面折叠', async ({ page }) => {
  let release!: () => void
  const gate = new Promise<void>(resolve => { release = resolve })
  await page.route('**/api/v1/products/search**', async route => { await gate; await route.fallback() })
  await page.setViewportSize({ width: 390, height: 844 })
  await page.addInitScript(() => localStorage.setItem('caiyun_ui_preferences', JSON.stringify({ isCollapse: true })))
  await page.goto('/exchange')
  await expect(page.locator('.product-skeleton').first()).toBeVisible()
  await expect(page.getByText('暂无商品', { exact: true })).toHaveCount(0)
  release()
  await expect(page.locator('.product-card')).toHaveCount(2)
  await page.getByRole('textbox', { name: '搜索商品名称', exact: true }).fill('不存在的商品')
  await expect(page.getByText('没有找到匹配的商品')).toBeVisible()
  await page.getByRole('button', { name: '清除筛选' }).click()
  await expect(page.locator('.product-card')).toHaveCount(2)
  await expect(page.locator('.sidebar-menu')).not.toHaveClass(/el-menu--collapse/)
  for (const item of await page.locator('.sidebar-menu .el-menu-item').all()) {
    const bounds = await item.boundingBox()
    expect(bounds?.height).toBeGreaterThanOrEqual(44)
    expect((bounds?.y || 0) + (bounds?.height || 0)).toBeLessThanOrEqual(844)
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})

test('减少动态效果覆盖弹窗，Escape 关闭后焦点回到触发按钮', async ({ page }) => {
  await page.goto('/exchange')
  await page.getByRole('button', { name: '打开用户菜单' }).click()
  await page.getByRole('menuitem', { name: '系统设置' }).click()
  await page.getByRole('switch', { name: '减少动态效果' }).press('Space')
  await expect(page.locator('html')).toHaveAttribute('data-reduce-motion', 'true')
  await page.getByRole('button', { name: '完成', exact: true }).click()
  const trigger = page.getByRole('button', { name: '预定：E2E售罄券', exact: true })
  await trigger.click()
  const dialog = page.getByRole('dialog', { name: '创建抢兑任务' })
  await expect(dialog).toBeVisible()
  expect(await dialog.evaluate(el => getComputedStyle(el).animationName)).toBe('none')
  await page.keyboard.press('Escape')
  await expect(dialog).not.toBeVisible()
  await expect(trigger).toBeFocused()
})

test('提交状态可见且双击不会重复创建任务', async ({ page }) => {
  let submitted = 0
  let release!: () => void
  const gate = new Promise<void>(resolve => { release = resolve })
  await page.route('**/api/v1/exchange/tasks', async route => {
    if (route.request().method() === 'POST') { submitted++; await gate }
    await route.fallback()
  })
  await page.goto('/exchange')
  await page.getByRole('button', { name: '预定：E2E售罄券', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '创建抢兑任务' })
  const submit = dialog.getByRole('button', { name: '确定', exact: true })
  await submit.click()
  await expect(submit).toHaveClass(/is-loading/)
  await expect(submit).toBeDisabled()
  await page.keyboard.press('Escape')
  await expect(dialog).toBeVisible()
  expect(submitted).toBe(1)
  release()
  await expect(dialog).not.toBeVisible()
})

test('系统减少动态效果在小屏弹窗中生效，底部操作仍可见', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.setViewportSize({ width: 320, height: 568 })
  await page.goto('/exchange')
  await expect(page.locator('html')).toHaveAttribute('data-reduce-motion', 'true')
  await page.getByRole('button', { name: '预定：E2E售罄券', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '创建抢兑任务' })
  await expect(dialog).toBeVisible()
  await expect(dialog.getByRole('button', { name: '确定', exact: true })).toBeInViewport()
  expect(await dialog.evaluate(el => getComputedStyle(el).animationName)).toBe('none')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})

test('桌面、手机与弹窗视觉检查', async ({ page }) => {
  const artifacts = path.resolve(process.cwd(), '../.local/review/interaction-20261007')
  fs.mkdirSync(artifacts, { recursive: true })
  await page.goto('/exchange')
  await expect(page.locator('.product-card')).toHaveCount(2)
  await page.screenshot({ path: path.join(artifacts, 'exchange-desktop.png'), fullPage: true, animations: 'disabled' })
  await page.getByRole('button', { name: '预定：E2E售罄券', exact: true }).click()
  await expect(page.getByRole('dialog', { name: '创建抢兑任务' })).toBeVisible()
  await page.screenshot({ path: path.join(artifacts, 'dialog-desktop.png'), fullPage: true, animations: 'disabled' })
  await page.keyboard.press('Escape')
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(page.locator('.mobile-page-title')).toBeVisible()
  await page.locator('.main-content').evaluate(el => { el.scrollTop = 0 })
  await page.screenshot({ path: path.join(artifacts, 'exchange-mobile.png'), fullPage: true, animations: 'disabled' })
  await page.getByRole('button', { name: '预定：E2E售罄券', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '创建抢兑任务' })
  await expect(dialog).toBeVisible()
  const bounds = await dialog.boundingBox()
  expect((bounds?.x || 0) + (bounds?.width || 0)).toBeLessThanOrEqual(390)
  expect((bounds?.y || 0) + (bounds?.height || 0)).toBeLessThanOrEqual(844)
  await expect(dialog.getByRole('button', { name: '确定', exact: true })).toBeInViewport()
  await page.screenshot({ path: path.join(artifacts, 'dialog-mobile.png'), fullPage: true, animations: 'disabled' })
})
