import { describe, expect, it } from 'vitest'
import type { WsMessage } from '@/api/websocket'
import { exchangeNotification, mergeNotification, notificationTime, operationNotification, restoreNotifications, taskSummaryNotification, type Notification } from './notification-feed'
import { isActiveTaskStatus, taskLogDate, taskLogStatus } from './task-log-status'

const operation = (status: string, timestamp = '2026-11-12T01:00:02Z', attempt = 1): WsMessage => ({
  type: 'operation.updated', created_at: Date.parse(timestamp),
  data: { operation_id: 'op-one', type: 'account.task.execute', status, attempt_count: attempt, updated_at: timestamp }
})
const summary = (accountID = 1, gained = 11, attempt = 1): WsMessage => ({
  type: 'task_summary', message_id: `summary-${accountID}-${attempt}`, created_at: Date.parse('2026-11-12T01:00:01Z'),
  data: { operation_id: 'op-one', operation_attempt: attempt, account_id: accountID, phone: '13900000001', total_gained: gained, cloud_count: 12064, task_count: 14, success_count: 12, failed_count: 1, pending_count: 1 }
})
const append = (feed: Notification[], item?: Notification) => item ? mergeNotification(feed, item) : feed

describe('business notification history', () => {
  it('keeps note rewards separate and does not accumulate replayed music earnings', () => {
    const event = summary()
    event.data.total_migu_notes_gained = 40
    let feed = append([], taskSummaryNotification([], event))
    expect(feed[0].message).toContain('已确认到账 11 云朵、40 音符')
    expect(feed[0].message).not.toContain('51 云朵')
    expect(taskSummaryNotification(feed, event)).toBeUndefined()
    const second = summary(2, 5)
    second.data.total_migu_notes_gained = 25
    feed = append(feed, taskSummaryNotification(feed, second))
    expect(feed[0].message).toContain('已确认到账 16 云朵、65 音符')
    expect(taskSummaryNotification(feed, second)).toBeUndefined()
  })

  it('stores one result per operation and keeps progress out of history', () => {
    expect(operationNotification([], operation('queued'))).toBeUndefined()
    expect(operationNotification([], operation('running'))).toBeUndefined()
    let feed = append([], taskSummaryNotification([], summary()))
    feed = append(feed, operationNotification(feed, operation('failed')))
    expect(feed).toHaveLength(1)
    expect(feed[0].level).toBe('warning')
    expect(feed[0].message).toBe('已确认到账 11 云朵，成功 12 项，失败 1 项，待确认 1 项')
    expect(feed[0].message).not.toContain('12064')
    expect(JSON.stringify(feed)).not.toContain('13900000001')
  })

  it('handles durable replay in either order without showing all tasks as failed', () => {
    let feed = append([], operationNotification([], operation('failed')))
    feed = append(feed, taskSummaryNotification(feed, summary()))
    const once = JSON.stringify(feed)
    feed = append(feed, taskSummaryNotification(feed, summary()))
    feed = append(feed, operationNotification(feed, operation('failed')))
    expect(JSON.stringify(feed)).toBe(once)
    expect(feed).toHaveLength(1)
    expect(feed[0].message).toContain('失败 1 项')
    expect(feed[0].level).toBe('warning')
  })

  it('combines account results once and rejects stale retry attempts', () => {
    let feed = append([], taskSummaryNotification([], summary()))
    feed = append(feed, taskSummaryNotification(feed, summary(2, 5)))
    expect(feed).toHaveLength(1)
    expect(feed[0].message).toContain('2 个账号，已确认到账 16 云朵')
    expect(taskSummaryNotification(feed, summary(2, 5))).toBeUndefined()
    feed = append(feed, operationNotification(feed, operation('failed', '2026-11-12T01:00:03Z', 2)))
    expect(taskSummaryNotification(feed, summary(1, 11, 1))).toBeUndefined()
    expect(feed).toHaveLength(1)
  })

  it('preserves true failures and does not infer success from a finished worker', () => {
    const event = summary()
    event.data.success_count = 0
    event.data.failed_count = 14
    event.data.pending_count = 0
    event.data.total_gained = 0
    const item = taskSummaryNotification([], event)
    expect(item?.level).toBe('error')
    expect(item?.message).toContain('失败 14 项')
    const oldServer = summary()
    delete oldServer.data.success_count
    delete oldServer.data.failed_count
    delete oldServer.data.pending_count
    expect(taskSummaryNotification([], oldServer)?.level).toBe('info')
  })

  it('keeps old event times when reconnecting', () => {
    const event: WsMessage = { type: 'task_summary', data: {}, created_at: Date.parse('2026-11-10T01:00:00Z') }
    expect(notificationTime(event)).toBe('2026-11-10T01:00:00.000Z')
    expect(notificationTime({ type: 'task_summary', data: {} }, '2026-11-10 09:00:00')).toBe('2026-11-10T01:00:00.000Z')
  })

  it('merges exchange business failures with their operation result', () => {
    const event: WsMessage = { type: 'exchange_complete', created_at: Date.parse('2026-11-12T01:00:01Z'), data: { operation_id: 'op-one', operation_attempt: 1, task_id: 7, success: false, account_name: '小海', product_name: '月卡', message: '您的云朵数不足；滑块识别offset=376 confidence=0.8511' } }
    let feed = append([], exchangeNotification([], event))
    feed = append(feed, operationNotification(feed, operation('failed')))
    expect(feed).toHaveLength(1)
    expect(feed[0].level).toBe('warning')
    expect(feed[0].message).toBe('小海 · 月卡：云朵不足，兑换失败')
  })

  it('removes cached lifecycle noise and keeps genuine distinct terminal failures', () => {
    const item: Notification = { id: 'x', level: 'error', title: '操作执行失败', message: '操作执行失败，请稍后重试（60330868）', timestamp: '2026-11-12T01:00:00Z', read: true }
    const restored = restoreNotifications([
      { ...item, id: 'queued', title: '操作已加入队列' }, { ...item, id: 'running', title: '操作执行中' },
      { ...item, id: 'operation_event-a' }, { ...item, id: 'operation_event-b' },
      { ...item, id: 'operation_event-c', message: '操作执行失败，请稍后重试（923ab4f8）' }
    ])
    expect(restored).toHaveLength(2)
    expect(restored.every(item => item.read && item.level === 'error')).toBe(true)
    expect(restored.every(item => !item.message.includes('（'))).toBe(true)
  })

  it('retains failed exchanges when a later item in the same batch succeeds', () => {
    const base = { type: 'exchange_complete', data: { operation_id: 'op-one', operation_attempt: 1 } }
    let feed = append([], operationNotification([], operation('failed')))
    feed = append(feed, exchangeNotification(feed, { ...base, created_at: Date.parse('2026-11-12T01:00:00Z'), data: { ...base.data, task_id: 1, success: false, message: '云朵不足，兑换失败', product_name: '月卡' } }))
    feed = append(feed, exchangeNotification(feed, { ...base, created_at: Date.parse('2026-11-12T01:00:01Z'), data: { ...base.data, task_id: 2, success: true, message: '兑换成功', product_name: '礼券' } }))
    feed = append(feed, operationNotification(feed, operation('failed')))
    expect(feed).toHaveLength(1)
    expect(feed[0].message).toContain('共 2 项，成功 1 项，未完成 1 项')
    expect(feed[0].level).toBe('warning')
  })
})

describe('daily task log presentation', () => {
  it('treats a saved running outcome as unconfirmed and uses CST days', () => {
    expect(taskLogStatus('running')).toBe('pending')
    expect(taskLogStatus('failed')).toBe('failed')
    expect(isActiveTaskStatus({ status: 'pending', progress: 0 })).toBe(false)
    expect(isActiveTaskStatus({ status: 'running', progress: 1, end_time: '2026-11-12T01:00:00Z' })).toBe(false)
    expect(isActiveTaskStatus({ status: 'running', progress: 0.5 })).toBe(true)
    expect(taskLogDate(new Date('2026-11-11T16:00:00Z'))).toBe('2026-11-12')
    expect(taskLogDate(new Date('2026-11-11T15:59:59Z'))).toBe('2026-11-11')
  })
})
