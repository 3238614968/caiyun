import { expect, test } from '@playwright/test'
import { authenticateAsAdmin, mockBackend } from './helpers/mockApi'

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
