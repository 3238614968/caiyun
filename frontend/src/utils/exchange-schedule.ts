interface ScheduleForm {
  task_type: string
  scheduled_exchange_time: string
  restock_times: string[]
  custom_cron: string
}

export function normalizeExchangeSchedule(form: ScheduleForm) {
  const longTerm = form.task_type === 'long_term'
  const cron = longTerm ? form.custom_cron.trim() : ''
  const slots = longTerm ? [...new Set(form.restock_times.map(value => value.trim()).filter(Boolean))] : []
  if (cron) return { scheduled_exchange_time: undefined, restock_times: undefined, custom_cron: cron }
  if (slots.length > 1) return { scheduled_exchange_time: undefined, restock_times: slots, custom_cron: undefined }
  const single = String(form.scheduled_exchange_time || '').trim() || slots[0]
  return { scheduled_exchange_time: single || undefined, restock_times: slots.length && single ? [single] : undefined, custom_cron: undefined }
}
