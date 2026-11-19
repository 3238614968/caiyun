import { expect, test } from '@playwright/test'
import { authenticateAsAdmin, mockBackend } from './helpers/mockApi'

test('旧通知缓存显示简短结果，保留账号商品和已读状态', async ({ page }) => {
  await mockBackend(page)
  await authenticateAsAdmin(page)
  await page.addInitScript(() => {
    localStorage.setItem('caiyun_notifications:1', JSON.stringify([{
      id: 'exchange_exchange_complete_legacy', level: 'error', title: '抢兑失败', read: true,
      timestamp: new Date().toISOString(),
      message: '小海 · QQ音乐绿钻会员月卡：您的云朵数不足 | http_status=200 | code=1001 | body={"code":1001,"msg":"您的云朵数不足"} | 滑块识别offset=376 confidence=0.8511 method=内置多特征NCC peak_margin=0.5681 solver=builtin solve_ms=93.76 session=bound-h5 资格 exchangeCode=0 | build=current commit=dirty built_at=2026-11-12'
    }]))
  })
  await page.goto('/dashboard')
  await page.getByRole('button', { name: '通知中心', exact: true }).click()
  const center = page.locator('.notification-center:visible')
  await expect(center.locator('.notification-message')).toHaveText('小海 · QQ音乐绿钻会员月卡：云朵不足，兑换失败')
  await expect(center.locator('.notification-item')).not.toHaveClass(/unread/)
  await expect(center).not.toContainText(/offset=|confidence=|http_status=|body=|build=|commit=/)
  await center.getByRole('button', { name: '查看全部' }).click()
  const history = page.getByRole('dialog', { name: '通知历史' })
  await expect(history).toContainText('小海 · QQ音乐绿钻会员月卡：云朵不足，兑换失败')
  await expect(history).not.toContainText(/offset=|confidence=|http_status=|body=|build=|commit=/)
})
