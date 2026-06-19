import axios, { type AxiosError, type AxiosInstance, type AxiosRequestConfig, type AxiosResponse } from 'axios'
import { ElMessage } from 'element-plus'

interface AppAxiosRequestConfig extends AxiosRequestConfig {
  /**
   * 会话探测类请求使用：401 只代表未登录，不应弹出全局错误或触发二次跳转。
   */
  silentAuthError?: boolean
}

function appPath(path: string): string {
  const base = import.meta.env.BASE_URL || '/'
  const normalizedBase = base.endsWith('/') ? base.slice(0, -1) : base
  const normalizedPath = path.startsWith('/') ? path : `/${path}`
  return `${normalizedBase}${normalizedPath}` || normalizedPath
}

// 创建 axios 实例
const service: AxiosInstance = axios.create({
  // 注意：本项目各接口 `url` 已包含 `/api/...` 前缀，因此这里默认不再追加 `/api`，
  // 否则会出现 `/api/api/...` 导致 404。
  baseURL: import.meta.env.VITE_API_BASE_URL || '',
  timeout: 30000,
  withCredentials: true,
  headers: {
    'Content-Type': 'application/json'
  }
})

let authExpiredHandled = false

export function resetAuthExpiredState(): void {
  authExpiredHandled = false
}

function isSilentAuthError(error: AxiosError<ErrorResponseBody>): boolean {
  const config = error.config as AppAxiosRequestConfig | undefined
  const url = String(config?.url || '')
  return !!config?.silentAuthError || url.includes('/api/auth/me')
}

// 请求拦截器
service.interceptors.request.use(
  (config) => {
    const method = (config.method || 'get').toLowerCase()
    if (['post', 'put', 'patch', 'delete'].includes(method)) {
      const csrfToken = getCookie('csrf_token')
      if (csrfToken) {
        config.headers['X-CSRF-Token'] = csrfToken
      }
    }
    return config
  },
  (error) => {
    console.error('请求错误:', error)
    return Promise.reject(error)
  }
)

function getCookie(name: string): string {
  const prefix = `${encodeURIComponent(name)}=`
  return document.cookie
    .split(';')
    .map(item => item.trim())
    .find(item => item.startsWith(prefix))
    ?.slice(prefix.length) || ''
}

interface ErrorResponseBody {
  message?: string
  errors?: Record<string, string[] | string>
}

function validationErrorMessage(data: ErrorResponseBody): string {
  if (data.message) return data.message
  const first = Object.values(data.errors || {})[0]
  if (Array.isArray(first)) return first.join('；')
  return first || '提交内容校验失败，请检查后重试'
}

function isLoginPath(path: string): boolean {
  return path === appPath('/login') || path.startsWith(`${appPath('/login')}?`)
}

function safeCurrentRedirect(): string {
  const current = window.location.pathname + window.location.search
  if (!current.startsWith('/') || current.startsWith('//') || current.includes('\\')) {
    return '/'
  }
  return current
}

function redirectToLogin(): void {
  if (isLoginPath(window.location.pathname)) {
    return
  }
  const redirect = safeCurrentRedirect()
  void import('@/router').then(({ default: router }) => {
    router.replace({ name: 'Login', query: redirect !== '/' ? { redirect } : undefined })
  })
}

// 响应拦截器
service.interceptors.response.use(
  (response: AxiosResponse) => {
    const { data } = response
    return data
  },
  (error: AxiosError<ErrorResponseBody>) => {
    const { response } = error

    if (response) {
      const { status, data } = response

      switch (status) {
        case 400:
          ElMessage.error(data.message || '请求参数错误')
          break
        case 401:
          if (!isSilentAuthError(error)) {
            // 多个并发请求同时 401 时，只提示/跳转一次，避免刷屏。
            if (!authExpiredHandled) {
              authExpiredHandled = true
              ElMessage.error('登录已过期，请重新登录')
              redirectToLogin()
            }
            // 清除登录状态。会话探测请求由调用方自行处理，不触发全局提示。
            localStorage.removeItem('user')
            window.dispatchEvent(new Event('auth:clear'))
          }
          break
        case 403:
          ElMessage.error('没有权限执行此操作')
          break
        case 404:
          ElMessage.error(data.message || '请求的资源不存在')
          break
        case 422:
          ElMessage.error(validationErrorMessage(data))
          break
        case 429:
          ElMessage.error(data.message || '请求过于频繁，请稍后重试')
          break
        case 500:
          ElMessage.error(data.message || '服务器内部错误')
          break
        default:
          ElMessage.error(data.message || '网络错误')
      }
    } else {
      ElMessage.error('网络连接失败，请检查网络')
    }

    return Promise.reject(error)
  }
)

// 封装请求方法
export default function request<T = unknown>(config: AppAxiosRequestConfig): Promise<T> {
  return service.request(config) as Promise<T>
}

// 导出 axios 实例的方法
export const http = {
  get: <T = unknown>(url: string, config?: AxiosRequestConfig): Promise<T> =>
    service.get(url, config) as Promise<T>,
  post: <T = unknown, D = unknown>(url: string, data?: D, config?: AxiosRequestConfig): Promise<T> =>
    service.post(url, data, config) as Promise<T>,
  put: <T = unknown, D = unknown>(url: string, data?: D, config?: AxiosRequestConfig): Promise<T> =>
    service.put(url, data, config) as Promise<T>,
  delete: <T = unknown>(url: string, config?: AxiosRequestConfig): Promise<T> =>
    service.delete(url, config) as Promise<T>,
  patch: <T = unknown, D = unknown>(url: string, data?: D, config?: AxiosRequestConfig): Promise<T> =>
    service.patch(url, data, config) as Promise<T>,
}
