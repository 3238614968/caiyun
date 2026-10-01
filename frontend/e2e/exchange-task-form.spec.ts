import { expect, test, type Locator, type Page } from '@playwright/test'
import path from 'node:path'
import fs from 'node:fs'
import { authenticateAsAdmin, mockBackend } from './helpers/mockApi'

test.beforeEach(async ({ page }) => {
  await mockBackend(page)
  await authenticateAsAdmin(page)
})

async function openTaskDialog(page: Page) {
  await page.goto('/exchange')
  await page.getByRole('tab', { name: '抢兑任务' }).click()
  await page.getByRole('button', { name: '新建抢兑任务' }).click()
  return page.getByRole('dialog', { name: '创建抢兑任务' })
}

async function addTime(dialog: Locator, time: string) {
  await dialog.getByLabel('新增抢兑时间').fill(time)
  await dialog.getByLabel('新增抢兑时间').press('Tab')
  await dialog.getByRole('button', { name: '添加时间' }).click()
}

test('简化表单支持多账号、多时间与长期抢兑', async ({ page }) => {
  const dialog = await openTaskDialog(page)
  for (const label of ['自定义 Cron', '日历策略', '节假日覆盖', '调休工作日', '补货周期', '最大次数', '指定抢兑时间']) {
    await expect(dialog.getByText(label, { exact: true })).toHaveCount(0)
  }
  await dialog.getByRole('button', { name: '全选可用规则' }).click()
  await addTime(dialog, '17:51')
  await addTime(dialog, '16:00')
  await addTime(dialog, '10:00')
  await expect(dialog.locator('.selected-times .el-tag')).toHaveCount(3)
  await dialog.locator('.long-term-option .el-switch').click()
  await expect(dialog.getByRole('switch', { name: '长期抢兑' })).toBeChecked()
  const artifacts = path.resolve(process.cwd(), '../.local/review')
  fs.mkdirSync(artifacts, { recursive: true })
  await dialog.screenshot({ path: path.join(artifacts, 'exchange-task-simple-desktop.png') })
  const submitted = page.waitForRequest(request => new URL(request.url()).pathname === '/api/v1/exchange/tasks' && request.method() === 'POST')
  await dialog.getByRole('button', { name: '确定', exact: true }).click()
  const payload = (await submitted).postDataJSON()
  expect(payload.exchange_rule_ids).toEqual([11, 12])
  expect(payload.task_type).toBe('long_term')
  expect(payload.restock_times).toEqual(['10:00:00', '16:00:00', '17:51:00'])
  expect(payload.scheduled_exchange_time).toBeUndefined()
  expect(payload.restock_cycle).toBe('daily')
  expect(payload.calendar_policy).toBe('all')
  for (const key of ['custom_cron', 'restock_weekday', 'restock_day_of_month', 'holiday_dates', 'workday_dates']) {
    expect(payload).not.toHaveProperty(key)
  }
})

test('手机端可增删时间，单次任务支持多个候选时间', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  const dialog = await openTaskDialog(page)
  await expect(dialog.getByRole('switch', { name: '长期抢兑' })).not.toBeChecked()
  await dialog.locator('.selected-times .el-tag__close').click()
  await expect(dialog.getByRole('button', { name: '确定', exact: true })).toBeDisabled()
  await addTime(dialog, '00:00')
  await addTime(dialog, '16:00')
  await expect(dialog.locator('.selected-times .el-tag')).toHaveCount(2)
  await expect(dialog.getByRole('button', { name: '确定', exact: true })).toBeInViewport()
  expect(await dialog.evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true)
  const artifacts = path.resolve(process.cwd(), '../.local/review')
  fs.mkdirSync(artifacts, { recursive: true })
  await dialog.screenshot({ path: path.join(artifacts, 'exchange-task-simple-mobile.png') })
  const submitted = page.waitForRequest(request => new URL(request.url()).pathname === '/api/v1/exchange/tasks' && request.method() === 'POST')
  await dialog.getByRole('button', { name: '确定', exact: true }).click()
  const payload = (await submitted).postDataJSON()
  expect(payload.task_type).toBe('fixed')
  expect(payload.max_attempts).toBe(1)
  expect(payload.restock_times).toEqual(['00:00:00', '16:00:00'])
})
