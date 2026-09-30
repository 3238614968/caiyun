// Collapse event bursts and allow at most one in-flight refresh.
export function createCoalescedRefresh(refresh: () => Promise<unknown>, delay = 750) {
  let timer: ReturnType<typeof setTimeout> | null = null
  let running: Promise<unknown> | null = null
  let pending = false
  let disposed = false
  let firstEventAt = 0

  const trigger = () => {
    if (disposed) return
    if (!firstEventAt) firstEventAt = Date.now()
    if (timer) clearTimeout(timer)
    const remaining = Math.max(0, 3000 - (Date.now() - firstEventAt))
    timer = setTimeout(() => { timer = null; firstEventAt = 0; void run() }, Math.min(delay, remaining))
  }
  const run = (): Promise<unknown> => {
    if (disposed) return Promise.resolve()
    if (timer) { clearTimeout(timer); timer = null; firstEventAt = 0 }
    if (running) { pending = true; return running }
    running = Promise.resolve().then(refresh).finally(() => {
      running = null
      if (pending && !disposed) { pending = false; trigger() }
    })
    return running
  }
  return {
    trigger,
    run,
    dispose() { disposed = true; pending = false; if (timer) clearTimeout(timer); timer = null }
  }
}
