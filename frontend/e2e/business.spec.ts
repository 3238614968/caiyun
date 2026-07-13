import { expect, test } from '@playwright/test'
import { authenticateAsAdmin, mockBackend } from './helpers/mockApi'

test.beforeEach(async ({ page }) => {
  await mockBackend(page)
  await authenticateAsAdmin(page)
})

test('账号管理支持创建、编辑和删除账号', async ({ page }) => {
  await page.goto('/accounts')

  await expect(page.getByText('账号管理')).toBeVisible()
  await expect(page.getByText('主账号')).toBeVisible()

  await page.getByRole('button', { name: '添加账号' }).click()
  const addDialog = page.getByRole('dialog', { name: '添加账号' })
  await addDialog.getByPlaceholder('请输入手机号').fill('13900009999')
  await addDialog.getByPlaceholder('请输入Auth').fill('AUTH=e2e-created')
  await addDialog.getByPlaceholder('请输入备注').first().fill('E2E新增账号')
  await addDialog.getByRole('button', { name: '确定' }).click()

  await expect(page.getByText('E2E新增账号')).toBeVisible()

  const createdRow = page.getByRole('row', { name: /13900009999/ })
  await createdRow.getByRole('button', { name: '编辑' }).click()
  const editDialog = page.getByRole('dialog', { name: '编辑账号' })
  await editDialog.getByPlaceholder('请输入备注').fill('E2E编辑账号')
  await editDialog.getByRole('button', { name: '确定' }).click()

  await expect(page.getByText('E2E编辑账号')).toBeVisible()

  const editedRow = page.getByRole('row', { name: /E2E编辑账号/ })
  await editedRow.getByRole('button', { name: '删除' }).click()
  await page.getByRole('dialog', { name: '提示' }).getByRole('button', { name: '确定' }).click()

  await expect(page.getByText('E2E编辑账号')).toBeHidden()
})

test('兑换中心可批量选择账号创建抢兑任务', async ({ page }) => {
  await page.goto('/exchange')

  await expect(page.getByText('商品中心')).toBeVisible()
  await expect(page.getByText('E2E月卡')).toBeVisible()

  const productCard = page.locator('.product-card').filter({ hasText: 'E2E月卡' })
  await productCard.getByRole('button', { name: '立即抢兑' }).click()

  const taskDialog = page.getByRole('dialog', { name: '创建抢兑任务' })
  await expect(taskDialog.getByText('E2E月卡', { exact: true })).toBeVisible()
  await taskDialog.getByRole('button', { name: '全选可用规则' }).click()
  await expect(taskDialog.getByText('已选择 2 个抢兑规则')).toBeVisible()
  await taskDialog.getByRole('button', { name: '确定' }).click()

  await page.getByRole('tab', { name: '抢兑任务' }).click()
  const table = page.getByRole('table')
  await expect(table.filter({ hasText: 'E2E月卡' })).toBeVisible()
  await expect(table.filter({ hasText: '抢兑主账号' })).toBeVisible()
  await expect(table.filter({ hasText: '抢兑备用账号' })).toBeVisible()
})

test('兑换中心预定商品支持指定抢兑时间和补货周期', async ({ page }) => {
  await page.goto('/exchange')

  const soldOutCard = page.locator('.product-card').filter({ hasText: 'E2E售罄券' })
  await expect(soldOutCard).toBeVisible()
  await soldOutCard.getByRole('button', { name: '预定' }).click()

  const taskDialog = page.getByRole('dialog', { name: '创建抢兑任务' })
  await expect(taskDialog.getByText('E2E售罄券', { exact: true })).toBeVisible()
  await expect(taskDialog.getByText('指定抢兑时间')).toBeVisible()
  await expect(taskDialog.getByText('补货周期')).toBeVisible()
  await expect(taskDialog.getByText('每周')).toBeVisible()
  await expect(taskDialog.getByText('周五')).toBeVisible()
  await taskDialog.getByRole('button', { name: '确定' }).click()

  await page.getByRole('tab', { name: '抢兑任务' }).click()
  await expect(page.getByRole('table').filter({ hasText: 'E2E售罄券' })).toBeVisible()
  await expect(page.getByRole('table').filter({ hasText: '10:30 · 每周 周五' })).toBeVisible()
})

test('管理员可保存抢兑配置并切换任务配置', async ({ page }) => {
  await page.goto('/admin')

  await expect(page.getByText('管理员面板')).toBeVisible()

  await page.getByRole('tab', { name: '抢兑配置' }).click()
  await expect(page.getByText('抢兑并发数')).toBeVisible()

  await page.locator('.el-input-number input').first().fill('7')
  await page.getByRole('button', { name: '保存配置' }).first().click()
  await expect(page.getByText('保存配置成功')).toBeVisible()

  await page.getByRole('tab', { name: '任务管理' }).click()
  await expect(page.getByText('每日签到')).toBeVisible()
  await page.locator('.task-status-cell .el-switch').first().click()
  await expect(page.getByText('任务已下架')).toBeVisible()
})
