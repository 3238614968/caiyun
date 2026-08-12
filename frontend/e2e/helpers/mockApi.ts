import type { Page, Route } from '@playwright/test'

export const adminUser = {
  id: 1,
  username: 'e2e-admin',
  email: 'e2e@example.com',
  role: 'admin'
}

const now = '2026-06-06T00:00:00+08:00'

const dashboardData = {
  total_cloud: 12345,
  account_count: 3,
  today_gained: 120,
  yesterday_diff: 32,
  week_diff: 210,
  success_rate: 98.5,
  trend_data: [
    { date: '2026-06-01', cloud_count: 11000 },
    { date: '2026-06-02', cloud_count: 11200, cloud_diff: 200 },
    { date: '2026-06-03', cloud_count: 11500, cloud_diff: 300 },
    { date: '2026-06-04', cloud_count: 11880, cloud_diff: 380 },
    { date: '2026-06-05', cloud_count: 12225, cloud_diff: 345 },
    { date: '2026-06-06', cloud_count: 12345, cloud_diff: 120 }
  ],
  account_ranking: [
    { account_id: 1, phone: '13900000001', remark: '主账号', cloud_count: 6800 },
    { account_id: 2, phone: '13900000002', remark: '备用账号', cloud_count: 5545 }
  ]
}

const adminDashboardData = {
  total_cloud: dashboardData.total_cloud,
  account_count: dashboardData.account_count,
  user_count: 2,
  today_gained: dashboardData.today_gained,
  yesterday_gained: 88,
  success_rate: dashboardData.success_rate,
  account_ranking: [
    {
      account_id: 1,
      phone: '13900000001',
      remark: '主账号',
      owner_username: adminUser.username,
      cloud_count: 6800,
      today_gained: 80
    },
    {
      account_id: 2,
      phone: '13900000002',
      remark: '备用账号',
      owner_username: adminUser.username,
      cloud_count: 5545,
      today_gained: 40
    }
  ]
}

const announcement = {
  id: 1,
  title: 'E2E 公告',
  content: '这是一条端到端测试公告。',
  is_popup: false,
  is_top: true,
  is_published: true,
  popup_count: 0,
  created_at: now,
  updated_at: now
}

const baseAccount = {
  id: 1,
  user_id: adminUser.id,
  phone: '13900000001',
  auth: 'AUTH=fixture',
  token: '',
  jwt_token: '',
  platform: 'android',
  expire_at: 1780761600000,
  cloud_count: 6800,
  remark: '主账号',
  is_active: true,
  created_at: now,
  updated_at: now,
  user: { username: adminUser.username }
}

const secondaryAccount = {
  ...baseAccount,
  id: 2,
  phone: '13900000002',
  cloud_count: 5545,
  remark: '备用账号'
}

const baseProduct = {
  id: 101,
  prize_id: '1001',
  prize_name: 'E2E月卡',
  p_order: 100,
  category: '月卡',
  daily_remainder_count: 8,
  daily_limit_count: 10,
  stock_status: 'available',
  last_stock_check: now,
  is_active: true,
  is_deleted: false,
  created_at: now,
  updated_at: now
}

const soldOutProduct = {
  ...baseProduct,
  id: 102,
  prize_id: '1002',
  prize_name: 'E2E售罄券',
  p_order: 200,
  category: '奶茶饮品权益',
  daily_remainder_count: 0,
  daily_limit_count: 10,
  stock_status: 'sold_out'
}

const baseExchangeAccount = {
  id: 11,
  user_id: adminUser.id,
  account_id: baseAccount.id,
  phone: baseAccount.phone,
  remark: '抢兑主账号',
  exchange_time_1: '10:00:00',
  exchange_time_2: '20:00:00',
  is_active: true,
  product_id: baseProduct.id,
  product: baseProduct,
  created_at: now,
  updated_at: now
}

const secondaryExchangeAccount = {
  ...baseExchangeAccount,
  id: 12,
  account_id: secondaryAccount.id,
  phone: secondaryAccount.phone,
  remark: '抢兑备用账号'
}

