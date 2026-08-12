import request from './axios'
import { unwrapOperationResponse, type ApiResponse, type OperationResponse } from './response'

export function newIdempotencyKey(): string {
  if (typeof globalThis.crypto?.randomUUID === 'function') return globalThis.crypto.randomUUID()
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`
}
export function operationHeaders(key = newIdempotencyKey()): Record<string, string> { return { 'Idempotency-Key': key } }

export function operationQueuedMessage(_operation: OperationResponse, subject = '任务'): string {
  return `${subject}已加入队列`
}

export function getOperation(id: string): Promise<OperationResponse> {
  return request<OperationResponse | ApiResponse<OperationResponse>>({ url: `/api/v1/operations/${encodeURIComponent(id)}`, method: 'get' }).then(unwrapOperationResponse)
}
export function cancelOperation(id: string): Promise<OperationResponse> {
  return request<OperationResponse | ApiResponse<OperationResponse>>({ url: `/api/v1/operations/${encodeURIComponent(id)}/cancel`, method: 'post' }).then(unwrapOperationResponse)
}
