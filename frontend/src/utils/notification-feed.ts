import type { WsMessage } from '@/api/websocket'
import { formatNotificationMessage } from './notification-message'
import { formatExchangeResult } from './exchange-result'

interface AccountResult {
  phone: string
  gained: number
  miguNotes?: number
  tasks: number
  success?: number
  failed?: number
  pending?: number
  timestamp: string
}

interface ExchangeResult {
  success: boolean
  message: string
  product: string
  account: string
  timestamp: string
}

export interface Notification {
  id: string
  level: 'success' | 'error' | 'warning' | 'info'
  title: string
  message: string
  timestamp: string
  read: boolean
  account_id?: number
  task_type?: string
  operation_id?: string
  operation_attempt?: number
  operation_status?: string
  business_result?: boolean
  account_results?: Record<string, AccountResult>
  exchange_results?: Record<string, ExchangeResult>
}

const count = (value: unknown) => Math.max(0, Number.isFinite(Number(value)) ? Number(value) : 0)
const optionalCount = (value: unknown) => value === undefined ? undefined : count(value)

export const notificationTime = (event: WsMessage, value?: unknown) => {
  const timestamp = typeof event.created_at === 'number' ? event.created_at : value
  const normalized = typeof timestamp === 'string' && /^\d{4}-\d\d-\d\d \d\d:\d\d:\d\d$/.test(timestamp)
    ? timestamp.replace(' ', 'T') + '+08:00' : timestamp
  const date = new Date(typeof normalized === 'number' || typeof normalized === 'string' ? normalized : Date.now())
  return Number.isNaN(date.getTime()) ? new Date().toISOString() : date.toISOString()
}

export const mergeNotification = (feed: Notification[], incoming: Notification): Notification[] => {
  const previous = feed.find(item => item.id === incoming.id)
  if (previous && JSON.stringify(previous) === JSON.stringify(incoming)) return feed
  return [incoming, ...feed.filter(item => item.id !== incoming.id)]
    .sort((a, b) => Date.parse(b.timestamp) - Date.parse(a.timestamp)).slice(0, 50)
}

export const taskSummaryNotification = (feed: Notification[], event: WsMessage): Notification | undefined => {
  const data = event.data as Record<string, unknown>
  const operationID = String(data.operation_id || '')
  const id = operationID ? `operation_${operationID}` : `summary_${event.message_id || `${data.account_id}_${event.sequence}`}`
  const previous = feed.find(item => item.id === id)
  const attempt = count(data.operation_attempt)
  if (previous && attempt < (previous.operation_attempt || 0)) return
  const previousStatus = previous?.operation_attempt === attempt ? previous.operation_status : undefined
  const timestamp = notificationTime(event, data.completed_at)
  const results: Record<string, AccountResult> = attempt === previous?.operation_attempt ? { ...previous.account_results } : {}
  const accountKey = String(data.account_id)
  if (results[accountKey] && Date.parse(results[accountKey].timestamp) >= Date.parse(timestamp)) return
  results[accountKey] = {
    phone: String(data.phone || '').replace(/1[3-9]\d{9}/g, phone => `${phone.slice(0, 3)}****${phone.slice(-4)}`), gained: count(data.total_gained), miguNotes: count(data.total_migu_notes_gained), tasks: count(data.task_count),
    success: optionalCount(data.success_count), failed: optionalCount(data.failed_count), pending: optionalCount(data.pending_count), timestamp
  }
  const accounts = Object.values(results)
  const gained = accounts.reduce((sum, item) => sum + item.gained, 0)
  const miguNotes = accounts.reduce((sum, item) => sum + (item.miguNotes || 0), 0)
  const failed = accounts.reduce((sum, item) => sum + (item.failed || 0), 0)
  const pending = accounts.reduce((sum, item) => sum + (item.pending || 0), 0)
  const success = accounts.reduce((sum, item) => sum + (item.success || 0), 0)
  const knownOutcomes = accounts.every(item => item.success !== undefined && item.failed !== undefined && item.pending !== undefined)
  const prefix = accounts.length > 1 ? `${accounts.length} 个账号，` : ''
  const outcomes = knownOutcomes
    ? `成功 ${success} 项${failed ? `，失败 ${failed} 项` : ''}${pending ? `，待确认 ${pending} 项` : ''}`
    : `执行 ${accounts.reduce((sum, item) => sum + item.tasks, 0)} 项`
  const suffix = previousStatus === 'failed' && !failed && !pending ? '；操作未完成，请查看任务明细' : ''
  const level = failed ? (success || gained || miguNotes ? 'warning' : 'error') : pending || suffix ? 'warning' : knownOutcomes ? 'success' : 'info'
  return {
    id, level, title: accounts.length === 1 && accounts[0].phone ? `任务结果 [${accounts[0].phone}]` : '任务结果',
    message: `${prefix}已确认到账 ${gained} 云朵${miguNotes ? `、${miguNotes} 音符` : ''}，${outcomes}${suffix}`, timestamp: previous && previous.timestamp > timestamp ? previous.timestamp : timestamp,
    read: false, account_id: accounts.length === 1 ? Number(accountKey) : undefined,
    operation_id: operationID || undefined, operation_attempt: attempt,
    operation_status: previousStatus, business_result: true, account_results: results
  }
}

