import type { CreateExchangeTaskResponse, ExchangeRecord, ExchangeRule, ExchangeTask, GetExchangeRulesResponse } from './types'

export function normalizeExchangeRule(rule?: Partial<ExchangeRule> | null): ExchangeRule {
  return { ...(rule || {}) } as ExchangeRule
}

export function normalizeExchangeRulesResponse(payload: Partial<GetExchangeRulesResponse>): GetExchangeRulesResponse {
  const rules = (payload.rules || payload.accounts || []).map(normalizeExchangeRule)
  return {
    ...payload,
    rules,
    accounts: rules,
    total: payload.total ?? rules.length
  } as GetExchangeRulesResponse
}

export function normalizeExchangeTask(task?: Partial<ExchangeTask> | null): ExchangeTask {
  const exchangeRule = task?.exchange_rule || task?.exchange_account
  const exchangeRuleId = task?.exchange_rule_id ?? task?.exchange_account_id
  const normalizedRule = exchangeRule ? normalizeExchangeRule(exchangeRule) : undefined
  return {
    ...(task || {}),
    exchange_rule_id: exchangeRuleId,
    exchange_account_id: exchangeRuleId ?? task?.exchange_account_id,
    exchange_rule: normalizedRule,
    exchange_account: normalizedRule
  } as ExchangeTask
}

export function normalizeCreateExchangeTaskResponse(payload: Partial<CreateExchangeTaskResponse>): CreateExchangeTaskResponse {
  const tasks = (payload.tasks || []).map(normalizeExchangeTask)
  const task = payload.task ? normalizeExchangeTask(payload.task) : (tasks[0] || ({} as ExchangeTask))
  return {
    ...payload,
    task,
    tasks,
    results: (payload.results || []).map((item) => ({
      ...item,
      task: item.task ? normalizeExchangeTask(item.task) : undefined
    }))
  } as CreateExchangeTaskResponse
}

export function normalizeExchangeRecord(record?: Partial<ExchangeRecord> | null): ExchangeRecord {
  const exchangeRule = record?.exchange_rule || record?.exchange_account
  const exchangeRuleId = record?.exchange_rule_id ?? record?.exchange_account_id
  const normalizedRule = exchangeRule ? normalizeExchangeRule(exchangeRule) : undefined
  return {
    ...(record || {}),
    exchange_rule_id: exchangeRuleId,
    exchange_account_id: exchangeRuleId ?? record?.exchange_account_id,
    exchange_rule: normalizedRule,
    exchange_account: normalizedRule
  } as ExchangeRecord
}
