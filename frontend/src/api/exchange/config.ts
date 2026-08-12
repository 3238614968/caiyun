import request from '../axios'
import { unwrapApiData, unwrapOperationResponse, type ApiResponse, type OperationResponse } from '../response'
import { operationHeaders } from '../operation'
import type { ExchangeConfig, ExchangeConfigPublic, SuccessResponse, UpdateExchangeConfigRequest } from './types'

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
    url: '/api/v1/admin/exchange/config',
    method: 'get'
  }).then((res) => unwrapApiData(res, fallback))
}

export function getExchangeConfigPublic(): Promise<ExchangeConfigPublic> {
  const fallback: ExchangeConfigPublic = { enabled: true, immediate_exchange_enabled: false }
  return request<ExchangeConfigPublic | ApiResponse<ExchangeConfigPublic>>({
    url: '/api/v1/exchange/config',
    method: 'get'
  }).then((res) => unwrapApiData(res, fallback))
}

export function updateExchangeConfig(data: UpdateExchangeConfigRequest): Promise<SuccessResponse> {
  return request<SuccessResponse>({
    url: '/api/v1/admin/exchange/config',
    method: 'put',
    data
  })
}

export function executeMonthlyExchange(): Promise<OperationResponse> {
  return request<OperationResponse | ApiResponse<OperationResponse>>({ url: '/api/v1/admin/exchange/execute-monthly', method: 'post', headers: operationHeaders() }).then(unwrapOperationResponse)
}
