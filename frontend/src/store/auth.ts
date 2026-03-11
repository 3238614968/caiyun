import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { login as apiLogin, register as apiRegister } from '@/api/auth'

export interface User {
  id: number
  username: string
  email: string
  role: string
}

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string>(localStorage.getItem('token') || '')
  const user = ref<User | null>(null)

  const isAuthenticated = computed(() => !!token.value)

  async function login(username: string, password: string) {
    const data = await apiLogin({ username, password })
    token.value = data.token
    user.value = data.user as User
    localStorage.setItem('token', data.token)
    localStorage.setItem('user', JSON.stringify(data.user))
    return data
  }

  async function register(username: string, password: string, email?: string) {
    const data = await apiRegister({ username, password, email })
    token.value = data.token
    user.value = data.user as User
    localStorage.setItem('token', data.token)
    localStorage.setItem('user', JSON.stringify(data.user))
    return data
  }

  function logout() {
    token.value = ''
    user.value = null
    localStorage.removeItem('token')
    localStorage.removeItem('user')
  }

  function initialize() {
    const savedUser = localStorage.getItem('user')
    if (savedUser && savedUser !== 'undefined') {
      try {
        user.value = JSON.parse(savedUser)
      } catch {
        localStorage.removeItem('user')
      }
    }
  }

  return {
    token,
    user,
    isAuthenticated,
    login,
    register,
    logout,
    initialize
  }
})
