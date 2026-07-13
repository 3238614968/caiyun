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
  image_url?: string
  prize_price?: number
  prize_count?: number
}

// 账号规则数据结构。
export interface ExchangeRule {
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
  current_product?: Product
  tasks?: ExchangeTask[]
  account?: Record<string, unknown>
}

// ExchangeAccount 是历史兼容别名，新代码请优先使用 ExchangeRule。
export type ExchangeAccount = ExchangeRule

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
  scheduled_exchange_time?: string
  restock_cycle?: 'daily' | 'weekly' | 'monthly' | 'once'
  restock_weekday?: number | null
  restock_day_of_month?: number | null
  restock_times?: string
  custom_cron?: string
  calendar_policy?: 'all' | 'workday' | 'holiday'
  holiday_dates?: string
  workday_dates?: string
  skip_reason?: string
  next_run_at?: string
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
  exchange_rule_id?: number
  exchange_rule?: ExchangeRule
  exchange_account?: ExchangeRule
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
  exchange_rule_id?: number
  exchange_rule?: ExchangeRule
  exchange_account?: ExchangeRule
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

// 获取抢兑规则列表响应。
export interface GetExchangeRulesResponse {
  rules: ExchangeRule[]
  accounts?: ExchangeRule[]
  total: number
}

// GetExchangeAccountsResponse 是历史兼容别名。
export type GetExchangeAccountsResponse = GetExchangeRulesResponse

// 添加抢兑规则响应。
export interface AddExchangeRuleResponse {
  rule: ExchangeRule
  account?: ExchangeRule
}

// AddExchangeAccountResponse 是历史兼容别名。
export type AddExchangeAccountResponse = AddExchangeRuleResponse

// 获取抢兑任务列表响应。
export interface GetExchangeTasksResponse {
  tasks: ExchangeTask[]
  total: number
}

// 创建抢兑任务响应。
export interface CreateExchangeTaskItemResult {
  target_type: 'cloud_account' | 'exchange_rule'
  target_id: number
  success: boolean
  message: string
  task?: ExchangeTask
}

export interface CreateExchangeTaskResponse {
  task: ExchangeTask
  tasks?: ExchangeTask[]
  created?: number
  errors?: string[]
  results?: CreateExchangeTaskItemResult[]
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
  immediate_exchange_enabled: boolean
}

// ==================== 请求参数 ====================

// 添加抢兑规则请求参数。
export interface AddExchangeRuleRequest {
  account_id: number
  remark?: string
  exchange_time_1?: string
  exchange_time_2?: string

  // 当前后端暂未消费该字段，但前端表单仍保留。
  product_id?: number
}

// AddExchangeAccountRequest 是历史兼容别名。
export type AddExchangeAccountRequest = AddExchangeRuleRequest

// 更新抢兑规则请求参数。
export interface UpdateExchangeRuleRequest {
  remark?: string
  exchange_time_1?: string
  exchange_time_2?: string
  is_active?: boolean
  product_id?: number
}

// UpdateExchangeAccountRequest 是历史兼容别名。
export type UpdateExchangeAccountRequest = UpdateExchangeRuleRequest

// 创建抢兑任务请求参数。
export interface CreateExchangeTaskRequest {
  exchange_rule_id?: number | null
  exchange_rule_ids?: number[]
  exchange_account_id?: number | null
  exchange_account_ids?: number[]
  account_id?: number | null
  account_ids?: number[]
  product_id: number
  task_type?: ExchangeTaskType
  max_attempts?: number
  scheduled_exchange_time?: string
  restock_cycle?: 'daily' | 'weekly' | 'monthly' | 'once'
  restock_weekday?: number | null
  restock_day_of_month?: number | null
  restock_times?: string[] | string
  custom_cron?: string
  calendar_policy?: 'all' | 'workday' | 'holiday'
  holiday_dates?: string[] | string
  workday_dates?: string[] | string
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
  immediate_exchange_enabled?: boolean
}


export interface GetExchangeTasksParams {
  account_keyword?: string
  remark?: string
  status?: ExchangeTaskStatus | ''
  restock_cycle?: string
  min_cloud?: number | null
  max_cloud?: number | null
  active?: boolean | null
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

export interface ImmediateExchangeRequest {
  exchange_rule_id?: number | null
  exchange_account_id?: number | null
  account_id?: number | null
  product_id: number
}

export interface ImmediateExchangeResponse {
  success: boolean
  message: string
}
