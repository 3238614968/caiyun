import request from './axios'
import { unwrapApiData, unwrapOperationResponse, type ApiResponse, type OperationResponse } from './response'
import { operationHeaders } from './operation'
import type {
  Account as AccountContract,
  AccountList as AccountListContract,
  CreateAccountRequest as CreateAccountRequestContract,
  UpdateAccountRequest as UpdateAccountRequestContract,
  AdminAccountRank as AdminAccountRankContract,
  AdminAccountSearch as AdminAccountSearchContract,
  AdminAccountSearchList as AdminAccountSearchListContract,
  AdminAccountSummary as AdminAccountSummaryContract,
  AdminAccountSummaryList as AdminAccountSummaryListContract,
  AdminDashboardData as AdminDashboardDataContract,
  AdminStatsOverview as AdminStatsOverviewContract,
  AdminUser as AdminUserContract,
  AdminUserList as AdminUserListContract,
  ResetUserPasswordRequest as ResetUserPasswordRequestContract,
  SmsLoginRequest as SmsLoginRequestContract,
  SmsSendRequest as SmsSendRequestContract,
  SmsSendResult as SmsSendResultContract,
  SmsStatus as SmsStatusContract,
  TaskConfig as TaskConfigContract,
  TaskConfigList as TaskConfigListContract,
  UpdateAccountStatusRequest as UpdateAccountStatusRequestContract,
  UpdateTaskConfigRequest as UpdateTaskConfigRequestContract,
  UpdateUserRoleRequest as UpdateUserRoleRequestContract
} from './generated/operation-contract'

// 契约由本地 OpenAPI 生成；敏感认证字段只可出现在写入 DTO，绝不出现在 Account 响应中。
export type Account = AccountContract
export type CreateAccountRequest = CreateAccountRequestContract
export type UpdateAccountRequest = UpdateAccountRequestContract
export type AccountListResponse = AccountListContract

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
    url: '/api/v1/accounts',
    method: 'get',
    params
  }).then((res) => unwrapApiData(res, fallback))
}

// 获取账号详情
export function getAccount(id: number): Promise<Account> {
  return request<Account | ApiResponse<Account>>({
    url: `/api/v1/accounts/${id}`,
    method: 'get'
  }).then(unwrapAccount)
}

// 创建账号
export function createAccount(data: CreateAccountRequest): Promise<Account> {
  return request<Account | ApiResponse<Account>>({
    url: '/api/v1/accounts',
    method: 'post',
    data
  }).then(unwrapAccount)
}

// 更新账号
export function updateAccount(id: number, data: UpdateAccountRequest): Promise<Account> {
  return request<Account | ApiResponse<Account>>({
    url: `/api/v1/accounts/${id}`,
    method: 'put',
    data
  }).then(unwrapAccount)
}

// 删除账号
export function deleteAccount(id: number): Promise<{ message: string }> {
  return request({
    url: `/api/v1/accounts/${id}`,
    method: 'delete'
  })
}

// 设置账号状态
export function setAccountStatus(id: number, isActive: boolean): Promise<{ message: string }> {
  return request({
    url: `/api/v1/accounts/${id}/status`,
    method: 'put',
    params: { is_active: isActive }
  })
}

// 刷新账号Token
export function refreshAccountToken(id: number): Promise<Account> {
  return request<Account | ApiResponse<Account>>({
    url: `/api/v1/accounts/${id}/refresh`,
    method: 'post'
  }).then(unwrapAccount)
}

// 触发账号任务
export function triggerAccountTask(id: number): Promise<OperationResponse> {
  return request<OperationResponse | ApiResponse<OperationResponse>>({ url: `/api/v1/accounts/${id}/trigger`, method: 'post', headers: operationHeaders() }).then(unwrapOperationResponse)
}

// 短信验证码 DTO 由 OpenAPI 生成；响应保留统一 API 信封以携带 code/message。
export type SmsSendResponse = ApiResponse<SmsSendResultContract>
export type SmsStatusResponse = ApiResponse<SmsStatusContract>
export type SmsLoginRequest = SmsLoginRequestContract

// 发送短信验证码
export function sendSmsCode(phone: string): Promise<SmsSendResponse> {
  const data: SmsSendRequestContract = { phone }
  return request<SmsSendResponse>({
    url: '/api/v1/accounts/sms/send',
    method: 'post',
    data
  })
}

// 查询验证码发送状态
export function getSmsStatus(taskId: string): Promise<SmsStatusResponse> {
  return request<SmsStatusResponse>({
    url: `/api/v1/accounts/sms/status/${encodeURIComponent(taskId)}`,
    method: 'get'
  })
}

// 短信验证码登录（创建账号）

export function smsLogin(data: SmsLoginRequest): Promise<Account> {
  return request<Account | ApiResponse<Account>>({
    url: '/api/v1/accounts/sms/verify',
    method: 'post',
    data
  }).then(unwrapAccount)
}

// ==================== 管理员API ====================

