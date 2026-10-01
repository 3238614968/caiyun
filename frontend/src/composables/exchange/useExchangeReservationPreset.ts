import type { Product } from '@/api/exchange'

export interface ExchangeReservationPreset {
  restockTimes: string[]
}

const DRINK_COUPON_KEYWORDS = ['奶茶', '饮品', '茶', '喜茶', '蜜雪']

export function isDrinkCouponProduct(product: Pick<Product, 'category' | 'prize_name'>): boolean {
  const category = product.category || ''
  const prizeName = product.prize_name || ''
  return DRINK_COUPON_KEYWORDS.some((keyword) => category.includes(keyword) || prizeName.includes(keyword))
}

export function createExchangeReservationPreset(product: Pick<Product, 'category' | 'prize_name'>): ExchangeReservationPreset {
  const exchangeTime = isDrinkCouponProduct(product) ? '10:30:00' : '10:00:00'

  return {
    restockTimes: [exchangeTime]
  }
}
