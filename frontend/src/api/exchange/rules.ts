import request from '../axios'
import { unwrapApiData, type ApiResponse } from '../response'
import type { AddExchangeRuleRequest, AddExchangeRuleResponse, ExchangeRule, GetExchangeRulesResponse, SuccessResponse, UpdateExchangeRuleRequest } from './types'
import { normalizeExchangeRule, normalizeExchangeRulesResponse } from './normalizers'

export function getExchangeRules(): Promise<GetExchangeRulesResponse> {
  const fallback: GetExchangeRulesResponse = { rules: [], accounts: [], total: 0 }
  return request<GetExchangeRulesResponse | ApiResponse<GetExchangeRulesResponse>>({
    url: '/api/exchange/rules',
    method: 'get'
  }).then((res) => normalizeExchangeRulesResponse(unwrapApiData(res, fallback)))
}

export const getExchangeAccounts = getExchangeRules

export function getAdminExchangeRules(): Promise<GetExchangeRulesResponse> {
  const fallback: GetExchangeRulesResponse = { rules: [], accounts: [], total: 0 }
  return request<GetExchangeRulesResponse | ApiResponse<GetExchangeRulesResponse>>({
    url: '/api/admin/exchange/rules',
    method: 'get'
  }).then((res) => normalizeExchangeRulesResponse(unwrapApiData(res, fallback)))
}

export function addExchangeRule(data: AddExchangeRuleRequest): Promise<AddExchangeRuleResponse> {
  const fallback: AddExchangeRuleResponse = { rule: {} as ExchangeRule, account: {} as ExchangeRule }
  return request<AddExchangeRuleResponse | ApiResponse<AddExchangeRuleResponse>>({
    url: '/api/exchange/rules',
    method: 'post',
    data
  }).then((res) => {
    const payload = unwrapApiData(res, fallback)
    const rule = normalizeExchangeRule(payload.rule || payload.account)
    return { ...payload, rule, account: rule }
  })
}

export const addExchangeAccount = addExchangeRule

export function updateExchangeRule(id: number, data: UpdateExchangeRuleRequest): Promise<SuccessResponse> {
  return request<SuccessResponse>({
    url: `/api/exchange/rules/${id}`,
    method: 'put',
    data
  })
}

export const updateExchangeAccount = updateExchangeRule

export function deleteExchangeRule(id: number): Promise<SuccessResponse> {
  return request<SuccessResponse>({
    url: `/api/exchange/rules/${id}`,
    method: 'delete'
  })
}

export const deleteExchangeAccount = deleteExchangeRule
