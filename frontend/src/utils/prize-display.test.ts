import { expect, it } from 'vitest'
import { prizeExpiryMillis, prizeExpiresToday } from './prize-display'

it('interprets upstream expiry dates in China time instead of browser timezone', () => {
  expect(prizeExpiryMillis('2026-09-30T23:59:59')).toBe(Date.parse('2026-09-30T15:59:59Z'))
  expect(prizeExpiresToday('2026-09-30T23:59:59', Date.parse('2026-09-30T10:00:00Z'))).toBe(true)
  expect(prizeExpiresToday('2026-10-01T23:59:59', Date.parse('2026-09-30T10:00:00Z'))).toBe(false)
  expect(prizeExpiryMillis('unknown')).toBeNull()
})
