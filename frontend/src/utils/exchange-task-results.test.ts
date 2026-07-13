import { describe, expect, it } from 'vitest'

import { buildCreateTaskResultDisplay, buildRetryCreateTaskPayload, resolveCreateTaskResultTargetLabel } from './exchange-task-results'

describe('exchange-task-results', () => {
  it('resolves human friendly labels for cloud accounts and exchange rules', () => {
    expect(resolveCreateTaskResultTargetLabel(
      { target_type: 'cloud_account', target_id: 2, success: true, message: '创建成功' },
      { userAccounts: [{ id: 2, remark: '上海', phone: '13391221213' } as any] }
    )).toEqual({
      typeLabel: '云盘账号',
      targetLabel: '上海 (13391221213)'
    })

    expect(resolveCreateTaskResultTargetLabel(
      { target_type: 'exchange_rule', target_id: 8, success: false, message: '已存在' },
      { rules: [{ id: 8, remark: '工作号', phone: '15000000000' } as any] }
    )).toEqual({
      typeLabel: '抢兑规则',
      targetLabel: '工作号 (15000000000)'
    })
  })

  it('builds summary counts and fallback labels', () => {
    const display = buildCreateTaskResultDisplay([
      { target_type: 'cloud_account', target_id: 1, success: true, message: '创建成功' },
      { target_type: 'exchange_rule', target_id: 5, success: false, message: '账号未启用' }
    ])

    expect(display.summary).toEqual({ total: 2, success: 1, failed: 1 })
    expect(display.rows[0]).toMatchObject({
      targetTypeLabel: '云盘账号',
      targetLabel: '云盘账号 #1'
    })
    expect(display.rows[1]).toMatchObject({
      targetTypeLabel: '抢兑规则',
      targetLabel: '抢兑规则 #5',
      message: '账号未启用'
    })
  })

  it('builds retry payload from failed cloud accounts and exchange rules', () => {
    const payload = buildRetryCreateTaskPayload({
      product_id: 100,
      task_type: 'long_term',
      max_attempts: 10,
      account_ids: [1, 2],
      account_id: 1,
      exchange_rule_ids: [7, 8],
      exchange_rule_id: 7,
      exchange_account_ids: [7, 8],
      exchange_account_id: 7,
      restock_cycle: 'weekly'
    }, [
      { target_type: 'cloud_account', target_id: 1, success: true, message: '创建成功' },
      { target_type: 'cloud_account', target_id: 2, success: false, message: '账号停用' },
      { target_type: 'exchange_rule', target_id: 8, success: false, message: '规则不存在' }
    ])

    expect(payload).toEqual({
      product_id: 100,
      task_type: 'long_term',
      max_attempts: 10,
      account_ids: [2],
      account_id: 2,
      exchange_rule_ids: [8],
      exchange_rule_id: 8,
      exchange_account_ids: [8],
      exchange_account_id: 8,
      restock_cycle: 'weekly'
    })
  })

  it('returns null when there are no failed results to retry', () => {
    const payload = buildRetryCreateTaskPayload({ product_id: 100 }, [
      { target_type: 'cloud_account', target_id: 1, success: true, message: '创建成功' }
    ])

    expect(payload).toBeNull()
  })
})
