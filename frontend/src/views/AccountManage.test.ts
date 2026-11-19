import { DOMWrapper, mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AccountManage from './AccountManage.vue'
import { createAccount, getAccounts, updateAccount, type Account } from '@/api/account'

vi.mock('@/api/account', () => ({
  getAccounts: vi.fn(),
  getAllAccounts: vi.fn(),
  createAccount: vi.fn(),
  updateAccount: vi.fn(),
  deleteAccount: vi.fn(),
  deleteAdminAccount: vi.fn(),
  setAccountStatus: vi.fn(),
  updateAccountStatus: vi.fn(),
  triggerAccountTask: vi.fn(),
  sendSmsCode: vi.fn(),
  smsLogin: vi.fn()
}))

vi.mock('@/store/auth', () => ({
  useAuthStore: () => ({ user: { id: 1, role: 'user' } })
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ replace: vi.fn() })
}))

const account = {
  id: 7,
  user_id: 1,
  phone: '13800000007',
  remark: '原账号',
  is_active: true,
  cloud_count: 100,
  created_at: '2026-11-11T00:00:00Z'
} as Account

function button(label: string) {
  const target = new DOMWrapper(document.body).findAll('button').find(item => item.text() === label)
  if (!target) throw new Error(`找不到按钮：${label}`)
  return target
}

describe('account editing and creation', () => {
  let wrapper: VueWrapper | undefined

  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(getAccounts).mockResolvedValue({ accounts: [account], total: 1, page: 1, page_size: 10 })
    vi.mocked(createAccount).mockResolvedValue({ ...account, id: 8 })
    vi.mocked(updateAccount).mockResolvedValue(account)
    vi.stubGlobal('ResizeObserver', class {
      observe() {}
      unobserve() {}
      disconnect() {}
    })
  })

  afterEach(() => {
    wrapper?.unmount()
    document.body.innerHTML = ''
    vi.unstubAllGlobals()
  })

  it.each(['cancel', 'save'] as const)('creates a new account after an edit is closed with %s', async closeMode => {
    wrapper = mount(AccountManage, { attachTo: document.body })
    await flushPromises()

    await button('编辑').trigger('click')
    await flushPromises()
    await button(closeMode === 'cancel' ? '取消' : '确定').trigger('click')
    await flushPromises()
    if (closeMode === 'save') {
      expect(updateAccount).toHaveBeenCalledWith(7, { phone: account.phone, remark: account.remark })
    }
    vi.mocked(updateAccount).mockClear()

    await button('添加账号').trigger('click')
    await flushPromises()
    const dialog = new DOMWrapper(document.body).get('.el-dialog')
    await dialog.get('input[placeholder="请输入手机号"]').setValue('13900000008')
    await dialog.get('textarea[placeholder="请输入Auth"]').setValue('new-account-auth')
    await dialog.get('input[placeholder="请输入备注"]').setValue('新账号')
    await button('确定').trigger('click')
    await flushPromises()

    expect(createAccount).toHaveBeenCalledOnce()
    expect(createAccount).toHaveBeenCalledWith({ phone: '13900000008', auth: 'new-account-auth', remark: '新账号' })
    expect(updateAccount).not.toHaveBeenCalled()
  })
})