// 管理员 DTO 均以生成契约为准；页面仅使用其稳定字段子集。
export type AdminUser = AdminUserContract
export type UserListResponse = AdminUserListContract
export type AccountSearchItem = AdminAccountSearchContract
export type SearchAllAccountsResponse = AdminAccountSearchListContract
export type StatsOverview = AdminStatsOverviewContract
export type AccountSummary = AdminAccountSummaryContract
export type AccountSummariesResponse = AdminAccountSummaryListContract
export type AdminDashboardData = AdminDashboardDataContract
export type AdminAccountRank = AdminAccountRankContract
export type TaskConfig = TaskConfigContract
export type TaskConfigListResponse = TaskConfigListContract
export type AdminRole = UpdateUserRoleRequestContract['role']

// 获取所有用户（管理员）
export function getAllUsers(page: number = 1, size: number = 10, keyword: string = ''): Promise<UserListResponse> {
  const fallback: UserListResponse = { users: [], total: 0, page, size }
  return request<UserListResponse | ApiResponse<UserListResponse>>({
    url: '/api/v1/admin/users',
    method: 'get',
    params: { page, size, ...(keyword.trim() ? { keyword: keyword.trim() } : {}) }
  }).then((res) => unwrapApiData(res, fallback))
}

// 获取所有账号（管理员）
export function getAllAccounts(page: number = 1, pageSize: number = 10, phone: string = ''): Promise<AccountListResponse> {
  const fallback: AccountListResponse = { accounts: [], total: 0, page, page_size: pageSize }
  return request<AccountListResponse | ApiResponse<AccountListResponse>>({
    url: '/api/v1/admin/accounts',
    method: 'get',
    params: { page, page_size: pageSize, ...(phone.trim() ? { phone: phone.trim() } : {}) }
  }).then((res) => unwrapApiData(res, fallback))
}

// 搜索所有账号（管理员）
export function searchAllAccounts(keyword: string, limit: number = 20): Promise<SearchAllAccountsResponse> {
  const fallback: SearchAllAccountsResponse = { accounts: [] }
  return request<SearchAllAccountsResponse | ApiResponse<SearchAllAccountsResponse>>({
    url: '/api/v1/admin/accounts/search',
    method: 'get',
    params: { keyword, limit }
  }).then((res) => unwrapApiData(res, fallback))
}

// 更新用户角色（管理员）
export function updateUserRole(id: number, role: UpdateUserRoleRequestContract['role']): Promise<{ message: string }> {
  const data: UpdateUserRoleRequestContract = { role }
  return request({
    url: `/api/v1/admin/users/${id}/role`,
    method: 'put',
    data
  })
}

// 管理员重置用户密码
export function resetUserPassword(id: number, password: string): Promise<{ message: string }> {
  const data: ResetUserPasswordRequestContract = { password }
  return request({
    url: `/api/v1/admin/users/${id}/password`,
    method: 'put',
    data
  })
}

// 更新账号状态（管理员）
export function updateAccountStatus(id: number, isActive: boolean): Promise<{ message: string }> {
  const data: UpdateAccountStatusRequestContract = { is_active: isActive }
  return request({
    url: `/api/v1/admin/accounts/${id}/status`,
    method: 'put',
    data
  })
}

// 删除用户（管理员）
export function deleteUser(id: number): Promise<{ message: string }> {
  return request({
    url: `/api/v1/admin/users/${id}`,
    method: 'delete'
  })
}

// 删除账号（管理员）
export function deleteAdminAccount(id: number): Promise<{ message: string }> {
  return request({
    url: `/api/v1/admin/accounts/${id}`,
    method: 'delete'
  })
}

export function getStatsOverview(): Promise<StatsOverview> {
  const fallback: StatsOverview = { user_count: 0, account_count: 0, total_cloud: 0, active_tasks: 0 }
  return request<StatsOverview | ApiResponse<StatsOverview>>({
    url: '/api/v1/admin/stats/overview',
    method: 'get'
  }).then((res) => unwrapApiData(res, fallback))
}

export function getAccountSummaries(page: number = 1, pageSize: number = 20): Promise<AccountSummariesResponse> {
  const fallback: AccountSummariesResponse = { summaries: [], total: 0, page, page_size: pageSize }
  return request<AccountSummariesResponse | ApiResponse<AccountSummariesResponse>>({
    url: '/api/v1/admin/accounts/summaries',
    method: 'get',
    params: { page, page_size: pageSize }
  }).then((res) => unwrapApiData(res, fallback))
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
    url: '/api/v1/admin/dashboard',
    method: 'get'
  }).then((res) => {
    const unified = unwrapApiData(res as ApiResponse<AdminDashboardData>, fallback)
    if ('total_cloud' in unified) {
      return unified
    }
    return res?.data ?? fallback
  })
}

export function getTaskConfigs(): Promise<TaskConfigListResponse> {
  const fallback: TaskConfigListResponse = { configs: [] }
  return request<TaskConfigListResponse | ApiResponse<TaskConfigListResponse>>({
    url: '/api/v1/admin/task-configs',
    method: 'get'
  }).then((res) => unwrapApiData(res, fallback))
}

export function updateTaskConfig(taskType: string, isEnabled: boolean): Promise<{ message: string }> {
  const data: UpdateTaskConfigRequestContract = { is_enabled: isEnabled }
  return request({
    url: `/api/v1/admin/task-configs/${taskType}`,
    method: 'put',
    data
  })
}

