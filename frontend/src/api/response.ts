export interface ApiResponse<T> {
  code: number
  message: string
  data?: T
}

export function isApiResponse<T>(value: unknown): value is ApiResponse<T> {
  return !!value &&
    typeof value === 'object' &&
    'code' in value &&
    'message' in value
}

export function unwrapApiData<T>(value: T | ApiResponse<T>, fallback: T): T {
  if (isApiResponse<T>(value)) {
    return value.data ?? fallback
  }
  return value ?? fallback
}

export type OperationStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled'
export interface OperationResponse {
  operation_id: string
  type: string
  status: OperationStatus
  account_id?: number
  resource_id?: number
  attempt_count: number
  error_summary?: string
  queued_at: string
  started_at?: string
  completed_at?: string
  created_at: string
  updated_at: string
}
export function unwrapOperationResponse(value: OperationResponse | ApiResponse<OperationResponse>): OperationResponse {
  return unwrapApiData(value, { operation_id: '', type: '', status: 'queued', attempt_count: 0, queued_at: '', created_at: '', updated_at: '' })
}