import axios, { type AxiosInstance, type AxiosRequestConfig, type AxiosResponse } from 'axios'
import { ElMessage } from 'element-plus'

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

// 响应拦截器
service.interceptors.response.use(
  (response: AxiosResponse) => {
    const { data } = response
    return data
  },
  (error) => {
    const { response } = error

    if (response) {
      const { status, data } = response

      switch (status) {
        case 400:
          ElMessage.error(data.message || '请求参数错误')
          break
        case 401:
          ElMessage.error('登录已过期，请重新登录')
          // 清除登录状态并跳转到登录页
          localStorage.removeItem('user')
          window.location.href = '/login'
          break
        case 403:
          ElMessage.error('没有权限执行此操作')
          break
        case 404:
          ElMessage.error(data.message || '请求的资源不存在')
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
export default function request<T = any>(config: AxiosRequestConfig): Promise<T> {
  return service.request(config) as Promise<T>
}

// 导出 axios 实例的方法
export const http = {
  get: <T = any>(url: string, config?: AxiosRequestConfig): Promise<T> =>
    service.get(url, config) as Promise<T>,
  post: <T = any>(url: string, data?: any, config?: AxiosRequestConfig): Promise<T> =>
    service.post(url, data, config) as Promise<T>,
  put: <T = any>(url: string, data?: any, config?: AxiosRequestConfig): Promise<T> =>
    service.put(url, data, config) as Promise<T>,
  delete: <T = any>(url: string, config?: AxiosRequestConfig): Promise<T> =>
    service.delete(url, config) as Promise<T>,
  patch: <T = any>(url: string, data?: any, config?: AxiosRequestConfig): Promise<T> =>
    service.patch(url, data, config) as Promise<T>,
}
