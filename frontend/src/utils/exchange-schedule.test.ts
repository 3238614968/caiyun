import { expect, it } from 'vitest'
import { normalizeExchangeSchedule } from './exchange-schedule'

it('does not let a hidden reservation preset override an edited single time', () => {
  expect(normalizeExchangeSchedule({ task_type: 'long_term', scheduled_exchange_time: '17:51:00', restock_times: ['10:30:00'], custom_cron: '' })).toEqual({ scheduled_exchange_time: '17:51:00', restock_times: ['17:51:00'], custom_cron: undefined })
})

it('sends one authoritative schedule for multiple times and cron', () => {
  const form = { task_type: 'long_term', scheduled_exchange_time: '17:51:00', restock_times: ['10:30', '16:00'], custom_cron: '' }
  expect(normalizeExchangeSchedule(form).scheduled_exchange_time).toBeUndefined()
  expect(normalizeExchangeSchedule({ ...form, custom_cron: '30 10 * * 5' })).toEqual({ scheduled_exchange_time: undefined, restock_times: undefined, custom_cron: '30 10 * * 5' })
})

it('does not submit invisible recurrence options for a fixed task', () => {
  expect(normalizeExchangeSchedule({ task_type: 'fixed', scheduled_exchange_time: '17:51:00', restock_times: ['10:30'], custom_cron: '30 10 * * 5' })).toEqual({ scheduled_exchange_time: '17:51:00', restock_times: undefined, custom_cron: undefined })
})
