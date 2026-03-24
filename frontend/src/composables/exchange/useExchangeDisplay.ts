import { computed, type Ref } from 'vue'

export function useExchangeDisplay(
  products: Ref<any[]>,
  tasks: Ref<any[]>,
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
      result = result.filter((p) => p.prize_name.toLowerCase().includes(keyword))
    }

    return result
  })

  const totalSuccess = computed(() => {
    return tasks.value.reduce((sum, task) => sum + (task.success_count || 0), 0)
  })

  const totalFail = computed(() => {
    return tasks.value.reduce((sum, task) => sum + (task.fail_count || 0), 0)
  })

  const getStockPercentage = (product: any) => {
    if (!product.daily_limit_count || product.daily_limit_count === 0) {
      return product.daily_remainder_count > 0 ? 50 : 0
    }
    return Math.round((product.daily_remainder_count / product.daily_limit_count) * 100)
  }

  const getStockStatus = (product: any) => {
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