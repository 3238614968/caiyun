import { expect, test } from '@playwright/test'
import path from 'node:path'
import { authenticateAsAdmin, mockBackend } from './helpers/mockApi'

test.beforeEach(async ({ page }) => {
  await mockBackend(page)
  await authenticateAsAdmin(page)
})

test('默认按今天和账号汇总收益，失败和待确认可以展开查询', async ({ page }) => {
  await page.route('**/api/v1/tasks/daily-summary**', route => {
    const date = new URL(route.request().url()).searchParams.get('date')!
    return route.fulfill({ json: { code: 0, message: 'success', data: { date, accounts: [
      { account_id: 1, phone: '13900007646', remark: '主账号', cloud_gained: 11, migu_notes_gained: 40, success_count: 12, failed_count: 1, pending_count: 1, skipped_count: 0, last_executed_at: `${date}T08:17:37+08:00` },
      { account_id: 2, phone: '13900009115', remark: '备用账号', cloud_gained: 11, migu_notes_gained: 0, success_count: 13, failed_count: 0, pending_count: 1, skipped_count: 0, last_executed_at: `${date}T08:15:27+08:00` }
    ], total: 2, page: 1, page_size: 20 } } })
  })
  await page.route('**/api/v1/tasks/logs**', route => {
    const date = new URL(route.request().url()).searchParams.get('date')!
    return route.fulfill({ json: { code: 0, message: 'success', data: { task_logs: [{ id: 901, user_id: 1, account_id: 1, account: { phone: '13900007646' }, task_type: 'coc3', status: 'failed', message: '活动验证未通过，任务未完成', cloud_gained: 0, execution_time: 100, created_at: `${date}T08:17:37+08:00` }], total: 1, page: 1, page_size: 20 } } })
  })
  await page.goto('/logs')
  await expect(page.getByRole('button', { name: '每日汇总', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await expect(page.locator('.daily-summary .el-table__row')).toHaveCount(2)
  await expect(page.locator('.daily-summary .summary-cloud-gain')).toHaveText(['+11', '+11'])
  await expect(page.locator('.daily-summary .summary-note-gain')).toHaveText(['+40', '0'])
  await expect(page.locator('.daily-summary')).not.toContainText('执行中')
  if (process.env.TASK_RECORD_SCREENSHOTS) await page.locator('.task-logs-container').screenshot({ path: path.join(process.env.TASK_RECORD_SCREENSHOTS, 'daily-summary-desktop.png') })
  const request = page.waitForRequest(request => {
    const url = new URL(request.url())
    return url.pathname === '/api/v1/tasks/logs' && url.searchParams.get('account_id') === '1' && url.searchParams.get('status') === 'failed' && url.searchParams.get('latest') === '1' && !!url.searchParams.get('date')
  })
  await page.getByRole('row').filter({ hasText: '139****7646' }).getByRole('button', { name: '失败 1 项' }).click()
  await request
  await expect(page.getByRole('button', { name: '任务明细', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByRole('switch', { name: '仅看确认收益' })).not.toBeChecked()
  await expect(page.getByRole('row').filter({ hasText: '139****7646' })).toContainText('活动验证未通过，任务未完成')
})

test('咪咕音符在明细和详情中独立展示，收益筛选保留音符任务', async ({ page }) => {
  await page.route('**/api/v1/tasks/logs**', route => route.fulfill({ json: { code: 0, message: 'success', data: { task_logs: [{
    id: 902, user_id: 1, account_id: 1, account: { phone: '13900007646' }, task_type: 'migu_song', status: 'success', message: '咪咕听歌30.0分钟，已确认领取7/7档，本次AI音符40', cloud_gained: 0, migu_notes_gained: 40, execution_time: 100, created_at: '2026-11-16T09:33:04+08:00'
  }], total: 1, page: 1, page_size: 20 } } }))
  await page.goto('/logs')
  await page.getByRole('button', { name: '任务明细', exact: true }).click()
  const row = page.getByRole('row').filter({ hasText: '咪咕听歌与领奖' })
  await expect(row.getByRole('cell').nth(3)).toHaveText('-')
  await expect(row.getByRole('cell').nth(4)).toHaveText('+40')
  await Promise.all([
    page.waitForRequest(request => new URL(request.url()).searchParams.get('has_reward') === '1'),
    page.getByText('仅看确认收益', { exact: true }).click()
  ])
  await expect(page.getByRole('switch', { name: '仅看确认收益' })).toBeChecked()
  await expect(row).toContainText('+40')
  await row.getByRole('button', { name: '详情', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '日志详情' })
  await expect(dialog).toContainText('获得云朵')
  await expect(dialog).toContainText('获得音符')
  await expect(dialog).toContainText('+40')
  await page.keyboard.press('Escape')
  await expect(dialog).not.toBeVisible()
  await page.setViewportSize({ width: 390, height: 844 })
  const noteRow = page.locator('.mobile-log-card .mobile-log-row').filter({ hasText: '获得音符' })
  await expect(noteRow).toContainText('+40')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
})

test('历史 running 记录展示为待确认，收益不被当作全部任务成功', async ({ page }) => {
  await page.route('**/api/v1/tasks/logs**', route => route.fulfill({ json: { code: 0, message: 'success', data: { task_logs: [{
    id: 900, user_id: 1, account_id: 1, account: { phone: '13900007646' }, task_type: 'tasklist', status: 'running', message: '执行中', cloud_gained: 11, execution_time: 10, created_at: '2026-11-12T08:17:37+08:00'
  }], total: 1, page: 1, page_size: 20 } } }))
  await page.goto('/logs')
  await page.getByRole('button', { name: '任务明细', exact: true }).click()
  const row = page.getByRole('row').filter({ hasText: '139****7646' })
  await expect(row.getByText('待确认', { exact: true })).toBeVisible()
  await expect(row).toContainText('执行已结束，结果待确认')
  await expect(row).toContainText('+11')
  await expect(row).not.toContainText('执行中')
})

test('同次操作的阶段和任务结果只形成一条通知，刷新重放不会累加云朵', async ({ page }) => {
  await page.goto('/dashboard')
  await page.waitForFunction(() => typeof (window as unknown as { __e2eRealtime?: unknown }).__e2eRealtime === 'function')
  const baseTime = Date.now() - 60_000
  const events = [
    { type: 'operation.updated', data: { status: 'queued' } },
    { type: 'operation.updated', data: { status: 'running' } },
    { type: 'task_summary', data: { account_id: 1, phone: '13900007646', total_gained: 11, cloud_count: 12064, task_count: 14, success_count: 12, failed_count: 1, pending_count: 1, operation_attempt: 1 } },
    { type: 'operation.updated', data: { status: 'failed', error_summary: '部分任务未完成：成功 12 项，失败 1 项，待确认 1 项；请查看任务明细' } }
  ].map((event, index) => ({ ...event, message_id: `e2e-result-${index}`, sequence: index + 1, created_at: baseTime + index, data: { ...event.data, operation_id: 'e2e-operation', type: 'account.task.execute', attempt_count: 1, queued_at: new Date(baseTime).toISOString(), updated_at: new Date(baseTime + index).toISOString() } }))
  const replay = () => page.evaluate(events => {
    const send = (window as unknown as { __e2eRealtime: (event: unknown) => void }).__e2eRealtime
    events.forEach(send)
  }, events)
  await replay()
  await page.getByRole('button', { name: /^通知中心/ }).click()
  const center = page.locator('.notification-center:visible')
  await expect(center.locator('.notification-item')).toHaveCount(1)
  await expect(center).toContainText('已确认到账 11 云朵，成功 12 项，失败 1 项，待确认 1 项')
  await expect(center).not.toContainText('12064')
  await expect(center).not.toContainText('操作执行中')
  await expect(center.locator('.notification-icon')).toHaveClass(/warning/)
  await center.locator('.notification-item').click()
  await expect(center.locator('.notification-item')).not.toHaveClass(/unread/)
  await page.reload()
  await page.waitForFunction(() => typeof (window as unknown as { __e2eRealtime?: unknown }).__e2eRealtime === 'function')
  await replay()
  await page.getByRole('button', { name: /^通知中心/ }).click()
  await expect(page.locator('.notification-center:visible .notification-item')).toHaveCount(1)
  await expect(page.locator('.notification-center:visible .notification-item')).not.toHaveClass(/unread/)
})

test('每日汇总在手机上每个账号只显示一张卡片，没有横向溢出', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/logs')
  await expect(page.locator('.summary-account')).toHaveCount(1)
  await expect(page.locator('.summary-account')).toContainText('+5 云朵')
  await expect(page.locator('.summary-account')).toContainText('+40 音符')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  if (process.env.TASK_RECORD_SCREENSHOTS) await page.locator('.task-logs-container').screenshot({ path: path.join(process.env.TASK_RECORD_SCREENSHOTS, 'daily-summary-mobile.png') })
})
