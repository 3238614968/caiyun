import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useExchangeForms } from './useExchangeForms'
import { useExchangeTaskActions } from './useExchangeTaskActions'
import { immediateExchange, type ExchangeRule, type Product } from '@/api/exchange'
import type { Account } from '@/api/account'

vi.mock('@/api/exchange', () => ({
  createExchangeTask: vi.fn(),
  immediateExchange: vi.fn()
}))

vi.mock('element-plus', () => ({
  ElMessage: { warning: vi.fn(), success: vi.fn(), error: vi.fn(), info: vi.fn() }
}))

function setup(useRules = false) {
  const forms = useExchangeForms()
  const product = { id: 10, prize_name: '测试商品' } as Product
  const actions = useExchangeTaskActions({
    ...forms,
    isAdmin: ref(false),
    products: ref([product]),
    accounts: ref(useRules ? [
      { id: 101, is_active: true },
      { id: 202, is_active: true }
    ] as ExchangeRule[] : []),
    userAccounts: ref(useRules ? [] : [
      { id: 101, is_active: true },
      { id: 202, is_active: true }
    ] as Account[]),
    loadProducts: vi.fn().mockResolvedValue(undefined),
    loadAccounts: vi.fn().mockResolvedValue(undefined),
    loadTasks: vi.fn().mockResolvedValue(undefined),
    loadUserAccounts: vi.fn().mockResolvedValue(undefined),
    syncDefaultProductSelection: vi.fn()
  })
  return { ...forms, ...actions }
}

describe('exchange task account selection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(immediateExchange).mockResolvedValue({
      operation_id: 'operation-1',
      type: 'immediate_exchange',
      status: 'queued',
      attempt_count: 0,
      queued_at: '2026-11-11T00:00:00Z',
      created_at: '2026-11-11T00:00:00Z',
      updated_at: '2026-11-11T00:00:00Z'
    })
  })

  it('submits the selected immediate account instead of the previous batch default', async () => {
    const form = setup()
    await form.prepareTaskForm()
    expect(form.taskForm.value.account_ids).toEqual([101])

    // The immediate dialog changes the single-account selection.
    form.taskForm.value.account_id = 202
    expect(await form.confirmImmediateExchange()).toBe(true)
    expect(immediateExchange).toHaveBeenCalledWith(expect.objectContaining({ account_id: 202 }))
    expect(vi.mocked(immediateExchange).mock.calls[0][0]).not.toHaveProperty('account_ids')
  })

  it('does not exchange with a stale batch account after the selection is cleared', async () => {
    const form = setup()
    await form.prepareTaskForm()
    form.taskForm.value.account_id = null

    expect(await form.confirmImmediateExchange()).toBe(false)
    expect(immediateExchange).not.toHaveBeenCalled()
  })

  it('preserves multiple selected accounts when creating a scheduled task', async () => {
    const form = setup()
    await form.prepareTaskForm()
    form.taskForm.value.account_ids = [202, 101, 202]

    expect(form.buildTaskPayload()).toMatchObject({ account_id: 202, account_ids: [202, 101] })
  })

  it('uses the selected immediate rule without sending stale batch rule IDs', async () => {
    const form = setup(true)
    await form.prepareTaskForm()
    form.taskForm.value.exchange_rule_id = 202
    form.taskForm.value.exchange_account_id = 202

    expect(await form.confirmImmediateExchange()).toBe(true)
    expect(immediateExchange).toHaveBeenCalledWith(expect.objectContaining({ exchange_rule_id: 202, exchange_account_id: 202 }))
    expect(vi.mocked(immediateExchange).mock.calls[0][0]).not.toHaveProperty('exchange_rule_ids')
  })

  it('does not restore a cleared immediate rule from the previous batch default', async () => {
    const form = setup(true)
    await form.prepareTaskForm()
    form.taskForm.value.exchange_rule_id = null
    form.taskForm.value.exchange_account_id = null

    expect(await form.confirmImmediateExchange()).toBe(false)
    expect(immediateExchange).not.toHaveBeenCalled()
  })
})