const baseExchangeRecord = {
  id: 201,
  user_id: adminUser.id,
  exchange_account_id: baseExchangeAccount.id,
  exchange_task_id: 30,
  product_id: baseProduct.id,
  prize_id: baseProduct.prize_id,
  prize_name: baseProduct.prize_name,
  status: 'success',
  message: '兑换成功',
  execution_time_ms: 856,
  created_at: now,
  exchange_account: baseExchangeAccount,
  product: baseProduct
}

const baseExchangeConfig = {
  auto_update_products: false,
  concurrency: 10,
  enabled: true,
  exchange_monthly_enabled: false,
  exchange_time: '10:00',
  monthly_prize_id: '1001',
  immediate_exchange_enabled: false
}

const baseTaskConfig = {
  id: 1,
  task_type: 'signin',
  task_name: '每日签到',
  is_enabled: true,
  sort_order: 1,
  updated_at: now
}

function ok(data: unknown) {
  return {
    code: 0,
    message: 'success',
    data
  }
}

function fulfill(route: Route, data: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: 'application/json',
    body: JSON.stringify(data)
  })
}

function installMockWebSocket(page: Page) {
  return page.addInitScript(() => {
    class MockWebSocket extends EventTarget {
      static CONNECTING = 0
      static OPEN = 1
      static CLOSING = 2
      static CLOSED = 3

      url: string
      readyState = MockWebSocket.CONNECTING
      onopen: ((event: Event) => void) | null = null
      onmessage: ((event: MessageEvent) => void) | null = null
      onclose: ((event: Event) => void) | null = null
      onerror: ((event: Event) => void) | null = null

      constructor(url: string) {
        super()
        this.url = url
        window.setTimeout(() => {
          this.readyState = MockWebSocket.OPEN
          const event = new Event('open')
          this.onopen?.(event)
          this.dispatchEvent(event)
        }, 0)
      }

      send() {}

      close() {
        this.readyState = MockWebSocket.CLOSED
        const event = new Event('close')
        this.onclose?.(event)
        this.dispatchEvent(event)
      }
    }

    class MockEventSource extends EventTarget {
      static CONNECTING = 0
      static OPEN = 1
      static CLOSED = 2

      url: string
      withCredentials: boolean
      readyState = MockEventSource.CONNECTING
      onopen: ((event: Event) => void) | null = null
      onmessage: ((event: MessageEvent) => void) | null = null
      onerror: ((event: Event) => void) | null = null

      constructor(url: string, init?: EventSourceInit) {
        super()
        this.url = url
        this.withCredentials = Boolean(init?.withCredentials)
        window.setTimeout(() => {
          this.readyState = MockEventSource.OPEN
          const event = new Event('open')
          this.onopen?.(event)
          this.dispatchEvent(event)
        }, 0)
      }

      close() {
        this.readyState = MockEventSource.CLOSED
      }
    }

    // Default production transport is SSE. Mock it as well as WebSocket so
    // browser unit scenarios never make accidental requests to Vite's proxy.
    window.WebSocket = MockWebSocket as unknown as typeof WebSocket
    window.EventSource = MockEventSource as unknown as typeof EventSource
  })
}

export function loginResponse() {
  return ok({
    expires_at: Math.floor(Date.now() / 1000) + 3600,
    user: adminUser
  })
}

