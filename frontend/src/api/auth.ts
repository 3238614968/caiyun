import request from './axios'
import { unwrapApiData, type ApiResponse } from './response'
import type {
  AuthResponse as AuthResponseContract,
  AuthUser as AuthUserContract,
  LoginRequest as LoginRequestContract,
  RefreshTokenResponse as RefreshTokenResponseContract,
  RegisterRequest as RegisterRequestContract,
  ResetPasswordRequest as ResetPasswordRequestContract,
  SendPasswordResetCodeRequest as SendPasswordResetCodeRequestContract
} from './generated/operation-contract'

// Cookie 会话 DTO 由 OpenAPI 生成；created_at 为旧页面显示字段兼容位。
export type User = AuthUserContract & { created_at?: string }
export type LoginRequest = LoginRequestContract
export type ResetPasswordRequest = ResetPasswordRequestContract
export type SendPasswordResetCodeRequest = SendPasswordResetCodeRequestContract
export type RegisterRequest = RegisterRequestContract
export type AuthResponse = Omit<AuthResponseContract, 'user'> & { user: User }
export type RefreshTokenResponse = RefreshTokenResponseContract

function unwrapRequired<T>(value: T | ApiResponse<T>, fallback: T): T {
  return unwrapApiData(value, fallback)
}

// 登录
export function login(data: LoginRequest): Promise<AuthResponse> {
  const fallback: AuthResponse = { expires_at: 0, refresh_expires_at: 0, user: {} as User }
  return request<AuthResponse | ApiResponse<AuthResponse>>({
    url: '/api/v1/auth/login',
    method: 'post',
    data
  }).then((res) => unwrapRequired(res, fallback))
}

// 注册
export function register(data: RegisterRequest): Promise<AuthResponse> {
  const fallback: AuthResponse = { expires_at: 0, refresh_expires_at: 0, user: {} as User }
  return request<AuthResponse | ApiResponse<AuthResponse>>({
    url: '/api/v1/auth/register',
    method: 'post',
    data
  }).then((res) => unwrapRequired(res, fallback))
}

// 发送密码重置邮箱验证码
export function sendPasswordResetCode(data: SendPasswordResetCodeRequest): Promise<{ message: string }> {
  return request({
    url: '/api/v1/auth/password/reset-code/send',
    method: 'post',
    data
  })
}

// 通过邮箱验证码重置密码
export function resetPassword(data: ResetPasswordRequest): Promise<{ message: string }> {
  return request({
    url: '/api/v1/auth/password/reset',
    method: 'post',
    data
  })
}

// 刷新Token
export function refreshToken(): Promise<RefreshTokenResponse> {
  const fallback: RefreshTokenResponse = { expires_at: 0, refresh_expires_at: 0 }
  return request<RefreshTokenResponse | ApiResponse<RefreshTokenResponse>>({
    url: '/api/v1/auth/refresh',
    method: 'post'
  }).then((res) => unwrapRequired(res, fallback))
}

// 退出登录
export function logout(): Promise<{ message: string }> {
  return request({
    url: '/api/v1/auth/logout',
    method: 'post'
  })
}

// 获取当前用户信息
export function getCurrentUser(): Promise<User> {
  return request<User | ApiResponse<User>>({
    url: '/api/v1/auth/me',
    method: 'get',
    // /me 用于启动时探测 Cookie 会话；未登录时 401 是正常结果，不应触发全局错误提示。
    silentAuthError: true
  }).then((res) => unwrapRequired(res, {} as User))
}
