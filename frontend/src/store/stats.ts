import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  getDashboard,
  getTaskLogs,
  getCloudStats,
  getTrendData,
  triggerAllTasks,
  calculateStats,
  getTotalCloudCount,
  type DashboardData,
  type TaskLog,
  type CloudStats,
  type TrendPoint
} from '../api/task'

export const useStatsStore = defineStore('stats', () => {
  const dashboard = ref<DashboardData | null>(null)
  const taskLogs = ref<TaskLog[]>([])
  const cloudStats = ref<CloudStats[]>([])
  const trendData = ref<TrendPoint[]>([])
  const loading = ref(false)
  const totalCloud = ref(0)

  // 获取仪表盘数据
  const fetchDashboard = async () => {
    loading.value = true
    try {
      const data = await getDashboard()
      dashboard.value = data
      return data
    } finally {
      loading.value = false
    }
  }

  // 获取任务日志
  const fetchTaskLogs = async (accountId?: number, page: number = 1, pageSize: number = 20) => {
    loading.value = true
    try {
      const data = await getTaskLogs(accountId, page, pageSize)
      taskLogs.value = data.task_logs
      return data
    } finally {
      loading.value = false
    }
  }

  // 获取云朵统计
  const fetchCloudStats = async (accountId?: number, page: number = 1, pageSize: number = 10) => {
    loading.value = true
    try {
      const data = await getCloudStats(accountId, page, pageSize)
      cloudStats.value = data.cloud_stats
      return data
    } finally {
      loading.value = false
    }
  }

  // 获取趋势数据
  const fetchTrendData = async (days: number = 7) => {
    loading.value = true
    try {
      const data = await getTrendData(days)
      trendData.value = data.trend_data
      return data.trend_data
    } finally {
      loading.value = false
    }
  }

  // 触发所有账号的任务
  const triggerAll = async () => {
    loading.value = true
    try {
      const result = await triggerAllTasks()
      return result
    } finally {
      loading.value = false
    }
  }

  // 计算统计数据
  const calculate = async () => {
    loading.value = true
    try {
      const result = await calculateStats()
      return result
    } finally {
      loading.value = false
    }
  }

  // 获取总云朵数
  const fetchTotalCloudCount = async () => {
    loading.value = true
    try {
      const data = await getTotalCloudCount()
      totalCloud.value = data.total_cloud
      return data.total_cloud
    } finally {
      loading.value = false
    }
  }

  // 获取今日获得云朵数
  const getTodayGained = () => {
    return dashboard.value?.today_gained || 0
  }

  // 获取昨日对比
  const getYesterdayDiff = () => {
    return dashboard.value?.yesterday_diff || 0
  }

  // 获取上周对比
  const getWeekDiff = () => {
    return dashboard.value?.week_diff || 0
  }

  // 获取账号数
  const getAccountCount = () => {
    return dashboard.value?.account_count || 0
  }

  // 获取总云朵数
  const getTotalCloud = () => {
    return dashboard.value?.total_cloud || 0
  }

  // 获取成功率
  const getSuccessRate = () => {
    return dashboard.value?.success_rate || 0
  }

  // 获取账号排名
  const getAccountRanking = () => {
    return dashboard.value?.account_ranking || []
  }

  // 清空数据
  const clearData = () => {
    dashboard.value = null
    taskLogs.value = []
    cloudStats.value = []
    trendData.value = []
    totalCloud.value = 0
  }

  return {
    dashboard,
    taskLogs,
    cloudStats,
    trendData,
    loading,
    totalCloud,
    fetchDashboard,
    fetchTaskLogs,
    fetchCloudStats,
    fetchTrendData,
    triggerAll,
    calculate,
    fetchTotalCloudCount,
    getTodayGained,
    getYesterdayDiff,
    getWeekDiff,
    getAccountCount,
    getTotalCloud,
    getSuccessRate,
    getAccountRanking,
    clearData
  }
})
