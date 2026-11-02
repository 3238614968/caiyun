import { effectScope } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { useLatestRequest } from './useLatestRequest'

describe('latest list request', () => {
  it('keeps the newer result when an earlier search arrives last', async () => {
    const scope = effectScope()
    const state = scope.run(() => useLatestRequest())!
    const apply = vi.fn()
    const error = vi.fn()
    let completeOld!: (value: string) => void
    const old = state.execute(() => new Promise<string>(resolve => { completeOld = resolve }), apply, error)
    await state.execute(() => Promise.resolve('new search'), apply, error)
    completeOld('old search')
    await old
    expect(apply).toHaveBeenCalledExactlyOnceWith('new search')
    expect(error).not.toHaveBeenCalled()
    expect(state.loading.value).toBe(false)
    scope.stop()
  })

  it('does not clear the latest loading state or announce stale errors', async () => {
    const scope = effectScope()
    const state = scope.run(() => useLatestRequest())!
    const apply = vi.fn()
    const error = vi.fn()
    let rejectOld!: (value: Error) => void
    let completeNew!: (value: string) => void
    const old = state.execute(() => new Promise<string>((_, reject) => { rejectOld = reject }), apply, error)
    const current = state.execute(() => new Promise<string>(resolve => { completeNew = resolve }), apply, error)
    rejectOld(new Error('stale failure'))
    await old
    expect(state.loading.value).toBe(true)
    expect(error).not.toHaveBeenCalled()
    scope.stop()
    completeNew('arrived after leaving page')
    await current
    expect(apply).not.toHaveBeenCalled()
  })
})
