import { computed, type Ref } from 'vue'
import type { ExchangeTask, Product } from '@/api/exchange'

type DisplayProduct = Pick<Product, 'daily_remainder_count'> &
  Partial<Pick<Product, 'category' | 'prize_name' | 'daily_limit_count' | 'stock_status'>>
type DisplayTask = Partial<Pick<ExchangeTask, 'success_count' | 'fail_count'>>

export function useExchangeDisplay(
  products: Ref<DisplayProduct[]>,
  tasks: Ref<DisplayTask[]>,
  currentCategory: Ref<string>,
  searchKeyword: Ref<string>
) {
  const filteredProducts = computed(() => {
    let result = products.value

    if (currentCategory.value) {
      result = result.filter((p) => p.category === currentCategory.value)
    }

    if (searchKeyword.value) {
      const keyword = searchKeyword.value.toLowerCase()
      result = result.filter((p) => (p.prize_name || '').toLowerCase().includes(keyword))
    }

    return result
  })

  const totalSuccess = computed(() => {
    return tasks.value.reduce((sum, task) => sum + (task.success_count || 0), 0)
  })

  const totalFail = computed(() => {
    return tasks.value.reduce((sum, task) => sum + (task.fail_count || 0), 0)
  })

  const getStockPercentage = (product: DisplayProduct) => {
    if (!product.daily_limit_count || product.daily_limit_count === 0) {
      return product.daily_remainder_count > 0 ? 50 : 0
    }
    return Math.round((product.daily_remainder_count / product.daily_limit_count) * 100)
  }

  const getStockStatus = (product: DisplayProduct) => {
    if (product.stock_status === 'sold_out' || product.daily_remainder_count === 0) {
      return 'exception'
    }
    const percentage = getStockPercentage(product)
    if (percentage <= 20) return 'exception'
    if (percentage <= 50) return 'warning'
    return 'success'
  }

  return {
    filteredProducts,
    totalSuccess,
    totalFail,
    getStockPercentage,
    getStockStatus
  }
}
