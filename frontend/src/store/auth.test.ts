import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, disposePinia, setActivePinia } from 'pinia'
import { getCurrentUser, login, logout, type User } from '@/api/auth'
import { useAuthStore } from './auth'

vi.mock('@/api/auth', () => ({ getCurrentUser: vi.fn(), login: vi.fn(), logout: vi.fn(), register: vi.fn() }))
vi.mock('@/api/axios', () => ({ resetAuthExpiredState: vi.fn() }))

const marker = 'caiyun:logout-pending'
const user: User = { id: 5, username: 'fixture', email: '', role: 'user' }

function pending<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(done => { resolve = done })
  return { promise, resolve }
}

describe('logout confirmation', () => {
  let pinia: ReturnType<typeof createPinia>
  beforeEach(() => {
    localStorage.clear()
    vi.resetAllMocks()
    pinia = createPinia()
    setActivePinia(pinia)
  })
  afterEach(() => disposePinia(pinia))

  it('locks immediately, persists a failed logout across reload, and unlocks only after confirmation', async () => {
    const store = useAuthStore()
    await store.refreshProfile(async () => user)
    vi.mocked(logout).mockRejectedValueOnce(new Error('offline'))
    const failed = store.logout()
    expect(store.isAuthenticated).toBe(false)
    await expect(failed).rejects.toThrow('offline')
    expect(localStorage.getItem(marker)).toBe('1')
    disposePinia(pinia)
    pinia = createPinia()
    setActivePinia(pinia)
    const reloaded = useAuthStore()
    reloaded.initialize()
    expect(reloaded.hasCheckedSession).toBe(true)
    await expect(reloaded.refreshProfile()).rejects.toThrow('退出尚未确认')
    expect(getCurrentUser).not.toHaveBeenCalled()
    const confirmation = pending<{ message: string }>()
    vi.mocked(logout).mockReturnValueOnce(confirmation.promise)
    const retries = [reloaded.logout(), reloaded.logout()]
    expect(logout).toHaveBeenCalledTimes(2)
    expect(reloaded.loggingOut).toBe(true)
    confirmation.resolve({ message: 'ok' })
    await Promise.all(retries)
    expect(reloaded.logoutPending).toBe(false)
    expect(reloaded.loggingOut).toBe(false)
    expect(localStorage.getItem(marker)).toBeNull()
  })

  it('does not restore an in-flight profile after logout or overwrite a new login', async () => {
    const store = useAuthStore()
    const profile = pending<User>()
    const old = store.refreshProfile(() => profile.promise)
    vi.mocked(logout).mockRejectedValueOnce(new Error('offline'))
    await expect(store.logout()).rejects.toThrow('offline')
    const newUser = { ...user, id: 6 }
    vi.mocked(login).mockResolvedValueOnce({ expires_at: 60, refresh_expires_at: 120, user: newUser })
    await store.login('fixture', '')
    profile.resolve(user)
    await expect(old).rejects.toThrow('会话已变更')
    expect(store.user).toEqual(newUser)
    expect(store.logoutPending).toBe(false)
    expect(localStorage.getItem(marker)).toBeNull()
  })

  it('locks other open tabs without implicitly logging back in when the marker clears', async () => {
    const store = useAuthStore()
    await store.refreshProfile(async () => user)
    window.dispatchEvent(new StorageEvent('storage', { key: marker, newValue: '1' }))
    expect(store.user).toBeNull()
    expect(store.logoutPending).toBe(true)
    window.dispatchEvent(new StorageEvent('storage', { key: marker, newValue: null }))
    expect(store.logoutPending).toBe(false)
    expect(store.isAuthenticated).toBe(false)
    expect(getCurrentUser).not.toHaveBeenCalled()
  })
})
