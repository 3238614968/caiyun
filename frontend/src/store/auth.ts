import { defineStore } from 'pinia'
import { ref, computed, onScopeDispose } from 'vue'
import { getCurrentUser, login as apiLogin, register as apiRegister, logout as apiLogout } from '@/api/auth'
import { resetAuthExpiredState } from '@/api/axios'
import type { User } from '@/api/auth'

const logoutMarker = 'caiyun:logout-pending'

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const token = ref<string>('') // 兼容旧调用；真实令牌由 HttpOnly Cookie 保存
  const hasCheckedSession = ref(false)
  const logoutPending = ref(localStorage.getItem(logoutMarker) === '1')
  const loggingOut = ref(false)
  let logoutPromise: Promise<void> | null = null
  let sessionVersion = 0
  let refreshProfilePromise: Promise<User> | null = null

  const isAuthenticated = computed(() => !!user.value)

  function clearAuthState(options: { sessionChecked?: boolean } = {}) {
    sessionVersion++
    refreshProfilePromise = null
    const previousUserID = user.value?.id
    token.value = ''
    user.value = null
    if (typeof options.sessionChecked === 'boolean') {
      hasCheckedSession.value = options.sessionChecked
    }
    // 兼容历史版本：启动或退出时清理旧 localStorage 用户缓存。
    localStorage.removeItem('user')
    localStorage.removeItem('caiyun_notifications')
    if (previousUserID) {
      localStorage.removeItem(`caiyun_notifications:${previousUserID}`)
    }
  }

  async function login(username: string, password: string) {
    if (logoutPromise) await logoutPromise
    const data = await apiLogin({ username, password })
    acceptLogin(data.user)
    return data
  }

  async function register(username: string, password: string, email?: string) {
    if (logoutPromise) await logoutPromise
    const data = await apiRegister({ username, password, email })
    acceptLogin(data.user)
    return data
  }

  function acceptLogin(currentUser: User) {
    sessionVersion++
    refreshProfilePromise = null
    logoutPending.value = false
    localStorage.removeItem(logoutMarker)
    resetAuthExpiredState()
    token.value = 'cookie'
    user.value = currentUser
    hasCheckedSession.value = true
    localStorage.removeItem('user')
  }

  // refreshProfile 从服务端拉取最新用户信息，覆盖本地内存状态。
  // 不再把用户信息写入 localStorage，降低 XSS 读取身份信息的影响面。
  async function refreshProfile(fetchMe: () => Promise<User> = getCurrentUser) {
    if (logoutPending.value) throw new Error('退出尚未确认')
    if (refreshProfilePromise) {
      return refreshProfilePromise
    }

    const version = sessionVersion
    refreshProfilePromise = (async () => {
      try {
        const me = await fetchMe()
        if (version !== sessionVersion) throw new Error('会话已变更')
        user.value = me
        token.value = 'cookie'
        hasCheckedSession.value = true
        resetAuthExpiredState()
        localStorage.removeItem('user')
        return me
      } catch (error) {
        if (version === sessionVersion) clearAuthState({ sessionChecked: true })
        throw error
      } finally {
        if (version === sessionVersion) refreshProfilePromise = null
      }
    })()

    return refreshProfilePromise
  }

  function logout(): Promise<void> {
    if (logoutPromise) return logoutPromise
    localStorage.setItem(logoutMarker, '1')
    logoutPending.value = true
    clearAuthState({ sessionChecked: true })
    loggingOut.value = true
    logoutPromise = apiLogout().then(() => {
      localStorage.removeItem(logoutMarker)
      logoutPending.value = false
    }).finally(() => {
      logoutPromise = null
      loggingOut.value = false
    })
    return logoutPromise
  }

  function initialize() {
    localStorage.removeItem('user')
    hasCheckedSession.value = logoutPending.value
  }

  if (typeof window !== 'undefined') {
    const clearSession = () => clearAuthState({ sessionChecked: true })
    const syncLogout = (event: StorageEvent) => {
      if (event.key !== logoutMarker) return
      logoutPending.value = event.newValue === '1'
      if (logoutPending.value) clearSession()
    }
    window.addEventListener('auth:clear', clearSession)
    window.addEventListener('storage', syncLogout)
    onScopeDispose(() => {
      window.removeEventListener('auth:clear', clearSession)
      window.removeEventListener('storage', syncLogout)
    })
  }

  return {
    token,
    user,
    isAuthenticated,
    hasCheckedSession,
    logoutPending,
    loggingOut,
    login,
    register,
    logout,
    initialize,
    clearAuthState,
    refreshProfile
  }
})
