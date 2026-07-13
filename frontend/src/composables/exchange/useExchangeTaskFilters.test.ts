import { describe, expect, it } from 'vitest'
import { ref } from 'vue'
import { createDefaultExchangeTaskFilters, useExchangeTaskFilters } from './useExchangeTaskFilters'

describe('useExchangeTaskFilters', () => {
  it('filters by keyword, status, account availability and cloud range', () => {
    const tasks = ref<any[]>([
      {
        id: 1,
        prize_name: '腾讯视频会员',
        status: 'pending',
        exchange_rule: {
          phone: '13300000001',
          remark: '上海主号',
          is_active: true,
          account: {
            phone: '13300000001',
            remark: '主号',
            is_active: true,
            cloud_count: 6200
          }
        }
      },
      {
        id: 2,
        prize_name: '爱奇艺会员',
        status: 'completed',
        exchange_rule: {
          phone: '15500000002',
          remark: '备用账号',
          is_active: false,
          account: {
            phone: '15500000002',
            remark: '备用',
            is_active: false,
            cloud_count: 1800
          }
        }
      }
    ])

    const { taskFilters, filteredTasks, resetTaskFilters } = useExchangeTaskFilters(tasks as any)

    taskFilters.value.keyword = '上海'
    expect(filteredTasks.value.map((task) => task.id)).toEqual([1])

    taskFilters.value = {
      ...taskFilters.value,
      keyword: '',
      status: 'completed',
      active: false
    }
    expect(filteredTasks.value.map((task) => task.id)).toEqual([2])

    taskFilters.value = {
      ...taskFilters.value,
      status: '',
      active: null,
      minCloud: 2000,
      maxCloud: 7000
    }
    expect(filteredTasks.value.map((task) => task.id)).toEqual([1])

    resetTaskFilters()
    expect(taskFilters.value).toEqual(createDefaultExchangeTaskFilters())
    expect(filteredTasks.value.map((task) => task.id)).toEqual([1, 2])
  })
})
