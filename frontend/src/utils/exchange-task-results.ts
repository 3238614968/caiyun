import type { Account } from '@/api/account'
import type { CreateExchangeTaskItemResult, CreateExchangeTaskRequest, ExchangeRule } from '@/api/exchange'

export interface BatchCreateTaskResultRow {
  key: string
  success: boolean
  icon: string
  targetTypeLabel: string
  targetLabel: string
  message: string
}

export interface BatchCreateTaskResultDisplay {
  rows: BatchCreateTaskResultRow[]
  summary: {
    total: number
    success: number
    failed: number
  }
}

function uniqueNumbers(values: number[]): number[] {
  return Array.from(new Set(values.filter(Boolean)))
}

function formatAccountLabel(account?: Partial<Account> | null): string {
  if (!account) return ''
  if (account.remark && account.phone) return `${account.remark} (${account.phone})`
  return account.remark || account.phone || ''
}

function formatRuleLabel(rule?: Partial<ExchangeRule> | null): string {
  if (!rule) return ''
  if (rule.remark && rule.phone) return `${rule.remark} (${rule.phone})`
  return rule.remark || rule.phone || ''
}

export function resolveCreateTaskResultTargetLabel(
  item: CreateExchangeTaskItemResult,
  options: { userAccounts?: Account[]; rules?: ExchangeRule[] } = {}
): { typeLabel: string; targetLabel: string } {
  const typeLabel = item.target_type === 'cloud_account' ? '云盘账号' : '抢兑规则'

  if (item.target_type === 'cloud_account') {
    const account = options.userAccounts?.find((candidate) => candidate.id === item.target_id)
    const targetLabel = formatAccountLabel(account) || `${typeLabel} #${item.target_id}`
    return { typeLabel, targetLabel }
  }

  const rule = options.rules?.find((candidate) => candidate.id === item.target_id)
  const targetLabel = formatRuleLabel(rule) || `${typeLabel} #${item.target_id}`
  return { typeLabel, targetLabel }
}

export function buildCreateTaskResultDisplay(
  results: CreateExchangeTaskItemResult[],
  options: { userAccounts?: Account[]; rules?: ExchangeRule[] } = {}
): BatchCreateTaskResultDisplay {
  const rows = results.map((item) => {
    const { typeLabel, targetLabel } = resolveCreateTaskResultTargetLabel(item, options)
    return {
      key: `${item.target_type}-${item.target_id}`,
      success: item.success,
      icon: item.success ? '✅' : '❌',
      targetTypeLabel: typeLabel,
      targetLabel,
      message: item.message || (item.success ? '创建成功' : '创建失败')
    }
  })

  const success = rows.filter((row) => row.success).length
  return {
    rows,
    summary: {
      total: rows.length,
      success,
      failed: rows.length - success
    }
  }
}

export function buildRetryCreateTaskPayload(
  basePayload: CreateExchangeTaskRequest | null | undefined,
  results: CreateExchangeTaskItemResult[]
): CreateExchangeTaskRequest | null {
  if (!basePayload) return null

  const failedCloudAccountIds = uniqueNumbers(
    results.filter((item) => !item.success && item.target_type === 'cloud_account').map((item) => item.target_id)
  )
  const failedExchangeRuleIds = uniqueNumbers(
    results.filter((item) => !item.success && item.target_type === 'exchange_rule').map((item) => item.target_id)
  )

  if (failedCloudAccountIds.length === 0 && failedExchangeRuleIds.length === 0) {
    return null
  }

  return {
    ...basePayload,
    account_ids: failedCloudAccountIds.length > 0 ? failedCloudAccountIds : undefined,
    account_id: failedCloudAccountIds[0] ?? null,
    exchange_rule_ids: failedExchangeRuleIds.length > 0 ? failedExchangeRuleIds : undefined,
    exchange_rule_id: failedExchangeRuleIds[0] ?? null,
    exchange_account_ids: failedExchangeRuleIds.length > 0 ? failedExchangeRuleIds : undefined,
    exchange_account_id: failedExchangeRuleIds[0] ?? null
  }
}
