import request from './axios'

// ==================== 类型定义 ====================

// 通用成功响应。
export interface SuccessResponse {
  code?: number
  message: string
}

// 商品数据结构（与后端 `models.Product` 的 JSON 字段对齐）。
export interface Product {
  id: number
  prize_id: string
  prize_name: string
  p_order: number
  category: string
  daily_remainder_count: number
  daily_limit_count?: number
  stock_status?: string
  last_stock_check?: string
  memo?: string
  is_active?: boolean
  is_deleted?: boolean
  created_at: string
  updated_at: string

  // 兼容历史字段（前端旧代码可能仍在引用）。
  prized_name?: string
  porder?: number
  prize_image?: string
  prize_price?: number
  prize_count?: number
}

// 兑换账号数据结构。
export interface ExchangeAccount {
  id: number
  user_id: number
  account_id: number
  phone: string
  remark: string
  exchange_time_1: string
  exchange_time_2: string
  is_active: boolean
  auth?: string
  token?: string
  jwt_token?: string
  last_exchange_at?: string
  created_at: string
  updated_at: string

  // 扩展关联字段（按后端预加载情况可能存在）。
  product_id?: number
  product?: Product
  account?: Record<string, unknown>
}

// 抢兑任务状态。
export type ExchangeTaskStatus = 'pending' | 'running' | 'completed' | 'cancelled' | 'failed'

// 抢兑任务类型。
export type ExchangeTaskType = 'fixed' | 'long_term'

// 抢兑任务数据结构。
export interface ExchangeTask {
  id: number
  user_id: number
  exchange_account_id: number
  product_id: number
  prize_id: string
  prize_name: string
  task_type: ExchangeTaskType
  max_attempts: number
  attempted_count: number
  status: ExchangeTaskStatus
  last_attempt_at?: string
  last_result?: string

  // 增强字段。
  priority?: number
  task_group?: string
  timeout_seconds?: number
  max_retries?: number
  retry_count?: number
  last_retry_at?: string
  success_count?: number
  fail_count?: number

  created_at: string
  updated_at: string
  exchange_account?: ExchangeAccount
  product?: Product
}

// 抢兑记录数据结构。
export interface ExchangeRecord {
  id: number
  user_id: number
  exchange_account_id: number
  exchange_task_id?: number
  product_id: number
  prize_id: string
  prize_name: string
  status: 'success' | 'failed'
  message: string
  execution_time_ms: number
  created_at: string

  // 预加载关联字段（记录列表页会使用）。
  product?: Product
  exchange_account?: ExchangeAccount
}

// 抢兑记录统计数据。
export interface RecordStats {
  success: number
  failed: number
}

// 商品搜索响应。
export interface SearchProductsResponse {
  products: Product[]
  total: number
}

// 商品分类响应。
export interface GetProductCategoriesResponse {
  categories: string[]
}

// 手动更新商品响应。
export interface UpdateProductsResponse extends SuccessResponse {
  data?: {
    account_id: number
    count: number
  }
}

// 获取兑换账号列表响应。
export interface GetExchangeAccountsResponse {
  accounts: ExchangeAccount[]
  total: number
}

// 添加兑换账号响应。
export interface AddExchangeAccountResponse {
  account: ExchangeAccount
}

// 获取抢兑任务列表响应。
export interface GetExchangeTasksResponse {
  tasks: ExchangeTask[]
  total: number
}

// 创建抢兑任务响应。
export interface CreateExchangeTaskResponse {
  task: ExchangeTask
}

// 抢兑记录响应。
export interface GetExchangeRecordsResponse {
  records: ExchangeRecord[]
  total: number
  stats: RecordStats
}

// 批量执行单个任务结果。
export interface BatchExecuteResult {
  task_id: number
  success: boolean
  message: string
}

// 批量执行抢兑任务响应。
export interface BatchExecuteExchangeTasksResponse {
  message: string
  results: BatchExecuteResult[]
}

// 抢兑配置数据结构（管理员）。
export interface ExchangeConfig {
  auto_update_products: boolean
  concurrency: number
  enabled: boolean
  exchange_monthly_enabled: boolean
  exchange_time: string
  monthly_prize_id: string
}

// ==================== 请求参数 ====================

// 添加兑换账号请求参数。
export interface AddExchangeAccountRequest {
  account_id: number
  remark?: string
  exchange_time_1?: string
  exchange_time_2?: string

  // 当前后端暂未消费该字段，但前端表单仍保留。
  product_id?: number
}

// 更新兑换账号请求参数。
export interface UpdateExchangeAccountRequest {
  remark?: string
  exchange_time_1?: string
  exchange_time_2?: string
  is_active?: boolean
}

// 创建抢兑任务请求参数。
export interface CreateExchangeTaskRequest {
  exchange_account_id: number
  product_id: number
  task_type?: ExchangeTaskType
  max_attempts?: number
}

// 更新抢兑任务请求参数。
export interface UpdateExchangeTaskRequest {
  max_attempts?: number
}

// 更新抢兑配置请求参数。
export interface UpdateExchangeConfigRequest {
  auto_update_products: boolean
  concurrency: number
  enabled: boolean
  exchange_monthly_enabled?: boolean
  exchange_time?: string
  monthly_prize_id?: string
}

