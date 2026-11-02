import { expect, test } from '@playwright/test'
import { authenticateAsAdmin, mockBackend } from './helpers/mockApi'

test.use({ video: 'off' })

for (const force of [false, true]) {
  test(`短信登录${force ? '明确强制替换' : '默认保护'}当前凭据`, async ({ page }) => {
    await mockBackend(page)
    await authenticateAsAdmin(page)
    let sendPayload: Record<string, unknown> | undefined
    let loginPayload: Record<string, unknown> | undefined
    await page.route('**/api/v1/accounts/sms/send', async route => {
      sendPayload = route.request().postDataJSON()
      await route.fulfill({ json: { code: 200, data: { phone: '13900009999', task_id: 'sms-fixture' } } })
    })
    await page.route('**/api/v1/accounts/sms/verify', async route => {
      loginPayload = route.request().postDataJSON()
      await route.fulfill({ json: { code: 200, data: { id: 99, phone: '13900009999', is_active: true } } })
    })

    await page.goto('/accounts')
    await page.getByRole('button', { name: '添加账号' }).click()
    const dialog = page.getByRole('dialog', { name: '添加账号' })
    await dialog.getByRole('tab', { name: '短信登录' }).click()
    await dialog.getByPlaceholder('请输入11位手机号').fill('13900009999')
    const forceCheckbox = dialog.getByRole('checkbox', { name: '强制替换已有凭据（当前凭据将立即失效）' })
    await expect(forceCheckbox).not.toBeChecked()
    if (force) {
      await dialog.getByText('强制替换已有凭据（当前凭据将立即失效）', { exact: true }).click()
      await expect(forceCheckbox).toBeChecked()
    }
    await dialog.getByRole('button', { name: '发送验证码' }).click()
    await expect.poll(() => sendPayload?.force).toBe(force)
    await dialog.getByPlaceholder('请输入验证码').fill('123456')
    if (force) await page.screenshot({ path: 'test-results/sms-force-login.png' })
    await dialog.getByRole('button', { name: '确定' }).click()
    await expect.poll(() => loginPayload?.force).toBe(force)
    expect(loginPayload?.task_id).toBe('sms-fixture')
    await expect(dialog).toBeHidden()
  })
}
