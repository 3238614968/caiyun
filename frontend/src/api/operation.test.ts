import { describe, expect, it } from 'vitest'
import { operationQueuedMessage } from './operation'
import type { OperationResponse } from './response'

function queuedOperation(operationId: string): OperationResponse {
  return {
    operation_id: operationId,
    type: 'account_tasks',
    status: 'queued',
    attempt_count: 0,
    queued_at: '2026-07-13T00:00:00Z',
    created_at: '2026-07-13T00:00:00Z',
    updated_at: '2026-07-13T00:00:00Z'
  }
}

describe('operationQueuedMessage', () => {
  it('shows queued semantics without exposing the operation id', () => {
    expect(operationQueuedMessage(queuedOperation('op-e2e-1')))
      .toBe('任务已加入队列')
  })

  it('does not render an empty operation id', () => {
    expect(operationQueuedMessage(queuedOperation('  '), '兑换任务'))
      .toBe('兑换任务已加入队列')
  })
})
