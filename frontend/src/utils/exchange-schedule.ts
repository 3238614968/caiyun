interface ScheduleForm {
  restock_times: string[]
}

export function normalizeExchangeTime(value: string): string {
  const time = value.trim()
  if (!/^([01]\d|2[0-3]):[0-5]\d(:00)?$/.test(time)) {
    throw new Error('抢兑时间格式应为 HH:mm，例如 10:00、16:00')
  }
  return `${time.slice(0, 5)}:00`
}

export function normalizeExchangeSchedule(form: ScheduleForm) {
  const slots = [...new Set(form.restock_times.map(normalizeExchangeTime))].sort()
  if (slots.length === 0) throw new Error('请至少选择一个抢兑时间')
  if (slots.length > 100) throw new Error('抢兑时间不能超过 100 个')
  return {
    scheduled_exchange_time: slots.length === 1 ? slots[0] : undefined,
    restock_times: slots,
    restock_cycle: 'daily' as const,
    calendar_policy: 'all' as const
  }
}
