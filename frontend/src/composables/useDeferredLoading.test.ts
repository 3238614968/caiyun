import { effectScope, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useDeferredLoading } from './useDeferredLoading'

describe('loading feedback', () => {
  afterEach(() => vi.useRealTimers())
  it('does not flash for a fast request and shows for a slow one', () => {
    vi.useFakeTimers()
    const scope = effectScope()
    const pending = ref(false)
    const visible = scope.run(() => useDeferredLoading(pending))!
    pending.value = true
    vi.advanceTimersByTime(60)
    pending.value = false
    vi.advanceTimersByTime(200)
    expect(visible.value).toBe(false)
    pending.value = true
    vi.advanceTimersByTime(120)
    expect(visible.value).toBe(true)
    pending.value = false
    expect(visible.value).toBe(false)
    scope.stop()
  })
  it('cancels deferred feedback when the page unmounts', () => {
    vi.useFakeTimers()
    const scope = effectScope()
    const pending = ref(true)
    const visible = scope.run(() => useDeferredLoading(pending))!
    scope.stop()
    vi.advanceTimersByTime(500)
    expect(visible.value).toBe(false)
  })
})