// 抢兑记录查询参数。
export interface GetExchangeRecordsParams {
  page?: number
  limit?: number
  account_id?: number
  product_name?: string
  status?: 'success' | 'failed'
  start_date?: string
  end_date?: string
}

// 导出抢兑记录参数。
export interface ExportExchangeRecordsParams {
  account_id?: number
  product_name?: string
  status?: string
  start_date?: string
  end_date?: string
  format?: 'csv' | 'json'
}

// ==================== API 函数 ====================

// 搜索商品。
export function searchProducts(keyword: string, limit?: number): Promise<SearchProductsResponse> {
  return request<SearchProductsResponse>({
    url: '/api/products/search',
    method: 'get',
    params: { keyword, limit }
  })
}

// 获取商品分类。
export function getProductCategories(): Promise<GetProductCategoriesResponse> {
  return request<GetProductCategoriesResponse>({
    url: '/api/products/categories',
    method: 'get'
  })
}

// 手动更新商品。
export function updateProducts(accountId?: number): Promise<UpdateProductsResponse> {
  return request<UpdateProductsResponse>({
    url: '/api/products/update',
    method: 'post',
    data: { account_id: accountId }
  })
}

// 获取兑换账号列表。
export function getExchangeAccounts(): Promise<GetExchangeAccountsResponse> {
  return request<GetExchangeAccountsResponse>({
    url: '/api/exchange/accounts',
    method: 'get'
  })
}

// 添加兑换账号。
export function addExchangeAccount(data: AddExchangeAccountRequest): Promise<AddExchangeAccountResponse> {
  return request<AddExchangeAccountResponse>({
    url: '/api/exchange/accounts',
    method: 'post',
    data
  })
}

// 更新兑换账号。
export function updateExchangeAccount(id: number, data: UpdateExchangeAccountRequest): Promise<SuccessResponse> {
  return request<SuccessResponse>({
    url: `/api/exchange/accounts/${id}`,
    method: 'put',
    data
  })
}

// 删除兑换账号。
export function deleteExchangeAccount(id: number): Promise<SuccessResponse> {
  return request<SuccessResponse>({
    url: `/api/exchange/accounts/${id}`,
    method: 'delete'
  })
}

// 创建抢兑任务。
export function createExchangeTask(data: CreateExchangeTaskRequest): Promise<CreateExchangeTaskResponse> {
  return request<CreateExchangeTaskResponse>({
    url: '/api/exchange/tasks',
    method: 'post',
    data
  })
}

// 获取抢兑任务列表。
export function getExchangeTasks(): Promise<GetExchangeTasksResponse> {
  return request<GetExchangeTasksResponse>({
    url: '/api/exchange/tasks',
    method: 'get'
  })
}

// 更新抢兑任务。
export function updateExchangeTask(id: number, data: UpdateExchangeTaskRequest): Promise<SuccessResponse> {
  return request<SuccessResponse>({
    url: `/api/exchange/tasks/${id}`,
    method: 'put',
    data
  })
}

// 删除抢兑任务。
export function deleteExchangeTask(id: number): Promise<SuccessResponse> {
  return request<SuccessResponse>({
    url: `/api/exchange/tasks/${id}`,
    method: 'delete'
  })
}

// 立即执行抢兑任务。
export function executeExchangeTask(id: number): Promise<SuccessResponse> {
  return request<SuccessResponse>({
    url: `/api/exchange/tasks/${id}/execute`,
    method: 'post'
  })
}

// 批量执行抢兑任务。
export function batchExecuteExchangeTasks(taskIds: number[]): Promise<BatchExecuteExchangeTasksResponse> {
  return request<BatchExecuteExchangeTasksResponse>({
    url: '/api/exchange/tasks/batch-execute',
    method: 'post',
    data: { task_ids: taskIds }
  })
}

// 获取抢兑配置（管理员）。
export function getExchangeConfig(): Promise<ExchangeConfig> {
  return request<ExchangeConfig>({
    url: '/api/admin/exchange/config',
    method: 'get'
  })
}

// 更新抢兑配置（管理员）。
export function updateExchangeConfig(data: UpdateExchangeConfigRequest): Promise<SuccessResponse> {
  return request<SuccessResponse>({
    url: '/api/admin/exchange/config',
    method: 'put',
    data
  })
}

// 立即执行月卡兑换（管理员）。
export function executeMonthlyExchange(): Promise<SuccessResponse> {
  return request<SuccessResponse>({
    url: '/api/admin/exchange/execute-monthly',
    method: 'post'
  })
}

// 查询抢兑记录。
export function getExchangeRecords(params: GetExchangeRecordsParams): Promise<GetExchangeRecordsResponse> {
  return request<GetExchangeRecordsResponse>({
    url: '/api/exchange/records',
    method: 'get',
    params
  })
}

// 导出抢兑记录。
export function exportExchangeRecords(params: ExportExchangeRecordsParams): Promise<Blob> {
  return request<Blob>({
    url: '/api/exchange/records/export',
    method: 'get',
    params,
    responseType: 'blob'
  })
}
