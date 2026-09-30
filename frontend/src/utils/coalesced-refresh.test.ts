import { afterEach, expect, it, vi } from 'vitest'
import { createCoalescedRefresh } from './coalesced-refresh'

afterEach(() => vi.useRealTimers())

it('collapses a websocket event burst and cancels work when unmounted', async () => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2026-09-30T08:00:00Z'))
  const load = vi.fn(async () => undefined)
  const refresh = createCoalescedRefresh(load)
  for (let i = 0; i < 100; i++) refresh.trigger()
  await vi.advanceTimersByTimeAsync(750)
  expect(load).toHaveBeenCalledTimes(1)
  refresh.trigger()
  refresh.dispose()
  await vi.advanceTimersByTimeAsync(1000)
  expect(load).toHaveBeenCalledTimes(1)
})

it('does not overlap requests and runs one pending refresh after completion', async () => {
  vi.useFakeTimers()
  let finish!: () => void
  const load = vi.fn().mockImplementationOnce(() => new Promise<void>(resolve => { finish = resolve })).mockResolvedValue(undefined)
  const refresh = createCoalescedRefresh(load, 100)
  void refresh.run()
  await Promise.resolve()
  refresh.trigger()
  await vi.advanceTimersByTimeAsync(100)
  expect(load).toHaveBeenCalledTimes(1)
  finish()
  await vi.advanceTimersByTimeAsync(100)
  expect(load).toHaveBeenCalledTimes(2)
  refresh.dispose()
})
