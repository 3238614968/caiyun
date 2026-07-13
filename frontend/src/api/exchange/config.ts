import request from '../axios'
import { unwrapApiData, unwrapOperationResponse, type ApiResponse, type OperationResponse } from '../response'
import { operationHeaders } from '../operation'
import type { ExchangeConfig, SuccessResponse, UpdateExchangeConfigRequest } from './types'

export function getExchangeConfig(): Promise<ExchangeConfig> {
  const fallback: ExchangeConfig = {
    auto_update_products: false,
    concurrency: 10,
    enabled: true,
    exchange_monthly_enabled: false,
    exchange_time: '10:00',
    monthly_prize_id: '1001',
    immediate_exchange_enabled: false
  }
  return request<ExchangeConfig | ApiResponse<ExchangeConfig>>({
    url: '/api/admin/exchange/config',
    method: 'get'
  }).then((res) => unwrapApiData(res, fallback))
}

export function getExchangeConfigPublic(): Promise<{ enabled: boolean; immediate_exchange_enabled: boolean }> {
  const fallback = { enabled: true, immediate_exchange_enabled: false }
  return request<typeof fallback | ApiResponse<typeof fallback>>({
    url: '/api/exchange/config',
    method: 'get'
  }).then((res) => unwrapApiData(res, fallback))
}

export function updateExchangeConfig(data: UpdateExchangeConfigRequest): Promise<SuccessResponse> {
  return request<SuccessResponse>({
    url: '/api/admin/exchange/config',
    method: 'put',
    data
  })
}

export function executeMonthlyExchange(): Promise<OperationResponse> {
  return request<OperationResponse | ApiResponse<OperationResponse>>({ url: '/api/admin/exchange/execute-monthly', method: 'post', headers: operationHeaders() }).then(unwrapOperationResponse)
}
