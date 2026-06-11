import request from './axios'
import { unwrapApiData, type ApiResponse } from './response'

// 账号接口
export interface Account {
  id: number
  user_id: number
  phone: string
  auth?: string
  token?: string
  jwt_token?: string
  platform: string
  expire_at: number
  cloud_count: number
  remark: string
  is_active: boolean
  created_at: string
  updated_at: string
  user?: {
    username: string
  }
}

// 创建账号请求
export interface CreateAccountRequest {
  phone: string
  auth: string
  remark?: string
}

// 更新账号请求
export interface UpdateAccountRequest {
  phone: string
  auth?: string
  remark?: string
}

// 账号列表响应
export interface AccountListResponse {
  accounts: Account[]
  total: number
  page: number
  page_size: number
}

function unwrapAccount(value: Account | ApiResponse<Account>): Account {
  return unwrapApiData(value, {} as Account)
}

// 获取账号列表
export function getAccounts(page: number = 1, pageSize: number = 10, phone: string = ''): Promise<AccountListResponse> {
  const params: Record<string, any> = { page, page_size: pageSize }
  if (phone) {
    params.phone = phone
  }
  const fallback: AccountListResponse = { accounts: [], total: 0, page, page_size: pageSize }
  return request<AccountListResponse | ApiResponse<AccountListResponse>>({
    url: '/api/accounts',
    method: 'get',
    params
  }).then((res) => unwrapApiData(res, fallback))
}

// 获取账号详情
export function getAccount(id: number): Promise<Account> {
  return request<Account | ApiResponse<Account>>({
    url: `/api/accounts/${id}`,
    method: 'get'
  }).then(unwrapAccount)
}

// 创建账号
export function createAccount(data: CreateAccountRequest): Promise<Account> {
  return request<Account | ApiResponse<Account>>({
    url: '/api/accounts',
    method: 'post',
    data
  }).then(unwrapAccount)
}

// 更新账号
export function updateAccount(id: number, data: UpdateAccountRequest): Promise<Account> {
  return request<Account | ApiResponse<Account>>({
    url: `/api/accounts/${id}`,
    method: 'put',
    data
  }).then(unwrapAccount)
}

// 删除账号
export function deleteAccount(id: number): Promise<{ message: string }> {
  return request({
    url: `/api/accounts/${id}`,
    method: 'delete'
  })
}

// 设置账号状态
export function setAccountStatus(id: number, isActive: boolean): Promise<{ message: string }> {
  return request({
    url: `/api/accounts/${id}/status`,
    method: 'put',
    params: { is_active: isActive }
  })
}

// 刷新账号Token
export function refreshAccountToken(id: number): Promise<Account> {
  return request<Account | ApiResponse<Account>>({
    url: `/api/accounts/${id}/refresh`,
    method: 'post'
  }).then(unwrapAccount)
}

// 触发账号任务
export function triggerAccountTask(id: number): Promise<{ message: string }> {
  return request({
    url: `/api/accounts/${id}/trigger`,
    method: 'post'
  })
}

// 发送短信验证码
export function sendSmsCode(phone: string): Promise<{ code: number; message: string; data: { phone: string; task_id: string } }> {
  return request({
    url: '/api/accounts/sms/send',
    method: 'post',
    data: { phone }
  })
}

// 查询验证码发送状态
export interface SmsStatusResponse {
  code: number
  message: string
  data: {
    phone: string
    task_id?: string
    status: string
    message?: string
    retryable?: boolean
  }
}

export function getSmsStatus(phone: string): Promise<SmsStatusResponse> {
  return request({
    url: `/api/accounts/sms/status/${phone}`,
    method: 'get'
  })
}

// 短信验证码登录（创建账号）
export interface SmsLoginRequest {
  phone: string
  sms_code: string
  task_id: string
  remark?: string
}

export function smsLogin(data: SmsLoginRequest): Promise<Account> {
  return request<Account | ApiResponse<Account>>({
    url: '/api/accounts/sms/verify',
    method: 'post',
    data
  }).then(unwrapAccount)
}

// ==================== 管理员API ====================

// 用户接口
export interface AdminUser {
  id: number
  username: string
  email: string
  role: string
  created_at: string
}

// 用户列表响应
export interface UserListResponse {
  users: AdminUser[]
  total: number
  page: number
  size: number
}

// 获取所有用户（管理员）
export function getAllUsers(page: number = 1, size: number = 10): Promise<UserListResponse> {
  const fallback: UserListResponse = { users: [], total: 0, page, size }
  return request<UserListResponse | ApiResponse<UserListResponse>>({
    url: '/api/admin/users',
    method: 'get',
    params: { page, size }
  }).then((res) => unwrapApiData(res, fallback))
}

// 获取所有账号（管理员）
export function getAllAccounts(page: number = 1, pageSize: number = 10): Promise<AccountListResponse> {
  const fallback: AccountListResponse = { accounts: [], total: 0, page, page_size: pageSize }
  return request<AccountListResponse | ApiResponse<AccountListResponse>>({
    url: '/api/admin/accounts',
    method: 'get',
    params: { page, page_size: pageSize }
  }).then((res) => unwrapApiData(res, fallback))
}

