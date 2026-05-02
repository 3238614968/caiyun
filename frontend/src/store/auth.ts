import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { login as apiLogin, register as apiRegister, logout as apiLogout } from '@/api/auth'

export interface User {
  id: number
  username: string
  email: string
  role: string
}

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const token = ref<string>('') // 兼容旧调用；真实令牌由 HttpOnly Cookie 保存

  const isAuthenticated = computed(() => !!user.value)

  function clearAuthState() {
    token.value = ''
    user.value = null
    localStorage.removeItem('user')
  }

  async function login(username: string, password: string) {
    const data = await apiLogin({ username, password })
    token.value = 'cookie'
    user.value = data.user as User
    localStorage.setItem('user', JSON.stringify(data.user))
    return data
  }

  async function register(username: string, password: string, email?: string) {
    const data = await apiRegister({ username, password, email })
    token.value = 'cookie'
    user.value = data.user as User
    localStorage.setItem('user', JSON.stringify(data.user))
    return data
  }

  async function logout() {
    try {
      await apiLogout()
    } catch {
      // 即使服务端清理失败，也清理本地状态。
    }
    clearAuthState()
  }

  function initialize() {
    const savedUser = localStorage.getItem('user')
    if (savedUser && savedUser !== 'undefined') {
      try {
        user.value = JSON.parse(savedUser)
        token.value = 'cookie'
      } catch {
        localStorage.removeItem('user')
      }
    }
  }

  if (typeof window !== 'undefined') {
    window.addEventListener('auth:clear', clearAuthState)
  }

  return {
    token,
    user,
    isAuthenticated,
    login,
    register,
    logout,
    initialize,
    clearAuthState
  }
})
