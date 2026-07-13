import request from '../axios'
import { unwrapOperationResponse, type ApiResponse, type OperationResponse } from '../response'
import { operationHeaders } from '../operation'
import type { ImmediateExchangeRequest } from './types'

export function immediateExchange(data: ImmediateExchangeRequest): Promise<OperationResponse> {
  return request<OperationResponse | ApiResponse<OperationResponse>>({ url: '/api/exchange/immediate', method: 'post', data, headers: operationHeaders() }).then(unwrapOperationResponse)
}