export async function mockBackend(page: Page) {
  await installMockWebSocket(page)

  let accountSeq = 10
  let announcementSeq = 10
  let exchangeAccountSeq = 20
  let exchangeTaskSeq = 30
  let operationSeq = 0
  const operations = new Map<string, Record<string, unknown>>()
  const queueOperation = (
    type: string,
    references: { account_id?: number; resource_id?: number } = {}
  ) => {
    operationSeq += 1
    const operation = {
      operation_id: `op-e2e-${String(operationSeq).padStart(4, '0')}`,
      type,
      status: 'queued',
      attempt_count: 0,
      queued_at: now,
      created_at: now,
      updated_at: now,
      ...references
    }
    operations.set(String(operation.operation_id), operation)
    return operation
  }
  const accounts = [{ ...baseAccount }, { ...secondaryAccount }]
  const exchangeAccounts = [{ ...baseExchangeAccount }, { ...secondaryExchangeAccount }]
  const exchangeTasks: any[] = []
  const exchangeRecords = [
    { ...baseExchangeRecord },
    {
      ...baseExchangeRecord,
      id: 202,
      status: 'failed',
      message: '库存不足',
      execution_time_ms: 2450
    }
  ]
  const products = [{ ...baseProduct }, { ...soldOutProduct }]
  const taskConfigs = [{ ...baseTaskConfig }]
  const exchangeConfig = { ...baseExchangeConfig }
  const announcements = [{ ...announcement }]
  const adminUsers = [
    { ...adminUser, created_at: now, updated_at: now },
    { id: 2, username: 'e2e-user', email: 'user@example.com', role: 'user', created_at: now, updated_at: now }
  ]
  let isAuthenticated = false

  await page.route('**/api/v1/**', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const method = request.method()
    const path = url.pathname

    if (method === 'POST' && path === '/api/v1/auth/login') {
      isAuthenticated = true
      return fulfill(route, loginResponse())
    }

    if (method === 'POST' && path === '/api/v1/auth/logout') {
      isAuthenticated = false
      return fulfill(route, ok({ message: 'ok' }))
    }

    if (method === 'GET' && path === '/api/v1/auth/me') {
      const cookie = request.headers().cookie || ''
      if (!isAuthenticated && !cookie.includes('e2e_auth=1')) {
        return fulfill(route, { code: 401, message: '未提供认证信息' }, 401)
      }
      return fulfill(route, ok(adminUser))
    }

    const operationCancelMatch = path.match(/^\/api\/v1\/operations\/([^/]+)\/cancel$/)
    if (operationCancelMatch && method === 'POST') {
      const operation = operations.get(operationCancelMatch[1]) || {}
      const canceled = { ...operation, status: 'canceled', completed_at: now, updated_at: now }
      operations.set(operationCancelMatch[1], canceled)
      return fulfill(route, ok(canceled))
    }

    const operationMatch = path.match(/^\/api\/v1\/operations\/([^/]+)$/)
    if (operationMatch && method === 'GET') {
      const operation = operations.get(operationMatch[1])
      return operation
        ? fulfill(route, ok(operation))
        : fulfill(route, { code: 404, message: '操作不存在' }, 404)
    }

    if (method === 'GET' && path === '/api/v1/stats/dashboard') {
      return fulfill(route, ok(dashboardData))
    }

    if (method === 'GET' && path === '/api/v1/admin/dashboard') {
      return fulfill(route, ok(adminDashboardData))
    }

    if (method === 'GET' && path === '/api/v1/stats/trend') {
      const days = Number(url.searchParams.get('days') || 7)
      const base = dashboardData.trend_data
      const trend = Array.from({ length: days }, (_, index) => {
        const source = base[Math.max(0, base.length - days + index)] || base[base.length - 1]
        return {
          ...source,
          date: `2026-06-${String(Math.max(1, 7 - days + index)).padStart(2, '0')}`,
          cloud_count: 12000 + index * 15,
          cloud_diff: index === 0 ? 0 : 15
        }
      })
      trend[trend.length - 1] = { ...trend[trend.length - 1], date: '2026-06-06', cloud_count: 12345, cloud_diff: 120 }
      return fulfill(route, ok({ trend_data: trend }))
    }

    if (method === 'GET' && path === '/api/v1/announcements') {
      const published = announcements.filter(item => item.is_published)
      return fulfill(route, ok({ announcements: published, total: published.length }))
    }

    if (method === 'GET' && path === '/api/v1/announcements/popup') {
      return fulfill(route, ok({ has_popup: false, announcements: [] }))
    }

    if (method === 'GET' && path === '/api/v1/accounts') {
      const phone = url.searchParams.get('phone') || ''
      const pageSize = Number(url.searchParams.get('page_size') || 10)
      const filtered = phone
        ? accounts.filter(account => account.phone.includes(phone))
        : accounts
      return fulfill(route, ok({ accounts: filtered, total: filtered.length, page: 1, page_size: pageSize }))
    }

    if (method === 'POST' && path === '/api/v1/accounts') {
      const body = request.postDataJSON()
      const created = {
        ...baseAccount,
        id: ++accountSeq,
        phone: body.phone,
        auth: body.auth,
        remark: body.remark || '',
        cloud_count: 0,
        created_at: now,
        updated_at: now
      }
      accounts.push(created)
      return fulfill(route, ok(created))
    }

    const accountIDMatch = path.match(/^\/api\/v1\/accounts\/(\d+)$/)
    if (accountIDMatch && method === 'PUT') {
      const accountID = Number(accountIDMatch[1])
      const body = request.postDataJSON()
      const account = accounts.find(item => item.id === accountID)
      if (account) {
        Object.assign(account, {
          phone: body.phone ?? account.phone,
          auth: body.auth ?? account.auth,
          remark: body.remark ?? account.remark,
          updated_at: now
        })
      }
      return fulfill(route, ok(account || {}))
    }

    if (accountIDMatch && method === 'DELETE') {
      const accountID = Number(accountIDMatch[1])
      const index = accounts.findIndex(item => item.id === accountID)
      if (index >= 0) {
        accounts.splice(index, 1)
      }
      return fulfill(route, ok({ message: 'deleted' }))
    }

    const accountStatusMatch = path.match(/^\/api\/v1\/accounts\/(\d+)\/status$/)
    if (accountStatusMatch && method === 'PUT') {
      const accountID = Number(accountStatusMatch[1])
      const account = accounts.find(item => item.id === accountID)
      if (account) {
        account.is_active = url.searchParams.get('is_active') === 'true'
      }
      return fulfill(route, ok({ message: 'ok' }))
    }

    const accountTriggerMatch = path.match(/^\/api\/v1\/accounts\/(\d+)\/trigger$/)
    if (accountTriggerMatch && method === 'POST') {
      return fulfill(
        route,
        ok(queueOperation('account_tasks', { account_id: Number(accountTriggerMatch[1]) })),
        202
      )
    }

    if (method === 'POST' && path === '/api/v1/tasks/trigger-all') {
      return fulfill(route, ok(queueOperation('all_account_tasks')), 202)
    }

    if (method === 'GET' && path === '/api/v1/tasks/logs') {
      const taskType = url.searchParams.get('task_type') || ''
      const status = url.searchParams.get('status') || ''
      const logs = [
        {
          id: 501,
          user_id: adminUser.id,
          account_id: baseAccount.id,
          account: baseAccount,
          task_type: 'signin',
          status: 'success',
          message: '每日签到完成 | 结果: 已完成',
          cloud_gained: 5,
          execution_time: 320,
          created_at: now
        },
        {
          id: 502,
          user_id: adminUser.id,
          account_id: secondaryAccount.id,
          account: secondaryAccount,
          task_type: 'exchange',
          status: 'failed',
          message: '结果: 库存不足 | trace_id=e2e',
          cloud_gained: 0,
          execution_time: 856,
          created_at: now
        }
      ].filter(log => (!taskType || log.task_type === taskType) && (!status || log.status === status))
      return fulfill(route, ok({ task_logs: logs, total: logs.length, page: 1, page_size: 20 }))
    }

    if (method === 'GET' && path === '/api/v1/tasks/status') {
      return fulfill(route, ok({ tasks: [] }))
    }

    if (method === 'GET' && path === '/api/v1/tasks/queue-status') {
      return fulfill(route, ok({
        backend: 'streams',
        backend_meta: {
          stream_key: 'task:queue:stream',
          consumer_group: 'caiyun-workers',
          consumer_name: 'e2e-worker',
          max_len_approx: '100000'
        },
        is_healthy: true,
        errors: [],
        queue_length: 0,
        processing_count: 0,
        delayed_count: 0,
        dead_letter_count: 0,
        active_workers: 0,
        pending_tasks: 0,
        completed_tasks: 0,
        successful_tasks: 0,
        failed_tasks: 0
      }))
    }

    if (method === 'GET' && path === '/api/v1/products/search') {
      const keyword = url.searchParams.get('keyword') || ''
      const filtered = keyword
        ? products.filter(product => product.prize_name.includes(keyword))
        : products
      return fulfill(route, ok({ products: filtered, total: filtered.length }))
    }

    if (method === 'GET' && path === '/api/v1/products/categories') {
      return fulfill(route, ok({ categories: ['月卡', '流量'] }))
    }

    if (method === 'POST' && path === '/api/v1/products/update') {
      return fulfill(route, ok({ account_id: request.postDataJSON()?.account_id || 0, count: products.length }))
    }

    if (method === 'GET' && path === '/api/v1/exchange/config') {
      return fulfill(route, ok({
        enabled: exchangeConfig.enabled,
        immediate_exchange_enabled: exchangeConfig.immediate_exchange_enabled
      }))
    }

    if (method === 'GET' && (path === '/api/v1/exchange/accounts' || path === '/api/v1/exchange/rules' || path === '/api/v1/admin/exchange/rules')) {
      return fulfill(route, ok({ accounts: exchangeAccounts, total: exchangeAccounts.length }))
    }

    if (method === 'POST' && (path === '/api/v1/exchange/accounts' || path === '/api/v1/exchange/rules')) {
      const body = request.postDataJSON()
      const cloudAccount = accounts.find(item => item.id === Number(body.account_id)) || accounts[0]
      const created = {
        ...baseExchangeAccount,
        id: ++exchangeAccountSeq,
        account_id: Number(body.account_id),
        phone: cloudAccount.phone,
        remark: body.remark || cloudAccount.remark,
        exchange_time_1: body.exchange_time_1 || '10:00:00',
        exchange_time_2: body.exchange_time_2 || '20:00:00',
        product_id: body.product_id || baseProduct.id,
        created_at: now,
        updated_at: now
      }
      exchangeAccounts.push(created)
      return fulfill(route, ok({ account: created }))
    }

    const exchangeAccountMatch = path.match(/^\/api\/v1\/exchange\/(?:accounts|rules)\/(\d+)$/)
    if (exchangeAccountMatch && method === 'PUT') {
      const account = exchangeAccounts.find(item => item.id === Number(exchangeAccountMatch[1]))
      if (account) {
        Object.assign(account, request.postDataJSON(), { updated_at: now })
      }
      return fulfill(route, ok({ message: 'updated' }))
    }

    if (exchangeAccountMatch && method === 'DELETE') {
      const index = exchangeAccounts.findIndex(item => item.id === Number(exchangeAccountMatch[1]))
      if (index >= 0) {
        exchangeAccounts.splice(index, 1)
      }
      return fulfill(route, ok({ message: 'deleted' }))
    }

    if (method === 'POST' && path === '/api/v1/exchange/immediate') {
      const body = request.postDataJSON()
      return fulfill(route, ok(queueOperation('immediate_exchange', {
        account_id: Number(body.account_id || 0),
        resource_id: Number(body.product_id || 0)
      })), 202)
    }

    if (method === 'POST' && path === '/api/v1/exchange/tasks/batch-execute') {
      return fulfill(route, ok(queueOperation('batch_exchange_tasks')), 202)
    }

    if (method === 'GET' && (path === '/api/v1/exchange/tasks' || path === '/api/v1/admin/exchange/tasks')) {
      return fulfill(route, ok({ tasks: exchangeTasks, total: exchangeTasks.length }))
    }

    if (method === 'GET' && path === '/api/v1/exchange/records') {
      const productName = url.searchParams.get('product_name') || ''
      const status = url.searchParams.get('status') || ''
      const accountID = Number(url.searchParams.get('account_id') || 0)
      const filtered = exchangeRecords.filter(record => {
        const matchesProduct = productName ? record.prize_name.includes(productName) : true
        const matchesStatus = status ? record.status === status : true
        const matchesAccount = accountID ? record.exchange_account_id === accountID : true
        return matchesProduct && matchesStatus && matchesAccount
      })
      return fulfill(route, ok({
        records: filtered,
        total: filtered.length,
        stats: {
          success: filtered.filter(record => record.status === 'success').length,
          failed: filtered.filter(record => record.status === 'failed').length
        }
      }))
    }

    if (method === 'GET' && path === '/api/v1/exchange/records/export') {
      return route.fulfill({
        status: 200,
        contentType: 'text/csv;charset=utf-8',
        body: 'id,prize_name,status\n201,E2E月卡,success\n'
      })
    }

    if (method === 'POST' && path === '/api/v1/exchange/tasks') {
      const body = request.postDataJSON()
      const product = products.find(item => item.id === Number(body.product_id)) || baseProduct
      const explicitRuleIDs = Array.isArray(body.exchange_account_ids) ? body.exchange_account_ids.map(Number) : []
      if (body.exchange_account_id) explicitRuleIDs.push(Number(body.exchange_account_id))
      const cloudAccountIDs = Array.isArray(body.account_ids) ? body.account_ids.map(Number) : []
      if (body.account_id) cloudAccountIDs.push(Number(body.account_id))

      const selectedRules = explicitRuleIDs.length > 0
        ? explicitRuleIDs.map(id => exchangeAccounts.find(item => item.id === id)).filter(Boolean)
        : cloudAccountIDs.map(id => {
            const cloudAccount = accounts.find(item => item.id === id) || baseAccount
            return exchangeAccounts.find(item => item.account_id === cloudAccount.id) || {
              ...baseExchangeAccount,
              id: ++exchangeAccountSeq,
              account_id: cloudAccount.id,
              phone: cloudAccount.phone,
              remark: `抢兑${cloudAccount.remark}`,
              created_at: now,
              updated_at: now
            }
          })

      const rules = selectedRules.length > 0 ? selectedRules : [baseExchangeAccount]
      const createdTasks = rules.map((exchangeAccount: any) => {
        const created = {
          id: ++exchangeTaskSeq,
          user_id: adminUser.id,
          exchange_account_id: exchangeAccount.id,
          product_id: product.id,
          prize_id: product.prize_id,
          prize_name: product.prize_name,
          task_type: body.task_type || 'fixed',
          max_attempts: body.max_attempts || 1,
          scheduled_exchange_time: body.scheduled_exchange_time || '',
          restock_cycle: body.restock_cycle || 'daily',
          restock_weekday: body.restock_weekday ?? null,
          restock_day_of_month: body.restock_day_of_month ?? null,
          attempted_count: 0,
          status: 'pending',
          success_count: 0,
          fail_count: 0,
          last_result: '',
          created_at: now,
          updated_at: now,
          exchange_account: exchangeAccount,
          product
        }
        exchangeTasks.push(created)
        return created
      })
      return fulfill(route, ok({ task: createdTasks[0], tasks: createdTasks, created: createdTasks.length, errors: [] }))
    }

    if (path.match(/^\/api\/v1\/exchange\/tasks\/(\d+)$/) && method === 'DELETE') {
      return fulfill(route, ok({ message: 'deleted' }))
    }

    const exchangeTaskExecuteMatch = path.match(/^\/api\/v1\/exchange\/tasks\/(\d+)\/execute$/)
    if (exchangeTaskExecuteMatch && method === 'POST') {
      return fulfill(route, ok(queueOperation('exchange_task', {
        resource_id: Number(exchangeTaskExecuteMatch[1])
      })), 202)
    }

    if (method === 'GET' && path === '/api/v1/admin/accounts/summaries') {
      return fulfill(route, ok({
        summaries: accounts.map(account => ({
          ...account,
          owner_username: adminUser.username,
          today_gained: 80,
          yesterday_gained: 70,
          success_count: 3,
          failed_count: 0,
          last_executed_at: now
        })),
        total: accounts.length,
        page: 1,
        page_size: 20
      }))
    }

    if (method === 'GET' && path === '/api/v1/admin/accounts') {
      return fulfill(route, ok({ accounts, total: accounts.length, page: 1, page_size: 1000 }))
    }

    const adminAccountMatch = path.match(/^\/api\/v1\/admin\/accounts\/(\d+)$/)
    if (adminAccountMatch && method === 'DELETE') {
      const accountID = Number(adminAccountMatch[1])
      const index = accounts.findIndex(account => account.id === accountID)
      if (index >= 0) accounts.splice(index, 1)
      return fulfill(route, ok({ message: 'deleted' }))
    }

    if (method === 'GET' && path === '/api/v1/admin/accounts/search') {
      return fulfill(route, ok({
        accounts: accounts.map(account => ({
          id: account.id,
          phone: account.phone,
          remark: account.remark,
          user_id: account.user_id,
          username: adminUser.username,
          is_active: account.is_active
        }))
      }))
    }

    if (method === 'GET' && path === '/api/v1/admin/task-configs') {
      return fulfill(route, ok({ configs: taskConfigs }))
    }

    const taskConfigMatch = path.match(/^\/api\/v1\/admin\/task-configs\/([^/]+)$/)
    if (taskConfigMatch && method === 'PUT') {
      const body = request.postDataJSON()
      const config = taskConfigs.find(item => item.task_type === taskConfigMatch[1])
      if (config) {
        config.is_enabled = Boolean(body.is_enabled)
        config.updated_at = now
      }
      return fulfill(route, ok({ message: 'updated' }))
    }

    if (method === 'GET' && path === '/api/v1/admin/exchange/config') {
      return fulfill(route, ok(exchangeConfig))
    }

    if (method === 'PUT' && path === '/api/v1/admin/exchange/config') {
      Object.assign(exchangeConfig, request.postDataJSON())
      return fulfill(route, ok({ message: 'saved' }))
    }

    if (method === 'POST' && path === '/api/v1/admin/exchange/execute-monthly') {
      return fulfill(route, ok(queueOperation('monthly_exchange')), 202)
    }

    if (method === 'GET' && path === '/api/v1/admin/users') {
      return fulfill(route, ok({ users: adminUsers, total: adminUsers.length, page: 1, size: 10 }))
    }

    const adminUserMatch = path.match(/^\/api\/v1\/admin\/users\/(\d+)$/)
    if (adminUserMatch && method === 'DELETE') {
      const index = adminUsers.findIndex(user => user.id === Number(adminUserMatch[1]))
      if (index >= 0) {
        adminUsers.splice(index, 1)
      }
      return fulfill(route, ok({ message: 'deleted' }))
    }

    const adminUserRoleMatch = path.match(/^\/api\/v1\/admin\/users\/(\d+)\/role$/)
    if (adminUserRoleMatch && method === 'PUT') {
      const body = request.postDataJSON()
      const user = adminUsers.find(item => item.id === Number(adminUserRoleMatch[1]))
      if (user) {
        user.role = body.role || user.role
      }
      return fulfill(route, ok({ message: 'updated' }))
    }

    if (path.match(/^\/api\/v1\/admin\/users\/(\d+)\/password$/) && method === 'PUT') {
      return fulfill(route, ok({ message: 'password reset' }))
    }

    if (method === 'GET' && path === '/api/v1/admin/stats/overview') {
      return fulfill(route, ok({ user_count: 1, account_count: accounts.length, total_cloud: 6800, active_tasks: 1 }))
    }

    if (method === 'GET' && path === '/api/v1/admin/announcements') {
      return fulfill(route, ok({ announcements, total: announcements.length }))
    }

    if (method === 'POST' && path === '/api/v1/admin/announcements') {
      const body = request.postDataJSON()
      const created = {
        ...announcement,
        id: ++announcementSeq,
        title: body.title,
        content: body.content,
        is_popup: Boolean(body.is_popup),
        is_top: Boolean(body.is_top),
        is_published: true,
        popup_count: 0,
        created_at: now,
        updated_at: now
      }
      announcements.unshift(created)
      return fulfill(route, ok({ announcement: created }))
    }

    const adminAnnouncementMatch = path.match(/^\/api\/v1\/admin\/announcements\/(\d+)$/)
    if (adminAnnouncementMatch && method === 'GET') {
      const item = announcements.find(row => row.id === Number(adminAnnouncementMatch[1])) || null
      return fulfill(route, ok({ announcement: item }))
    }

    if (adminAnnouncementMatch && method === 'PUT') {
      const body = request.postDataJSON()
      const item = announcements.find(row => row.id === Number(adminAnnouncementMatch[1]))
      if (item) {
        Object.assign(item, body, { updated_at: now })
      }
      return fulfill(route, ok({ announcement: item || null }))
    }

    if (adminAnnouncementMatch && method === 'DELETE') {
      const index = announcements.findIndex(row => row.id === Number(adminAnnouncementMatch[1]))
      if (index >= 0) {
        announcements.splice(index, 1)
      }
      return fulfill(route, ok({ message: 'deleted' }))
    }

    return fulfill(route, ok({}))
  })
}

export async function authenticateAsAdmin(page: Page) {
  const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:4173'
  await page.context().addCookies([
    {
      name: 'e2e_auth',
      value: '1',
      url: baseURL
    }
  ])
}
