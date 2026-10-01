import { expect, it } from 'vitest'
import { normalizeExchangeSchedule, normalizeExchangeTime } from './exchange-schedule'

it('normalizes one daily slot', () => {
  expect(normalizeExchangeSchedule({ restock_times: ['17:51'] })).toEqual({
    scheduled_exchange_time: '17:51:00', restock_times: ['17:51:00'], restock_cycle: 'daily', calendar_policy: 'all'
  })
})

it.each(['fixed', 'long_term'])('supports multiple slots for %s and ignores obsolete hidden fields', (taskType) => {
  const form = { task_type: taskType, scheduled_exchange_time: '17:51:00', restock_times: ['16:00', '10:00', '16:00:00'], custom_cron: '30 10 * * 5', calendar_policy: 'workday' }
  expect(normalizeExchangeSchedule(form)).toEqual({
    scheduled_exchange_time: undefined, restock_times: ['10:00:00', '16:00:00'], restock_cycle: 'daily', calendar_policy: 'all'
  })
})

it('requires valid minute slots and accepts midnight', () => {
  expect(normalizeExchangeTime('00:00')).toBe('00:00:00')
  expect(() => normalizeExchangeSchedule({ restock_times: [] })).toThrow('至少选择')
  for (const time of ['25:00', '10:60', '10:00:05', 'invalid']) {
    expect(() => normalizeExchangeSchedule({ restock_times: [time] })).toThrow('时间格式')
  }
})
