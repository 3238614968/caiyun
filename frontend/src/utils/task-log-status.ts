export const taskLogDate = (date = new Date()) => new Intl.DateTimeFormat('en-CA', {
  timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit'
}).format(date)

// A saved log describes an ended attempt; a pending result needs confirmation.
export const taskLogStatus = (status?: string) => {
  switch (status?.toLowerCase()) {
    case 'success': case 'succeeded': case 'completed': return 'success'
    case 'failed': case 'failure': return 'failed'
    case 'skipped': case 'canceled': case 'cancelled': return 'skipped'
    default: return 'pending'
  }
}

export const isActiveTaskStatus = (task: { status: string; progress?: number; end_time?: string }) => {
  return (task.status === 'running' || task.status === 'retrying') && (task.progress ?? 0) < 1 && !task.end_time
}
