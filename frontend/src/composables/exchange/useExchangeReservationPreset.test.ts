import { describe, expect, it } from 'vitest'
import { createExchangeReservationPreset, isDrinkCouponProduct } from './useExchangeReservationPreset'

describe('useExchangeReservationPreset', () => {
  it('prefills drink coupon time without imposing a weekly schedule', () => {
    const preset = createExchangeReservationPreset({ category: '奶茶饮品权益', prize_name: '喜茶兑换券' })

    expect(isDrinkCouponProduct({ category: '视频类会员', prize_name: '蜜雪冰城券' } as any)).toBe(true)
    expect(preset).toEqual({ restockTimes: ['10:30:00'] })
  })

  it('keeps generic products at 10:00 without calendar options', () => {
    const preset = createExchangeReservationPreset({ category: '视频类会员', prize_name: '腾讯视频会员' })

    expect(isDrinkCouponProduct({ category: '视频类会员', prize_name: '腾讯视频会员' } as any)).toBe(false)
    expect(preset).toEqual({ restockTimes: ['10:00:00'] })
  })
})
