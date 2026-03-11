import request from './axios'

// 用户接口
export interface User {
  id: number
  username: string
  email: string
  role: string
  created_at?: string
}

// 登录请求
export interface LoginRequest {
  username: string
  password: string
}

// 注册请求
export interface RegisterRequest {
  username: string
  password: string
  email?: string
}

// 登录/注册响应
export interface AuthResponse {
  token: string
  user: User
}

// 登录
export function login(data: LoginRequest): Promise<AuthResponse> {
  return request({
    url: '/api/auth/login',
    method: 'post',
    data
  })
}

// 注册
export function register(data: RegisterRequest): Promise<AuthResponse> {
  return request({
    url: '/api/auth/register',
    method: 'post',
    data
  })
}

// 刷新Token
export function refreshToken(): Promise<{ token: string }> {
  return request({
    url: '/api/auth/refresh',
    method: 'post'
  })
}

// 获取当前用户信息
export function getCurrentUser(): Promise<User> {
  return request({
    url: '/api/auth/me',
    method: 'get'
  })
}
