import request from '../axios'
import { unwrapApiData, unwrapOperationResponse, type ApiResponse, type OperationResponse } from '../response'
import { operationHeaders } from '../operation'
import type { BatchExecuteExchangeTasksRequest, CreateExchangeTaskRequest, CreateExchangeTaskResponse, ExchangeTask, GetExchangeTasksParams, GetExchangeTasksResponse, SuccessResponse, UpdateExchangeTaskRequest } from './types'
import { normalizeCreateExchangeTaskResponse, normalizeExchangeTask } from './normalizers'

export function createExchangeTask(data: CreateExchangeTaskRequest): Promise<CreateExchangeTaskResponse> {
  const fallback: CreateExchangeTaskResponse = { task: {} as ExchangeTask, tasks: [], created: 0, errors: [], results: [] }
  return request<CreateExchangeTaskResponse | ApiResponse<CreateExchangeTaskResponse>>({
    url: '/api/v1/exchange/tasks',
    method: 'post',
    data
  }).then((res) => normalizeCreateExchangeTaskResponse(unwrapApiData(res, fallback)))
}

export function getExchangeTasks(params?: GetExchangeTasksParams): Promise<GetExchangeTasksResponse> {
  const fallback: GetExchangeTasksResponse = { tasks: [], total: 0 }
  return request<GetExchangeTasksResponse | ApiResponse<GetExchangeTasksResponse>>({
    url: '/api/v1/exchange/tasks',
    method: 'get',
    params
  }).then((res) => {
    const payload = unwrapApiData(res, fallback)
    return { ...payload, tasks: (payload.tasks || []).map(normalizeExchangeTask) }
  })
}

export function getAdminExchangeTasks(params?: GetExchangeTasksParams): Promise<GetExchangeTasksResponse> {
  const fallback: GetExchangeTasksResponse = { tasks: [], total: 0 }
  return request<GetExchangeTasksResponse | ApiResponse<GetExchangeTasksResponse>>({
    url: '/api/v1/admin/exchange/tasks',
    method: 'get',
    params
  }).then((res) => {
    const payload = unwrapApiData(res, fallback)
    return { ...payload, tasks: (payload.tasks || []).map(normalizeExchangeTask) }
  })
}

export function updateExchangeTask(id: number, data: UpdateExchangeTaskRequest): Promise<SuccessResponse> {
  return request<SuccessResponse>({
    url: `/api/v1/exchange/tasks/${id}`,
    method: 'put',
    data
  })
}

export function deleteExchangeTask(id: number): Promise<SuccessResponse> {
  return request<SuccessResponse>({
    url: `/api/v1/exchange/tasks/${id}`,
    method: 'delete'
  })
}

export function executeExchangeTask(id: number): Promise<OperationResponse> {
  return request<OperationResponse | ApiResponse<OperationResponse>>({ url: `/api/v1/exchange/tasks/${id}/execute`, method: 'post', headers: operationHeaders() }).then(unwrapOperationResponse)
}

export function batchExecuteExchangeTasks(taskIds: number[]): Promise<OperationResponse> {
  const data: BatchExecuteExchangeTasksRequest = { task_ids: taskIds }
  return request<OperationResponse | ApiResponse<OperationResponse>>({ url: '/api/v1/exchange/tasks/batch-execute', method: 'post', data, headers: operationHeaders() }).then(unwrapOperationResponse)
}
