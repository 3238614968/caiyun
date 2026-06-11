import { expect, test } from '@playwright/test'
import { authenticateAsAdmin, mockBackend } from './helpers/mockApi'

test.beforeEach(async ({ page }) => {
  await mockBackend(page)
  await authenticateAsAdmin(page)
})

test('兑换记录支持筛选、查看详情和导出当前结果', async ({ page }) => {
  await page.goto('/exchange/records')

  await expect(page.getByText('抢兑历史记录')).toBeVisible()
  await expect(page.getByText('E2E月卡').first()).toBeVisible()
  await expect(page.getByText('兑换成功').first()).toBeVisible()

  await page.getByPlaceholder('商品名称').fill('E2E月卡')
  await page.getByRole('button', { name: '查询' }).click()
  await expect(page.getByRole('table').filter({ hasText: 'E2E月卡' })).toBeVisible()

  await page.getByRole('button', { name: '详情' }).first().click()
  const detailDialog = page.getByRole('dialog', { name: '抢兑详情' })
  await expect(detailDialog).toBeVisible()
  await expect(detailDialog.getByText('E2E月卡')).toBeVisible()
  await detailDialog.press('Escape')

  await page.getByRole('button', { name: '导出' }).first().click()
  await expect(page.getByText('导出成功')).toBeVisible()
})

test('管理员用户管理支持改角色、重置密码和删除用户', async ({ page }) => {
  await page.goto('/admin')

  await page.getByRole('tab', { name: '用户管理' }).click()
  await expect(page.getByText('e2e-user')).toBeVisible()

  const userRow = page.getByRole('row', { name: /e2e-user/ })
  await userRow.getByRole('button', { name: '修改角色' }).click()
  const roleDialog = page.getByRole('dialog', { name: '修改用户角色' })
  await roleDialog.getByText('管理员').click()
  await roleDialog.getByRole('button', { name: '确定' }).click()
  await expect(page.getByText('角色修改成功')).toBeVisible()
  await expect(page.getByRole('row', { name: /e2e-user/ }).getByText('管理员')).toBeVisible()

  await page.getByRole('row', { name: /e2e-user/ }).getByRole('button', { name: '重置密码' }).click()
  const passwordDialog = page.getByRole('dialog', { name: '重置用户密码' })
  await passwordDialog.getByPlaceholder('至少12位，包含至少三类字符').fill('E2eStrongPass123!')
  await passwordDialog.getByRole('button', { name: '确定重置' }).click()
  await expect(page.getByText('密码重置成功')).toBeVisible()

  await page.getByRole('row', { name: /e2e-user/ }).getByRole('button', { name: '删除' }).click()
  await page.getByRole('dialog', { name: '提示' }).getByRole('button', { name: '确定' }).click()
  await expect(page.getByText('删除成功')).toBeVisible()
  await expect(page.getByText('e2e-user')).toBeHidden()
})

test('管理员公告管理支持发布、编辑、查看和删除公告', async ({ page }) => {
  await page.goto('/admin')

  await page.getByRole('tab', { name: '公告管理' }).click()
  await expect(page.getByText('E2E 公告')).toBeVisible()

  await page.getByRole('button', { name: '发布公告' }).click()
  const createDialog = page.getByRole('dialog', { name: '发布公告' })
  await createDialog.getByPlaceholder('请输入公告标题').fill('E2E新增公告')
  await createDialog.getByPlaceholder('请输入公告内容').fill('E2E 新增公告内容')
  await createDialog.getByText('置顶').click()
  await createDialog.getByRole('button', { name: '确定' }).click()
  await expect(page.getByText('发布成功')).toBeVisible()
  await expect(page.getByText('E2E新增公告')).toBeVisible()

  const createdRow = page.getByRole('row', { name: /E2E新增公告/ })
  await createdRow.getByRole('button', { name: '查看' }).click()
  const viewDialog = page.getByRole('dialog', { name: '公告详情' })
  await expect(viewDialog.getByText('E2E 新增公告内容')).toBeVisible()
  await viewDialog.press('Escape')

  await createdRow.getByRole('button', { name: '编辑' }).click()
  const editDialog = page.getByRole('dialog', { name: '编辑公告' })
  await editDialog.getByPlaceholder('请输入公告标题').fill('E2E编辑公告')
  await editDialog.getByRole('button', { name: '确定' }).click()
  await expect(page.getByText('更新成功')).toBeVisible()
  await expect(page.getByText('E2E编辑公告')).toBeVisible()

  await page.getByRole('row', { name: /E2E编辑公告/ }).getByRole('button', { name: '删除' }).click()
  await page.getByRole('dialog', { name: '确认删除' }).getByRole('button', { name: '确定' }).click()
  await expect(page.getByText('删除成功')).toBeVisible()
  await expect(page.getByText('E2E编辑公告')).toBeHidden()
})

test('仪表盘展示队列后端、健康状态和关键队列指标', async ({ page }) => {
  await page.goto('/dashboard')

  await expect(page.getByText('任务执行状态')).toBeVisible()
  await expect(page.getByText('队列后端')).toBeVisible()
  await expect(page.getByText('streams')).toBeVisible()
  await expect(page.getByText('健康', { exact: true })).toBeVisible()
  await expect(page.getByText('队列长度')).toBeVisible()
  await expect(page.getByText('死信')).toBeVisible()
})
