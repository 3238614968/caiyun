import { describe, expect, it } from 'vitest'
import { ApiContractError, requireApiData, unwrapOperationResponse } from './response'

describe('API response envelope helpers', () => {
  it('rejects a successful envelope that omits operation data', () => {
    expect(() => unwrapOperationResponse({ code: 0, message: 'ok' })).toThrow(ApiContractError)
  })

  it('rejects incomplete operation payloads instead of fabricating an ID', () => {
    expect(() => unwrapOperationResponse({ code: 0, message: 'ok', data: { operation_id: '', type: 'task', status: 'queued' } as never })).toThrow('incomplete')
  })

  it('keeps explicit fallbacks available to legacy read-only callers', () => {
    expect(requireApiData({ code: 0, message: 'ok', data: { id: 1 } }, 'test')).toEqual({ id: 1 })
  })
})
