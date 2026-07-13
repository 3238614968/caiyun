import { describe, expect, it } from 'vitest'
import { createExchangeReservationPreset, isDrinkCouponProduct } from './useExchangeReservationPreset'

describe('useExchangeReservationPreset', () => {
  const fixedNow = new Date('2026-07-10T09:00:00+08:00')

  it('prefills drink coupon products with Friday weekly restock window', () => {
    const preset = createExchangeReservationPreset({ category: '奶茶饮品权益', prize_name: '喜茶兑换券' } as any, fixedNow)

    expect(isDrinkCouponProduct({ category: '视频类会员', prize_name: '蜜雪冰城券' } as any)).toBe(true)
    expect(preset).toMatchObject({
      exchangeTime: '10:30:00',
      restockCycle: 'weekly',
      restockWeekday: 5,
      restockDayOfMonth: 10,
      restockTimes: ['10:30:00'],
      customCron: '',
      calendarPolicy: 'all'
    })
  })

  it('keeps generic products on daily default using the current weekday', () => {
    const preset = createExchangeReservationPreset({ category: '视频类会员', prize_name: '腾讯视频会员' } as any, fixedNow)

    expect(isDrinkCouponProduct({ category: '视频类会员', prize_name: '腾讯视频会员' } as any)).toBe(false)
    expect(preset).toMatchObject({
      exchangeTime: '10:00:00',
      restockCycle: 'daily',
      restockWeekday: 5,
      restockDayOfMonth: 10,
      restockTimes: ['10:00:00']
    })
  })
})
