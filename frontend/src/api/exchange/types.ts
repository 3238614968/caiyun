import type {
  AddExchangeRuleRequest as AddExchangeRuleRequestContract,
  CreateExchangeTaskRequest as CreateExchangeTaskRequestContract,
  BatchExecuteExchangeTasksRequest as BatchExecuteExchangeTasksRequestContract,
  ExchangeConfig as ExchangeConfigContract,
  ExchangeConfigPublic as ExchangeConfigPublicContract,
  ExchangeRuleResponse as ExchangeRuleResponseContract,
  ExchangeRecord as ExchangeRecordContract,
  ExchangeRecordList as ExchangeRecordListContract,
  ExchangeRule as ExchangeRuleContract,
  ExchangeRuleList as ExchangeRuleListContract,
  ExchangeTask as ExchangeTaskContract,
  ExchangeTaskCreateResponse as ExchangeTaskCreateResponseContract,
  ExchangeTaskCreateResult as ExchangeTaskCreateResultContract,
  ExchangeTaskList as ExchangeTaskListContract,
  Product as ProductContract,
  ProductCategoryList as ProductCategoryListContract,
  ProductList as ProductListContract,
  RecordStats as RecordStatsContract,
  UpdateExchangeRuleRequest as UpdateExchangeRuleRequestContract,
  UpdateExchangeTaskRequest as UpdateExchangeTaskRequestContract,
  UpdateExchangeConfigRequest as UpdateExchangeConfigRequestContract,
  UpdateProductsRequest as UpdateProductsRequestContract,
  ImmediateExchangeRequest as ImmediateExchangeRequestContract
} from '../generated/operation-contract'

// The generated OpenAPI contract is the source of truth. Compatibility fields
// below isolate the historical /api response variants until their sunset date.
export type Product = ProductContract & {
  prized_name?: string
  porder?: number
  prize_image?: string
  prize_price?: number
  prize_count?: number
}

export type ExchangeRule = ExchangeRuleContract & {
  auth?: never
  token?: never
  jwt_token?: never
  product_id?: number
  product?: Product
  tasks?: ExchangeTask[]
  account?: Record<string, unknown>
}

// Historical name retained for views that have not yet moved to ExchangeRule.
export type ExchangeAccount = ExchangeRule

export type ExchangeTaskStatus = ExchangeTaskContract['status'] | 'cancelled'
export type ExchangeTaskType = ExchangeTaskContract['task_type']
export type ExchangeTask = Omit<ExchangeTaskContract, 'status' | 'exchange_account' | 'product'> & {
  status: ExchangeTaskStatus
  exchange_rule_id?: number
  exchange_rule?: ExchangeRule
  exchange_account?: ExchangeRule
  product?: Product
}

export type ExchangeRecord = Omit<ExchangeRecordContract, 'exchange_account' | 'product'> & {
  exchange_rule_id?: number
  exchange_rule?: ExchangeRule
  exchange_account?: ExchangeRule
  product?: Product
}
export type RecordStats = RecordStatsContract

export type SearchProductsResponse = Omit<ProductListContract, 'products'> & { products: Product[] }
export type GetProductCategoriesResponse = ProductCategoryListContract
export interface SuccessResponse { code?: number; message: string }
export interface UpdateProductsResponse extends SuccessResponse { data?: { account_id: number; count: number } }

export type GetExchangeRulesResponse = Omit<ExchangeRuleListContract, 'rules' | 'accounts'> & {
  rules: ExchangeRule[]
  accounts?: ExchangeRule[]
}
export type GetExchangeAccountsResponse = GetExchangeRulesResponse
export type AddExchangeRuleResponse = Omit<ExchangeRuleResponseContract, 'rule' | 'account'> & { rule: ExchangeRule; account: ExchangeRule }
export type AddExchangeAccountResponse = AddExchangeRuleResponse
export type GetExchangeTasksResponse = Omit<ExchangeTaskListContract, 'tasks'> & { tasks: ExchangeTask[] }
export type CreateExchangeTaskItemResult = Omit<ExchangeTaskCreateResultContract, 'task'> & { task?: ExchangeTask }
export type CreateExchangeTaskResponse = Omit<ExchangeTaskCreateResponseContract, 'task' | 'tasks' | 'results'> & {
  task: ExchangeTask
  tasks: ExchangeTask[]
  results: CreateExchangeTaskItemResult[]
}
export type GetExchangeRecordsResponse = Omit<ExchangeRecordListContract, 'records' | 'stats'> & {
  records: ExchangeRecord[]
  stats: RecordStats
}

export interface BatchExecuteResult { task_id: number; success: boolean; message: string }
export interface BatchExecuteExchangeTasksResponse { message: string; results: BatchExecuteResult[] }
export type ExchangeConfig = ExchangeConfigContract
export type ExchangeConfigPublic = ExchangeConfigPublicContract

export type AddExchangeRuleRequest = AddExchangeRuleRequestContract
export type AddExchangeAccountRequest = AddExchangeRuleRequest
export type UpdateExchangeRuleRequest = UpdateExchangeRuleRequestContract
export type UpdateExchangeAccountRequest = UpdateExchangeRuleRequest
export type CreateExchangeTaskRequest = CreateExchangeTaskRequestContract
export type UpdateExchangeTaskRequest = UpdateExchangeTaskRequestContract
export type UpdateProductsRequest = UpdateProductsRequestContract
export type UpdateExchangeConfigRequest = UpdateExchangeConfigRequestContract

export interface GetExchangeTasksParams {
  account_keyword?: string
  remark?: string
  status?: ExchangeTaskStatus | ''
  restock_cycle?: string
  min_cloud?: number | null
  max_cloud?: number | null
  active?: boolean | null
}
export interface GetExchangeRecordsParams {
  page?: number
  limit?: number
  account_id?: number
  product_name?: string
  status?: 'success' | 'failed'
  start_date?: string
  end_date?: string
}
export interface ExportExchangeRecordsParams {
  account_id?: number
  product_name?: string
  status?: string
  start_date?: string
  end_date?: string
  format?: 'csv' | 'json'
}
export type BatchExecuteExchangeTasksRequest = BatchExecuteExchangeTasksRequestContract
export type ImmediateExchangeRequest = ImmediateExchangeRequestContract
export interface ImmediateExchangeResponse { success: boolean; message: string }
