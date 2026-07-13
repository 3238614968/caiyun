import type { Product } from '@/api/exchange'

export type ExchangeRestockCycle = 'daily' | 'weekly' | 'monthly' | 'once'

export interface ExchangeReservationPreset {
  exchangeTime: string
  restockCycle: ExchangeRestockCycle
  restockWeekday: number | null
  restockDayOfMonth: number
  restockTimes: string[]
  customCron: string
  calendarPolicy: 'all' | 'workday' | 'holiday'
}

const DRINK_COUPON_KEYWORDS = ['奶茶', '饮品', '茶', '喜茶', '蜜雪']

export function isDrinkCouponProduct(product: Pick<Product, 'category' | 'prize_name'>): boolean {
  const category = product.category || ''
  const prizeName = product.prize_name || ''
  return DRINK_COUPON_KEYWORDS.some((keyword) => category.includes(keyword) || prizeName.includes(keyword))
}

export function createExchangeReservationPreset(product: Pick<Product, 'category' | 'prize_name'>, now = new Date()): ExchangeReservationPreset {
  const exchangeTime = isDrinkCouponProduct(product) ? '10:30:00' : '10:00:00'
  const restockCycle: ExchangeRestockCycle = isDrinkCouponProduct(product) ? 'weekly' : 'daily'

  return {
    exchangeTime,
    restockCycle,
    restockWeekday: isDrinkCouponProduct(product) ? 5 : now.getDay(),
    restockDayOfMonth: now.getDate(),
    restockTimes: [exchangeTime],
    customCron: '',
    calendarPolicy: 'all'
  }
}