// 搜索所有账号（管理员）
export interface AccountSearchItem {
  id: number
  phone: string
  remark: string
  user_id: number
  username: string
  is_active: boolean
}

export interface SearchAllAccountsResponse {
  accounts: AccountSearchItem[]
}

export function searchAllAccounts(keyword: string, limit: number = 20): Promise<SearchAllAccountsResponse> {
  const fallback: SearchAllAccountsResponse = { accounts: [] }
  return request<SearchAllAccountsResponse | ApiResponse<SearchAllAccountsResponse>>({
    url: '/api/admin/accounts/search',
    method: 'get',
    params: { keyword, limit }
  }).then((res) => unwrapApiData(res, fallback))
}

// 更新用户角色（管理员）
export function updateUserRole(id: number, role: string): Promise<{ message: string }> {
  return request({
    url: `/api/admin/users/${id}/role`,
    method: 'put',
    data: { role }
  })
}

// 管理员重置用户密码
export function resetUserPassword(id: number, password: string): Promise<{ message: string }> {
  return request({
    url: `/api/admin/users/${id}/password`,
    method: 'put',
    data: { password }
  })
}

// 更新账号状态（管理员）
export function updateAccountStatus(id: number, isActive: boolean): Promise<{ message: string }> {
  return request({
    url: `/api/admin/accounts/${id}/status`,
    method: 'put',
    data: { is_active: isActive }
  })
}

// 删除用户（管理员）
export function deleteUser(id: number): Promise<{ message: string }> {
  return request({
    url: `/api/admin/users/${id}`,
    method: 'delete'
  })
}

// 删除账号（管理员）
export function deleteAdminAccount(id: number): Promise<{ message: string }> {
  return request({
    url: `/api/admin/accounts/${id}`,
    method: 'delete'
  })
}

// 统计概览（管理员）
export interface StatsOverview {
  user_count: number
  account_count: number
  total_cloud: number
  active_tasks: number
}

export function getStatsOverview(): Promise<StatsOverview> {
  const fallback: StatsOverview = { user_count: 0, account_count: 0, total_cloud: 0, active_tasks: 0 }
  return request<StatsOverview | ApiResponse<StatsOverview>>({
    url: '/api/admin/stats/overview',
    method: 'get'
  }).then((res) => unwrapApiData(res, fallback))
}

// 账号概况（管理员）
export interface AccountSummary {
  id: number
  phone: string
  remark: string
  owner_username: string
  cloud_count: number
  is_active: boolean
  created_at: string
  today_gained: number
  yesterday_gained: number
  success_count: number
  failed_count: number
  last_executed_at: string
}

export interface AccountSummariesResponse {
  summaries: AccountSummary[]
  total: number
  page: number
  page_size: number
}

export function getAccountSummaries(page: number = 1, pageSize: number = 20): Promise<AccountSummariesResponse> {
  const fallback: AccountSummariesResponse = { summaries: [], total: 0, page, page_size: pageSize }
  return request<AccountSummariesResponse | ApiResponse<AccountSummariesResponse>>({
    url: '/api/admin/accounts/summaries',
    method: 'get',
    params: { page, page_size: pageSize }
  }).then((res) => unwrapApiData(res, fallback))
}

// 管理员仪表盘数据
export interface AdminDashboardData {
  total_cloud: number
  account_count: number
  user_count: number
  today_gained: number
  yesterday_gained: number
  success_rate: number
  account_ranking: AdminAccountRank[]
}

export interface AdminAccountRank {
  account_id: number
  phone: string
  remark: string
  owner_username: string
  cloud_count: number
  today_gained: number
}

export function getAdminDashboard(): Promise<AdminDashboardData> {
  const fallback: AdminDashboardData = {
    total_cloud: 0,
    account_count: 0,
    user_count: 0,
    today_gained: 0,
    yesterday_gained: 0,
    success_rate: 0,
    account_ranking: []
  }

  return request<{ data: AdminDashboardData } | ApiResponse<AdminDashboardData>>({
    url: '/api/admin/dashboard',
    method: 'get'
  }).then((res) => {
    const unified = unwrapApiData(res as ApiResponse<AdminDashboardData>, fallback)
    if ('total_cloud' in unified) {
      return unified
    }
    return res?.data ?? fallback
  })
}

// 任务配置（管理员）
export interface TaskConfig {
  id: number
  task_type: string
  task_name: string
  is_enabled: boolean
  sort_order: number
  updated_at: string
}

export function getTaskConfigs(): Promise<{ configs: TaskConfig[] }> {
  const fallback = { configs: [] as TaskConfig[] }
  return request<{ configs: TaskConfig[] } | ApiResponse<{ configs: TaskConfig[] }>>({
    url: '/api/admin/task-configs',
    method: 'get'
  }).then((res) => unwrapApiData(res, fallback))
}

export function updateTaskConfig(taskType: string, isEnabled: boolean): Promise<{ message: string }> {
  return request({
    url: `/api/admin/task-configs/${taskType}`,
    method: 'put',
    data: { is_enabled: isEnabled }
  })
}