export const operationNotification = (feed: Notification[], event: WsMessage): Notification | undefined => {
  const data = event.data as Record<string, unknown>
  const status = String(data.status)
  // Progress belongs in the operation indicator, not in permanent history.
  if (status === 'queued' || status === 'running') return
  const operationID = String(data.operation_id || '')
  if (!operationID) return
  const id = `operation_${operationID}`
  const previous = feed.find(item => item.id === id)
  const attempt = count(data.attempt_count)
  const timestamp = notificationTime(event, data.updated_at)
  if (previous && (attempt < (previous.operation_attempt || 0) || (previous.operation_status === status && Date.parse(previous.timestamp) >= Date.parse(timestamp)))) return
  if (previous?.business_result && previous.operation_attempt === attempt) {
    const hasUnfinished = previous.level === 'warning' || previous.level === 'error'
    return {
      ...previous, operation_status: status, timestamp: previous.timestamp > timestamp ? previous.timestamp : timestamp,
      level: status === 'failed' && !hasUnfinished ? 'warning' : previous.level,
      message: status === 'failed' && !hasUnfinished ? `${previous.message}；操作未完成，请查看任务明细` : previous.message
    }
  }
  const failed = status === 'failed'
  const message = failed ? String(data.error_summary || '操作未完成，请查看任务明细') : status === 'canceled' ? '排队中的操作已取消' : '操作已完成'
  const partial = /部分任务未完成|未完成任务/.test(message)
  return {
    id, level: failed ? partial ? 'warning' : 'error' : status === 'canceled' ? 'info' : 'success',
    title: failed ? partial ? '部分任务未完成' : '操作未完成' : status === 'canceled' ? '操作已取消' : '操作已完成',
    message, timestamp, read: false, operation_id: operationID, operation_attempt: attempt, operation_status: status
  }
}

export const exchangeNotification = (feed: Notification[], event: WsMessage): Notification | undefined => {
  const data = event.data as Record<string, unknown>
  const operationID = String(data.operation_id || '')
  const id = operationID ? `operation_${operationID}` : data.task_id ? `exchange_task_${data.task_id}` : `exchange_${event.message_id || event.sequence || Date.now()}`
  const previous = feed.find(item => item.id === id)
  const attempt = count(data.operation_attempt)
  if (previous && attempt < (previous.operation_attempt || 0)) return
  const timestamp = notificationTime(event, data.completed_at)
  const skipped = event.type === 'exchange_skipped'
  const completed = event.type === 'exchange_completed' || event.type === 'monthly_exchange_complete'
  const success = data.success === true
  const message = formatNotificationMessage(String(data.message || '兑换结果待确认'), true)
  const outcome = String(data.outcome || '')
  const result = formatExchangeResult(message, success ? 'success' : 'failed', outcome)
  const level = completed ? 'info' : skipped || result.type === 'warning' ? 'warning' : success ? 'success' : 'error'
  const product = String(data.product_name || data.prize_name || '抢兑任务')
  const results: Record<string, ExchangeResult> = previous?.operation_attempt === attempt ? { ...previous.exchange_results } : {}
  const taskKey = String(data.task_id || id)
  if (results[taskKey] && Date.parse(results[taskKey].timestamp) >= Date.parse(timestamp)) return
  const maskPhone = (value: string) => value.replace(/1[3-9]\d{9}/g, phone => `${phone.slice(0, 3)}****${phone.slice(-4)}`)
  results[taskKey] = { success, message, product: maskPhone(product), account: maskPhone(String(data.account_name || '')), timestamp }
  const outcomes = Object.values(results)
  const succeeded = outcomes.filter(item => item.success).length
  const failed = outcomes.length - succeeded
  const multiple = !!operationID && outcomes.length > 1
  const unfinishedOperation = previous?.operation_attempt === attempt && previous.operation_status === 'failed' && !failed
  const detail = multiple ? `共 ${outcomes.length} 项，成功 ${succeeded} 项，未完成 ${failed} 项；请查看兑换记录` : completed ? message : `${data.account_name ? `${data.account_name} · ` : ''}${product}：${message}`
  return {
    id, level: unfinishedOperation ? 'warning' : multiple ? failed ? succeeded ? 'warning' : 'error' : 'success' : level,
    title: multiple ? '兑换结果汇总' : completed ? '兑换结果汇总' : skipped ? '兑换已跳过' : outcome === 'unknown' ? '兑换结果待核对' : success ? '兑换成功' : '兑换未完成',
    message: detail + (unfinishedOperation ? '；操作未完成，请查看兑换记录' : ''),
    timestamp: previous && previous.timestamp > timestamp ? previous.timestamp : timestamp,
    read: false, operation_id: operationID || undefined, operation_attempt: attempt, business_result: !completed, exchange_results: results,
    operation_status: previous?.operation_attempt === attempt ? previous.operation_status : undefined
  }
}

export const restoreNotifications = (feed: Notification[]) => {
  const result: Notification[] = []
  for (const item of [...feed].sort((a, b) => Date.parse(b.timestamp) - Date.parse(a.timestamp))) {
    if (['操作执行中', '操作已加入队列'].includes(item.title)) continue
    const legacyOperation = item.id.startsWith('operation_') && !item.operation_id ? item.message.match(/[（(]([a-z0-9]{8})[）)]$/i) : undefined
    let normalized = legacyOperation ? { ...item, id: `legacy_operation_${legacyOperation[1]}`, message: item.message.replace(/\s*[（(][a-z0-9]{8}[）)]$/i, '') } : item
    if (normalized.title === '操作执行失败' && normalized.message === '操作执行失败，请稍后重试') {
      normalized = { ...normalized, title: '操作未完成', message: '操作未完成，请查看任务明细' }
    }
    if (!result.some(existing => existing.id === normalized.id)) result.push(normalized)
  }
  return result.slice(0, 50)
}
