import request from './axios'
import { unwrapApiData, type ApiResponse } from './response'

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

export interface ResetPasswordRequest {
  username: string
  email: string
  code: string
  new_password: string
}

export interface SendPasswordResetCodeRequest {
  username: string
  email: string
}

// 注册请求
export interface RegisterRequest {
  username: string
  password: string
  email?: string
}

// 登录/注册响应
export interface AuthResponse {
  expires_at: number
  user: User
}

function unwrapRequired<T>(value: T | ApiResponse<T>, fallback: T): T {
  return unwrapApiData(value, fallback)
}

// 登录
export function login(data: LoginRequest): Promise<AuthResponse> {
  const fallback: AuthResponse = { expires_at: 0, user: {} as User }
  return request<AuthResponse | ApiResponse<AuthResponse>>({
    url: '/api/auth/login',
    method: 'post',
    data
  }).then((res) => unwrapRequired(res, fallback))
}

// 注册
export function register(data: RegisterRequest): Promise<AuthResponse> {
  const fallback: AuthResponse = { expires_at: 0, user: {} as User }
  return request<AuthResponse | ApiResponse<AuthResponse>>({
    url: '/api/auth/register',
    method: 'post',
    data
  }).then((res) => unwrapRequired(res, fallback))
}

// 发送密码重置邮箱验证码
export function sendPasswordResetCode(data: SendPasswordResetCodeRequest): Promise<{ message: string }> {
  return request({
    url: '/api/auth/password/reset-code/send',
    method: 'post',
    data
  })
}

// 通过邮箱验证码重置密码
export function resetPassword(data: ResetPasswordRequest): Promise<{ message: string }> {
  return request({
    url: '/api/auth/password/reset',
    method: 'post',
    data
  })
}

// 刷新Token
export function refreshToken(): Promise<{ expires_at: number }> {
  const fallback = { expires_at: 0 }
  return request<typeof fallback | ApiResponse<typeof fallback>>({
    url: '/api/auth/refresh',
    method: 'post'
  }).then((res) => unwrapRequired(res, fallback))
}

// 退出登录
export function logout(): Promise<{ message: string }> {
  return request({
    url: '/api/auth/logout',
    method: 'post'
  })
}

// 获取当前用户信息
export function getCurrentUser(): Promise<User> {
  return request<User | ApiResponse<User>>({
    url: '/api/auth/me',
    method: 'get',
    // /me 用于启动时探测 Cookie 会话；未登录时 401 是正常结果，不应触发全局错误提示。
    silentAuthError: true
  }).then((res) => unwrapRequired(res, {} as User))
}
