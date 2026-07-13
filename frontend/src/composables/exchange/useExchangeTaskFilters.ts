import { computed, ref, type Ref } from 'vue'
import type { Account } from '@/api/account'
import type { ExchangeRule, ExchangeTask } from '@/api/exchange'

export interface ExchangeTaskFilterState {
  keyword: string
  status: string
  active: boolean | null
  minCloud: number | null
  maxCloud: number | null
}

type TaskRule = ExchangeRule & {
  account?: Account | null
}

export function createDefaultExchangeTaskFilters(): ExchangeTaskFilterState {
  return {
    keyword: '',
    status: '',
    active: null,
    minCloud: null,
    maxCloud: null
  }
}

export function useExchangeTaskFilters(tasks: Ref<ExchangeTask[]>) {
  const taskFilters = ref<ExchangeTaskFilterState>(createDefaultExchangeTaskFilters())

  const filteredTasks = computed(() => {
    const keyword = taskFilters.value.keyword.trim().toLowerCase()
    return tasks.value.filter((task) => {
      const rule = (task.exchange_rule || task.exchange_account) as TaskRule | undefined
      const cloudAccount = rule?.account

      if (taskFilters.value.status && task.status !== taskFilters.value.status) return false
      if (taskFilters.value.active !== null) {
        const active = rule?.is_active !== false && cloudAccount?.is_active !== false
        if (active !== taskFilters.value.active) return false
      }

      const cloudCount = Number(cloudAccount?.cloud_count ?? 0)
      if (taskFilters.value.minCloud !== null && cloudCount < taskFilters.value.minCloud) return false
      if (taskFilters.value.maxCloud !== null && cloudCount > taskFilters.value.maxCloud) return false

      if (keyword) {
        const haystack = [rule?.remark, rule?.phone, cloudAccount?.remark, cloudAccount?.phone, task.prize_name]
          .filter(Boolean)
          .join(' ')
          .toLowerCase()
        if (!haystack.includes(keyword)) return false
      }

      return true
    })
  })

  const resetTaskFilters = () => {
    taskFilters.value = createDefaultExchangeTaskFilters()
  }

  return {
    taskFilters,
    filteredTasks,
    resetTaskFilters
  }
}
