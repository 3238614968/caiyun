import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { getCurrentUser, login as apiLogin, register as apiRegister, logout as apiLogout } from '@/api/auth'
import { resetAuthExpiredState } from '@/api/axios'
import type { User } from '@/api/auth'

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const token = ref<string>('') // 兼容旧调用；真实令牌由 HttpOnly Cookie 保存
  const hasCheckedSession = ref(false)
  let refreshProfilePromise: Promise<User> | null = null

  const isAuthenticated = computed(() => !!user.value)

  function clearAuthState(options: { sessionChecked?: boolean } = {}) {
    token.value = ''
    user.value = null
    if (typeof options.sessionChecked === 'boolean') {
      hasCheckedSession.value = options.sessionChecked
    }
    // 兼容历史版本：启动或退出时清理旧 localStorage 用户缓存。
    localStorage.removeItem('user')
  }

  async function login(username: string, password: string) {
    const data = await apiLogin({ username, password })
    resetAuthExpiredState()
    token.value = 'cookie'
    user.value = data.user as User
    hasCheckedSession.value = true
    localStorage.removeItem('user')
    return data
  }

  async function register(username: string, password: string, email?: string) {
    const data = await apiRegister({ username, password, email })
    resetAuthExpiredState()
    token.value = 'cookie'
    user.value = data.user as User
    hasCheckedSession.value = true
    localStorage.removeItem('user')
    return data
  }

  // refreshProfile 从服务端拉取最新用户信息，覆盖本地内存状态。
  // 不再把用户信息写入 localStorage，降低 XSS 读取身份信息的影响面。
  async function refreshProfile(fetchMe: () => Promise<User> = getCurrentUser) {
    if (refreshProfilePromise) {
      return refreshProfilePromise
    }

    refreshProfilePromise = (async () => {
      try {
        const me = await fetchMe()
        user.value = me
        token.value = 'cookie'
        hasCheckedSession.value = true
        resetAuthExpiredState()
        localStorage.removeItem('user')
        return me
      } catch (error) {
        clearAuthState({ sessionChecked: true })
        throw error
      } finally {
        refreshProfilePromise = null
      }
    })()

    return refreshProfilePromise
  }

  async function logout() {
    try {
      await apiLogout()
    } catch {
      // 即使服务端清理失败，也清理本地状态。
    }
    clearAuthState({ sessionChecked: true })
  }

  function initialize() {
    localStorage.removeItem('user')
    hasCheckedSession.value = false
  }

  if (typeof window !== 'undefined') {
    window.addEventListener('auth:clear', () => clearAuthState({ sessionChecked: true }))
  }

  return {
    token,
    user,
    isAuthenticated,
    hasCheckedSession,
    login,
    register,
    logout,
    initialize,
    clearAuthState,
    refreshProfile
  }
})
