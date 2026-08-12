import { expect, test } from '@playwright/test'

const adminUsername = process.env.E2E_ADMIN_USERNAME || 'e2e-admin'
const adminPassword = process.env.E2E_ADMIN_PASSWORD || 'E2eAdminPass123!'
const apiHealthURL = process.env.API_HEALTH_URL || 'http://backend-api:8080/health'
const workerHealthURL = process.env.WORKER_HEALTH_URL || 'http://backend-worker:8081/health'

async function loginAsE2EAdmin(page: import('@playwright/test').Page) {
  await page.goto('/dashboard')
  await expect(page).toHaveURL(/\/login(?:\?redirect=.*)?$/)
  await page.getByPlaceholder('请输入用户名').fill(adminUsername)
  await page.getByPlaceholder('请输入密码').fill(adminPassword)
  await page.getByRole('button', { name: '登录' }).click()
  await expect(page).toHaveURL(/\/(?:dashboard)?$/)
}

test('真实 Compose 服务健康检查通过', async ({ request }) => {
  const [apiHealth, workerHealth] = await Promise.all([
    request.get(apiHealthURL),
    request.get(workerHealthURL)
  ])

  expect(apiHealth.ok()).toBe(true)
  expect(workerHealth.ok()).toBe(true)
})

test('真实前后端链路可登录并访问核心页面', async ({ page }) => {
  await loginAsE2EAdmin(page)
  await expect(page.getByText('移动云盘').first()).toBeVisible()
  await expect(page.getByText('当前云朵数')).toBeVisible()

  await page.goto('/accounts')
  await expect(page.getByText('账号管理')).toBeVisible()
  await expect(page.getByText('E2E 主账号')).toBeVisible()

  await page.goto('/logs')
  await expect(page.getByText('运行日志')).toBeVisible()
  const logsTable = page.locator('.task-logs-container .el-table')
  await expect(logsTable.getByText('13900009001', { exact: true })).toBeVisible()
  await expect(logsTable.getByText('+6', { exact: true })).toBeVisible()

  await page.goto('/exchange')
  await expect(page.getByText('商品中心')).toBeVisible()
  await expect(page.getByText('E2E 视频会员')).toBeVisible()

  await page.goto('/admin')
  await expect(page.getByText('管理员面板')).toBeVisible()
})

test('真实 Operation 经 Redis Worker 到达终态', async ({ page }) => {
  await loginAsE2EAdmin(page)
  const csrfToken = (await page.context().cookies()).find(cookie => cookie.name === 'csrf_token')?.value
  expect(csrfToken).toBeTruthy()

  const api = page.context().request
  const submit = await api.post(new URL('/api/v1/admin/exchange/execute-monthly', page.url()).toString(), {
    headers: {
      'Idempotency-Key': `e2e-operation-${Date.now()}`,
      'X-CSRF-Token': csrfToken!
    }
  })
  expect(submit.status()).toBe(202)
  const accepted = await submit.json()
  const operationID = accepted.data?.operation_id as string | undefined
  expect(operationID).toBeTruthy()

  await expect.poll(async () => {
    const response = await api.get(new URL(`/api/v1/operations/${operationID}`, page.url()).toString())
    expect(response.ok()).toBe(true)
    const body = await response.json()
    return body.data?.status
  }, { timeout: 30_000 }).toBe('succeeded')
})

test('真实 Operation 终态通过 SSE operation.updated 到达浏览器', async ({ page }) => {
  await loginAsE2EAdmin(page)

  const event = await page.evaluate(async () => new Promise<{ operationID: string; status: string }>((resolve, reject) => {
    const source = new EventSource('/events', { withCredentials: true })
    let operationID = ''
    let submitted = false
    const earlyUpdates: Array<{ operation_id?: string; status?: string }> = []
    const timeout = window.setTimeout(() => {
      source.close()
      reject(new Error('timed out waiting for operation.updated'))
    }, 30_000)
    const finish = (value: { operationID: string; status: string }) => {
      window.clearTimeout(timeout)
      source.close()
      resolve(value)
    }
    source.onerror = () => {
      // EventSource may report a transient error during reconnect. Keep the
      // stream alive until the bounded test timeout decides the result.
    }
    const handleUpdate = (update: { operation_id?: string; status?: string }) => {
      if (update.operation_id === operationID && update.status === 'succeeded') {
        finish({ operationID, status: update.status })
      }
    }
    source.onmessage = message => {
      const envelope = JSON.parse(message.data) as { type?: string; data?: { operation_id?: string; status?: string } }
      if (envelope.type === 'operation.updated' && envelope.data) {
        if (operationID) {
          handleUpdate(envelope.data)
        } else {
          earlyUpdates.push(envelope.data)
        }
      }
    }
    source.onopen = async () => {
      if (submitted) return
      submitted = true
      try {
        const csrf = document.cookie.split('; ').find(value => value.startsWith('csrf_token='))?.split('=').slice(1).join('')
        const response = await fetch('/api/v1/admin/exchange/execute-monthly', {
          method: 'POST',
          credentials: 'same-origin',
          headers: {
            'Idempotency-Key': `e2e-sse-operation-${Date.now()}`,
            'X-CSRF-Token': decodeURIComponent(csrf || '')
          }
        })
        const body = await response.json()
        operationID = String(body.data?.operation_id || '')
        if (response.status !== 202 || !operationID) {
          throw new Error(`operation submit failed: ${response.status}`)
        }
        earlyUpdates.forEach(handleUpdate)
      } catch (error) {
        window.clearTimeout(timeout)
        source.close()
        reject(error)
      }
    }
  }))

  expect(event.operationID).toBeTruthy()
  expect(event.status).toBe('succeeded')
})

test('SSE 通过前端代理重放 Last-Event-ID 之后的持久化消息', async ({ page }) => {
  await loginAsE2EAdmin(page)

  const replay = await page.evaluate(async () => {
    const controller = new AbortController()
    const timeout = window.setTimeout(() => controller.abort(), 10_000)
    try {
      const response = await fetch('/events', {
        headers: { 'Last-Event-ID': '41' },
        signal: controller.signal
      })
      if (!response.ok || !response.body) {
        return { status: response.status, body: '' }
      }
      const reader = response.body.getReader()
      const decoder = new TextDecoder()
      let body = ''
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        body += decoder.decode(value, { stream: true })
        if (body.includes('e2e-sse-42')) {
          await reader.cancel()
          return { status: response.status, body }
        }
      }
      return { status: response.status, body }
    } finally {
      window.clearTimeout(timeout)
    }
  })

  expect(replay.status).toBe(200)
  expect(replay.body).toContain('e2e-sse-42')
  expect(replay.body).toContain('operation.updated')
})
