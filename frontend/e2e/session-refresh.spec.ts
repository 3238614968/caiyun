import { expect, test, type Page } from '@playwright/test'
import { adminUser, mockBackend } from './helpers/mockApi'

for (const mechanism of ['Web Locks', 'IndexedDB'] as const) {
  test(`两个标签同时续期只旋转一次会话（${mechanism}）`, async ({ context, page, baseURL }) => {
    const secondPage = await context.newPage()
    const pages = [page, secondPage]
    let initialProbes = 0
    let refreshCalls = 0
    let consumed = false
    let revoked = false
    let releaseInitialProbes!: () => void
    const bothInitialProbes = new Promise<void>(resolve => { releaseInitialProbes = resolve })
    let refreshStarted!: () => void
    const firstRefresh = new Promise<void>(resolve => { refreshStarted = resolve })
    let releaseRefresh!: () => void
    const allowRefreshResponse = new Promise<void>(resolve => { releaseRefresh = resolve })

    await context.addCookies([
      { name: 'auth_token', value: 'expired-access', url: baseURL!, httpOnly: true, sameSite: 'Lax' },
      { name: 'refresh_token', value: 'initial-refresh', url: baseURL!, httpOnly: true, sameSite: 'Lax' },
      { name: 'csrf_token', value: 'shared-csrf', url: baseURL!, sameSite: 'Lax' }
    ])

    const installSessionRoutes = async (tab: Page) => {
      await mockBackend(tab)
      // Count attempts while preserving each real browser locking mechanism.
      // This lets the fixture keep the first network response pending until
      // both tabs have entered the concurrent refresh path, without sleeps.
      await tab.addInitScript(mode => {
        const state = window as typeof window & { refreshLockAttempts: number }
        state.refreshLockAttempts = 0
        if (mode === 'Web Locks') {
          const request = navigator.locks.request.bind(navigator.locks)
          navigator.locks.request = ((...args: Parameters<typeof request>) => {
            state.refreshLockAttempts++
            return request(...args)
          }) as typeof request
        } else {
          Object.defineProperty(Navigator.prototype, 'locks', { configurable: true, get: () => undefined })
          const transaction = IDBDatabase.prototype.transaction
          IDBDatabase.prototype.transaction = function (...args: Parameters<typeof transaction>) {
            if (this.name === 'caiyun:session-refresh' && args[1] === 'readwrite') {
              state.refreshLockAttempts++
            }
            return transaction.apply(this, args)
          }
        }
      }, mechanism)

      let firstProbe = true
      await tab.route('**/api/v1/auth/me', async route => {
        if (firstProbe) {
          firstProbe = false
          initialProbes++
          if (initialProbes === 2) releaseInitialProbes()
          await bothInitialProbes
          return route.fulfill({ status: 401, json: { code: 401, message: 'access expired' } })
        }
        const cookie = route.request().headers().cookie || ''
        return cookie.includes('auth_token=fresh-access') && !revoked
          ? route.fulfill({ json: { code: 0, message: 'success', data: adminUser } })
          : route.fulfill({ status: 401, json: { code: 401, message: 'session revoked or expired' } })
      })
      await tab.route('**/api/v1/auth/refresh', async route => {
        expect(route.request().method()).toBe('POST')
        expect(route.request().headers()['x-csrf-token']).toBe('shared-csrf')
        refreshCalls++
        const cookie = route.request().headers().cookie || ''
        if (!cookie.includes('refresh_token=initial-refresh') || consumed) {
          // Match the server's replay policy: a second request using the old
          // refresh credential revokes the freshly issued replacement too.
          revoked = true
          return route.fulfill({ status: 401, json: { code: 401, message: 'refresh credential reused' } })
        }
        consumed = true
        refreshStarted()
        await allowRefreshResponse
        return route.fulfill({
          headers: {
            'set-cookie': [
              'auth_token=fresh-access; Path=/; HttpOnly; SameSite=Lax',
              'refresh_token=rotated-refresh; Path=/; HttpOnly; SameSite=Lax'
            ].join('\n')
          },
          json: { code: 0, message: 'success', data: { expires_at: 9999999999, refresh_expires_at: 9999999999 } }
        })
      })
    }

    await Promise.all(pages.map(installSessionRoutes))
    const navigation = Promise.all(pages.map(tab => tab.goto('/dashboard')))
    try {
      await firstRefresh
      await expect.poll(() => Promise.all(pages.map(tab => tab.evaluate(() => (
        window as typeof window & { refreshLockAttempts: number }
      ).refreshLockAttempts)))).toEqual([1, 1])
    } finally {
      releaseRefresh()
    }
    await navigation

    for (const tab of pages) {
      await expect(tab).toHaveURL(/\/dashboard$/)
      await expect(tab.getByText('当前云朵数')).toBeVisible()
      expect(await tab.evaluate(() => Boolean(navigator.locks))).toBe(mechanism === 'Web Locks')
    }
    expect(initialProbes).toBe(2)
    expect(refreshCalls).toBe(1)
    expect(revoked).toBe(false)
    expect((await context.cookies()).find(cookie => cookie.name === 'refresh_token')?.value).toBe('rotated-refresh')
  })
}
