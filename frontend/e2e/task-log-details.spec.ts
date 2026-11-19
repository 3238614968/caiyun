import { expect, test } from '@playwright/test'
import { authenticateAsAdmin, mockBackend } from './helpers/mockApi'

test('任务详情保留 COC 进度和未完成原因，不把待确认显示为运行中', async ({ page }) => {
  await mockBackend(page)
  await authenticateAsAdmin(page)
  await page.route('**/api/v1/tasks/logs**', route => route.fulfill({
    json: {
      code: 0,
      message: 'success',
      data: {
        task_logs: [{
          id: 900, account_id: 1, task_type: 'coc3', status: 'pending',
          account: { phone: '13900000001' }, cloud_gained: 0, execution_time: 500,
          message: '服务端确认完成0/3项，任务进度3040:0/3 | 原因: 抽奖令牌生成失败，需安装 Node.js 18+ | build=fixture',
          created_at: '2026-11-11T20:00:00+08:00'
        }],
        total: 1, page: 1, page_size: 20
      }
    }
  }))
  await page.goto('/logs')
  await page.getByRole('button', { name: '任务明细', exact: true }).click()
  await expect(page.getByRole('switch', { name: '仅看确认收益' })).not.toBeChecked()
  const row = page.getByRole('row').filter({ hasText: /coc3|中国移动权益市场/ })
  await expect(row.getByText('待确认', { exact: true })).toBeVisible()
  await row.getByRole('button', { name: '详情' }).click()
  const dialog = page.getByRole('dialog', { name: '日志详情' })
  await expect(dialog.locator('.log-detail-message')).toContainText('服务端确认完成0/3项，任务进度3040:0/3')
  await expect(dialog.locator('.log-detail-message')).toContainText('抽奖令牌生成失败，需安装 Node.js 18+')
  await expect(dialog).not.toContainText('build=fixture')
})
