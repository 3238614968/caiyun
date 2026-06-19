import { describe, expect, it } from 'vitest'
import { ref } from 'vue'

import { useExchangeDisplay } from './useExchangeDisplay'

describe('useExchangeDisplay', () => {
  it('filters products by category and keyword', () => {
    const products = ref([
      { category: 'music', prize_name: 'QQ音乐绿钻会员月卡', daily_limit_count: 10, daily_remainder_count: 8 },
      { category: 'traffic', prize_name: '2GB全国流量日包', daily_limit_count: 10, daily_remainder_count: 2 },
      { category: 'music', prize_name: '酷狗会员月卡', daily_limit_count: 10, daily_remainder_count: 0 }
    ])
    const tasks = ref([])
    const currentCategory = ref('music')
    const searchKeyword = ref('qq')

    const { filteredProducts } = useExchangeDisplay(products, tasks, currentCategory, searchKeyword)

    expect(filteredProducts.value).toHaveLength(1)
    expect(filteredProducts.value[0].prize_name).toBe('QQ音乐绿钻会员月卡')
  })

  it('aggregates task success and failure counts', () => {
    const { totalSuccess, totalFail } = useExchangeDisplay(
      ref([]),
      ref([
        { success_count: 2, fail_count: 1 },
        { success_count: 3, fail_count: 4 },
        {}
      ]),
      ref(''),
      ref('')
    )

    expect(totalSuccess.value).toBe(5)
    expect(totalFail.value).toBe(5)
  })

  it('derives stock percentage and status', () => {
    const { getStockPercentage, getStockStatus } = useExchangeDisplay(ref([]), ref([]), ref(''), ref(''))

    expect(getStockPercentage({ daily_limit_count: 10, daily_remainder_count: 5 })).toBe(50)
    expect(getStockStatus({ daily_limit_count: 10, daily_remainder_count: 1 })).toBe('exception')
    expect(getStockStatus({ daily_limit_count: 10, daily_remainder_count: 4 })).toBe('warning')
    expect(getStockStatus({ daily_limit_count: 10, daily_remainder_count: 8 })).toBe('success')
    expect(getStockStatus({ stock_status: 'sold_out', daily_limit_count: 10, daily_remainder_count: 9 })).toBe('exception')
  })
})
