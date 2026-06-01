<template>
  <div class="dashboard-container">
    <el-row :gutter="20">
      <!-- 统计卡片 -->
      <el-col :span="6" v-for="stat in stats" :key="stat.key">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-content">
            <div class="stat-icon" :style="{ background: stat.color }">

              <el-icon><component :is="stat.icon" /></el-icon>
            </div>
            <div class="stat-info">
              <div class="stat-value">{{ stat.value }}</div>
              <div class="stat-label">{{ stat.label }}</div>
              <div v-if="stat.diff !== 0" class="stat-diff" :class="stat.diff > 0 ? 'positive' : 'negative'">
                <el-icon><component :is="stat.diff > 0 ? 'ArrowUp' : 'ArrowDown'" /></el-icon>
                {{ Math.abs(stat.diff) }}
              </div>
            </div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="20" style="margin-top: 20px">
      <!-- 首页公告列表 -->
      <el-col :span="24">
        <el-card shadow="hover" class="announcement-card">
          <template #header>
            <div class="card-header">
              <div class="announcement-title">
                <el-icon><Bell /></el-icon>
                <span>公告</span>
                <el-tag v-if="unreadAnnouncementCount > 0" type="danger" size="small">
                  {{ unreadAnnouncementCount }} 条未读
                </el-tag>
              </div>
              <el-button
                v-if="announcements.length > 0"
                type="primary"
                size="small"
                class="announcement-read-all-btn"
                @click="markAllAnnouncementsRead"
              >
                全部标为已读
              </el-button>
            </div>
          </template>

          <div v-loading="announcementLoading" class="announcement-list">
            <el-empty v-if="!announcementLoading && announcements.length === 0" description="暂无公告" :image-size="80" />
            <div
              v-for="item in announcements"
              :key="item.id"
              class="announcement-item"
              :class="{ unread: !isAnnouncementRead(item.id), top: item.is_top }"
              @click="openAnnouncement(item)"
            >
              <div class="announcement-main">
                <div class="announcement-line">
                  <el-tag v-if="item.is_top" type="danger" size="small">置顶</el-tag>
                  <el-tag v-if="item.is_popup" type="warning" size="small">弹窗</el-tag>
                  <el-tag v-if="!isAnnouncementRead(item.id)" type="success" size="small">未读</el-tag>
                  <span class="announcement-name">{{ item.title }}</span>
                </div>
                <div class="announcement-preview">{{ item.content }}</div>
              </div>
              <div class="announcement-date">{{ formatDateTime(item.created_at) }}</div>
            </div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="20" style="margin-top: 20px">
      <!-- 趋势图 -->
      <el-col :span="16">
        <el-card shadow="hover" class="chart-card">
          <template #header>
            <div class="card-header">
              <span>云朵趋势</span>
              <el-radio-group v-model="trendDays" size="small" @change="loadTrendData">
                <el-radio-button :label="7">7天</el-radio-button>
                <el-radio-button :label="14">14天</el-radio-button>
                <el-radio-button :label="30">30天</el-radio-button>
              </el-radio-group>
            </div>
          </template>
          <div ref="trendChartRef" style="height: 350px"></div>
        </el-card>
      </el-col>

      <!-- 账号排名 -->
      <el-col :span="8">
        <el-card shadow="hover" class="chart-card">
          <template #header>
            <div class="card-header">
              <span>{{ isAdmin ? '全局账号云朵排名' : '账号云朵排名' }}</span>
            </div>
          </template>
          <div class="ranking-wrapper">
            <el-table :data="topRanking" stripe size="small" style="width: 100%" class="ranking-table">
              <el-table-column type="index" label="#" width="40" align="center" />
              <el-table-column label="手机号" min-width="100">
                <template #default="{ row }">
                  <span class="nowrap">{{ maskPhone(row.phone) }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="remark" label="备注" min-width="60" show-overflow-tooltip />
              <el-table-column prop="cloud_count" label="云朵" width="65" align="right" />
              <el-table-column v-if="isAdmin" label="今日" width="60" align="right">
                <template #default="{ row }">
                  <span v-if="row.today_gained > 0" style="color: #10b981">+{{ row.today_gained }}</span>
                  <span v-else style="color: #999">0</span>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-row v-if="isAdmin" :gutter="20" style="margin-top: 20px">
      <!-- 任务状态监控（仅管理员可见） -->
      <el-col :span="24">
        <TaskStatusMonitor />
      </el-col>
    </el-row>

    <el-dialog
      v-model="announcementDetailVisible"
      title="公告详情"
      width="560px"
      class="announcement-detail-dialog"
    >
      <div v-if="currentAnnouncement" class="announcement-detail">
        <div class="announcement-detail-title">
          {{ currentAnnouncement.title }}
          <el-tag v-if="currentAnnouncement.is_top" type="danger" size="small">置顶</el-tag>
          <el-tag v-if="currentAnnouncement.is_popup" type="warning" size="small">弹窗</el-tag>
        </div>
        <div class="announcement-detail-time">
          发布时间：{{ formatDateTime(currentAnnouncement.created_at) }}
        </div>
        <div class="announcement-detail-content">{{ currentAnnouncement.content }}</div>
      </div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, onUnmounted, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { getDashboard, getTrendData, type DashboardData } from '../api/task'
import TaskStatusMonitor from '../components/TaskStatusMonitor.vue'
import { wsClient } from '../api/websocket'
import { getAdminDashboard, type AdminDashboardData } from '../api/account'
import { useAuthStore } from '../store/auth'
import { getAnnouncements, type Announcement } from '../api/announcement'
import * as echarts from 'echarts'

const authStore = useAuthStore()
const isAdmin = computed(() => authStore.user?.role === 'admin')

const trendChartRef = ref<HTMLElement>()
const trendChart = ref<echarts.ECharts>()
const trendDays = ref(7)
const announcements = ref<Announcement[]>([])
const announcementLoading = ref(false)
const announcementDetailVisible = ref(false)
const currentAnnouncement = ref<Announcement | null>(null)
const readAnnouncementIDs = ref<number[]>([])

// Admin dashboard data
const adminData = reactive<AdminDashboardData>({
  total_cloud: 0,
  account_count: 0,
  user_count: 0,
  today_gained: 0,
  yesterday_gained: 0,
  success_rate: 0,
  account_ranking: []
})

const dashboardData = reactive<DashboardData>({
  total_cloud: 0,
  account_count: 0,
  today_gained: 0,
  yesterday_diff: 0,
  week_diff: 0,
  success_rate: 0,
  trend_data: [],
  account_ranking: []
})

const stats = ref([
  {
    key: 'total_cloud',
    label: '当前云朵数',
    value: 0,
    diff: 0,
    icon: 'Cloudy',
    color: 'linear-gradient(135deg, #3b82f6 0%, #0ea5e9 100%)'
  },
  {
    key: 'account_count',
    label: '账号数',
    value: 0,
    diff: 0,
    icon: 'User',
    color: 'linear-gradient(135deg, #10b981 0%, #34d399 100%)'
  },
  {
    key: 'today_gained',
    label: '今日变化',
    value: 0,
    diff: 0,
    icon: 'TrendCharts',
    color: 'linear-gradient(135deg, #f59e0b 0%, #fbbf24 100%)'
  },
  {
    key: 'success_rate',
    label: '今日成功率',
    value: '0%',
    diff: 0,
    icon: 'CircleCheck',
    color: 'linear-gradient(135deg, #ef4444 0%, #f87171 100%)'
  }
])

const accountRanking = ref<any[]>([])

// 只显示前10名
const topRanking = computed(() => accountRanking.value.slice(0, 10))
const readAnnouncementStorageKey = computed(() => `readAnnouncements:${authStore.user?.id || 'guest'}`)
const unreadAnnouncementCount = computed(() => {
  return announcements.value.filter(item => !isAnnouncementRead(item.id)).length
})

// 手机号脱敏：前三后四
const maskPhone = (phone: string) => {
  if (!phone || phone.length < 7) return phone
  return phone.slice(0, 3) + '****' + phone.slice(-4)
}

const formatTrendDate = (value: string, short = false) => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }

  const year = date.getFullYear()
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')

  return short ? `${month}-${day}` : `${year}-${month}-${day}`
}

const formatCloudCount = (value: number | string) => {
  const count = Number(value)
  if (Number.isNaN(count)) {
    return String(value)
  }
  return count.toLocaleString('zh-CN')
}

const formatDateTime = (value: string) => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })
}

const loadNumberArrayFromStorage = (key: string) => {
  const rawValue = localStorage.getItem(key)
  if (!rawValue) return []

  try {
    const parsedValue = JSON.parse(rawValue)
    if (!Array.isArray(parsedValue)) {
      localStorage.removeItem(key)
      return []
    }
    return parsedValue
      .map(item => Number(item))
      .filter(item => Number.isInteger(item) && item > 0)
  } catch {
    localStorage.removeItem(key)
    return []
  }
}

const loadReadAnnouncementIDs = () => {
  const legacyDismissed = loadNumberArrayFromStorage('dismissedAnnouncements')
  const userRead = loadNumberArrayFromStorage(readAnnouncementStorageKey.value)
  readAnnouncementIDs.value = Array.from(new Set([...legacyDismissed, ...userRead]))
}

const persistReadAnnouncementIDs = () => {
  localStorage.setItem(readAnnouncementStorageKey.value, JSON.stringify(readAnnouncementIDs.value))
}

const isAnnouncementRead = (id: number) => readAnnouncementIDs.value.includes(id)

const markAnnouncementRead = (id: number) => {
  if (!readAnnouncementIDs.value.includes(id)) {
    readAnnouncementIDs.value.push(id)
    persistReadAnnouncementIDs()
  }
}

const markAllAnnouncementsRead = () => {
  const allIDs = announcements.value.map(item => item.id)
  readAnnouncementIDs.value = Array.from(new Set([...readAnnouncementIDs.value, ...allIDs]))
  persistReadAnnouncementIDs()
  ElMessage.success('已全部标为已读')
}

const openAnnouncement = (announcement: Announcement) => {
  currentAnnouncement.value = announcement
  announcementDetailVisible.value = true
  markAnnouncementRead(announcement.id)
}

const loadAnnouncements = async () => {
  announcementLoading.value = true
  try {
    const res: any = await getAnnouncements()
    announcements.value = res.announcements || []
  } catch (error) {
    ElMessage.error('加载公告失败')
  } finally {
    announcementLoading.value = false
  }
}

const loadDashboardData = async () => {
  try {
    const data = await getDashboard()
    Object.assign(dashboardData, data)

    if (isAdmin.value) {
      // Admin: load global data
      const ad = await getAdminDashboard()
      Object.assign(adminData, ad)
      stats.value[0].value = ad.total_cloud
      stats.value[0].diff = ad.today_gained - ad.yesterday_gained
      stats.value[1].value = ad.account_count
      stats.value[2].value = ad.today_gained
      stats.value[2].diff = 0
      stats.value[3].value = ad.success_rate.toFixed(1) + '%'
      // Use admin ranking
      accountRanking.value = ad.account_ranking.map(r => ({
        phone: r.phone,
        remark: r.remark || r.owner_username,
        cloud_count: r.cloud_count,
        today_gained: r.today_gained
      }))
    } else {
      // Normal user
      stats.value[0].value = data.total_cloud
      stats.value[0].diff = data.yesterday_diff
      stats.value[1].value = data.account_count
      stats.value[2].value = data.today_gained
      stats.value[2].diff = 0
      stats.value[3].value = data.success_rate.toFixed(1) + '%'
      accountRanking.value = data.account_ranking
    }
  } catch (error) {
    ElMessage.error('加载仪表盘数据失败')
  }
}

// 加载趋势数据
const loadTrendData = async () => {
  try {
    const { trend_data } = await getTrendData(trendDays.value)
    dashboardData.trend_data = trend_data
    renderTrendChart()
  } catch (error) {
    ElMessage.error('加载趋势数据失败')
  }
}

// 渲染趋势图
const renderTrendChart = () => {
  if (!trendChartRef.value) return

  if (!trendChart.value) {
    trendChart.value = echarts.init(trendChartRef.value)
  }

  const option = {
    grid: {
      top: 32,
      right: 20,
      bottom: 32,
      left: 72,
      containLabel: false
    },
    tooltip: {
      trigger: 'axis',
      triggerOn: 'mousemove|click',
      confine: true,
      backgroundColor: 'rgba(255, 255, 255, 0.96)',
      borderColor: 'rgba(219, 234, 254, 0.95)',
      borderWidth: 1,
      padding: 0,
      textStyle: {
        color: '#0f172a'
      },
      extraCssText: 'box-shadow: 0 14px 36px rgba(15, 23, 42, 0.14); border-radius: 16px;',
      axisPointer: {
        type: 'line',
        snap: true,
        lineStyle: {
          color: 'rgba(148, 163, 184, 0.85)',
          type: 'dashed',
          width: 1
        }
      },
      formatter: (params: any) => {
        const point = Array.isArray(params) ? params[0] : params
        const dateLabel = formatTrendDate(String(point?.axisValue ?? ''), false)
        const countLabel = formatCloudCount(point?.data ?? 0)

        return `
          <div style="padding: 12px 14px; min-width: 132px;">
            <div style="font-size: 14px; font-weight: 600; color: #475569; margin-bottom: 10px;">${dateLabel}</div>
            <div style="display: flex; align-items: center; justify-content: space-between; gap: 12px; font-size: 14px; color: #1e293b;">
              <span style="display: inline-flex; align-items: center; gap: 8px; color: #475569;">
                <span style="width: 12px; height: 12px; border-radius: 999px; background: #3b82f6; box-shadow: 0 0 0 4px rgba(59, 130, 246, 0.14);"></span>
                云朵数
              </span>
              <strong style="font-size: 24px; color: #0f172a;">${countLabel}</strong>
            </div>
          </div>
        `
      }
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: dashboardData.trend_data.map(item => item.date),
      axisLine: {
        lineStyle: {
          color: 'rgba(148, 163, 184, 0.55)'
        }
      },
      axisTick: {
        show: false
      },
      axisLabel: {
        color: '#64748b',
        formatter: (value: string) => formatTrendDate(String(value), true)
      }
    },
    yAxis: {
      type: 'value',
      name: '云朵数',
      nameTextStyle: {
        color: '#64748b',
        padding: [0, 0, 6, 0]
      },
      axisLabel: {
        color: '#64748b',
        formatter: (value: number) => formatCloudCount(value)
      },
      splitLine: {
        lineStyle: {
          color: 'rgba(148, 163, 184, 0.18)'
        }
      }
    },
    series: [
      {
        name: '云朵数',
        type: 'line',
        data: dashboardData.trend_data.map(item => item.cloud_count),
        smooth: true,
        showSymbol: true,
        symbol: 'circle',
        symbolSize: 8,
        lineStyle: {
          width: 4,
          color: '#3b82f6'
        },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(59, 130, 246, 0.36)' },
            { offset: 1, color: 'rgba(59, 130, 246, 0.08)' }
          ])
        },
        itemStyle: {
          color: '#ffffff',
          borderColor: '#3b82f6',
          borderWidth: 3
        },
        emphasis: {
          focus: 'series',
          scale: true,
          itemStyle: {
            color: '#3b82f6',
            borderColor: '#ffffff',
            borderWidth: 4,
            shadowBlur: 16,
            shadowColor: 'rgba(59, 130, 246, 0.28)'
          }
        }
      }
    ]
  }

  trendChart.value.setOption(option)
}

// 窗口大小变化时重新渲染图表
const handleResize = () => {
  trendChart.value?.resize()
}

// WebSocket推送：任务汇总到达时自动刷新仪表盘数据
const handleSummaryRefresh = () => {
  // 延迟1秒刷新，等数据库写入完成
  setTimeout(() => {
    loadDashboardData()
    loadTrendData()
  }, 1000)
}

onMounted(() => {
  loadReadAnnouncementIDs()
  loadDashboardData()
  loadTrendData()
  loadAnnouncements()
  window.addEventListener('resize', handleResize)
  wsClient.on('task_summary', handleSummaryRefresh)
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
  trendChart.value?.dispose()
  wsClient.off('task_summary', handleSummaryRefresh)
})
</script>

<style scoped>
.dashboard-container {
  padding: 20px;
  background: transparent;
  min-height: calc(100vh - 140px);
}

.stat-card {
  margin-bottom: 20px;
  border-radius: 16px;
  box-shadow: 0 8px 32px rgba(59, 130, 246, 0.1);
  border: 1px solid rgba(255, 255, 255, 0.6);
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  transition: all 0.3s ease;
}

.stat-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 12px 40px rgba(59, 130, 246, 0.15);
  background: rgba(255, 255, 255, 0.85);
}

.stat-content {
  display: flex;
  align-items: center;
}

.stat-icon {
  width: 60px;
  height: 60px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-right: 16px;
  box-shadow: 0 4px 15px rgba(0, 0, 0, 0.1);
}

.stat-icon .el-icon {
  font-size: 32px;
  color: white;
}

.stat-info {
  flex: 1;
}

.stat-value {
  font-size: 28px;
  font-weight: 800;
  color: #1d4ed8;
  margin-bottom: 4px;
  letter-spacing: 0.2px;
}

.stat-label {
  font-size: 14px;
  color: #2563eb;
  font-weight: 600;
  margin-bottom: 4px;
}

.stat-diff {
  font-size: 12px;
  display: flex;
  align-items: center;
}

.stat-diff.positive {
  color: #10b981;
}

.stat-diff.negative {
  color: #ef4444;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  color: #1e40af;
  font-weight: 600;
}

:deep(.el-button--primary) {
  background: linear-gradient(135deg, #3b82f6 0%, #0ea5e9 50%, #06b6d4 100%);
  border: none;
  box-shadow: 0 4px 15px rgba(59, 130, 246, 0.35);
  border-radius: 10px;
}

:deep(.el-button--primary:hover) {
  background: linear-gradient(135deg, #2563eb 0%, #0284c7 100%);
  box-shadow: 0 6px 20px rgba(59, 130, 246, 0.45);
  transform: translateY(-2px);
}

:deep(.el-button--success) {
  background: linear-gradient(135deg, #10b981 0%, #34d399 100%);
  border: none;
  box-shadow: 0 4px 15px rgba(16, 185, 129, 0.35);
  border-radius: 10px;
}

:deep(.el-button--success:hover) {
  background: linear-gradient(135deg, #059669 0%, #10b981 100%);
  box-shadow: 0 6px 20px rgba(16, 185, 129, 0.45);
  transform: translateY(-2px);
}

:deep(.el-card) {
  border-radius: 16px;
  box-shadow: 0 8px 32px rgba(59, 130, 246, 0.1);
  border: 1px solid rgba(255, 255, 255, 0.6);
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
}

:deep(.el-radio-button__original-radio:checked + .el-radio-button__inner) {
  background: linear-gradient(135deg, #3b82f6 0%, #0ea5e9 100%);
  border-color: #3b82f6;
}

.ranking-table {
  font-size: 13px;
}

.ranking-table .nowrap {
  white-space: nowrap;
}

:deep(.ranking-table .el-table__cell) {
  padding: 6px 0;
}

.chart-card {
  height: 100%;
}

.chart-card :deep(.el-card__body) {
  padding-bottom: 12px;
}

.ranking-wrapper {
  height: 350px;
  overflow-y: auto;
}

.announcement-card :deep(.el-card__body) {
  padding: 0;
}

.announcement-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: #1e3a8a;
  font-size: 18px;
  font-weight: 700;
}

.announcement-read-all-btn {
  color: #ffffff;
  font-weight: 700;
  background: #2563eb;
  border-color: #2563eb;
  box-shadow: 0 8px 18px rgba(37, 99, 235, 0.24);
}

.announcement-read-all-btn:hover,
.announcement-read-all-btn:focus {
  color: #ffffff;
  background: #1d4ed8;
  border-color: #1d4ed8;
}

.announcement-list {
  min-height: 96px;
  max-height: 280px;
  overflow-y: auto;
}

.announcement-item {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 18px;
  border-bottom: 1px solid rgba(148, 163, 184, 0.16);
  cursor: pointer;
  transition: background 0.2s ease, transform 0.2s ease;
}

.announcement-item:last-child {
  border-bottom: none;
}

.announcement-item:hover {
  background: rgba(239, 246, 255, 0.72);
}

.announcement-item.unread {
  background: rgba(236, 253, 245, 0.52);
}

.announcement-item.top {
  border-left: 3px solid #ef4444;
}

.announcement-main {
  min-width: 0;
  flex: 1;
}

.announcement-line {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.announcement-name {
  font-size: 15px;
  font-weight: 700;
  color: #1e3a8a;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.announcement-preview {
  margin-top: 6px;
  color: #334155;
  font-size: 14px;
  font-weight: 500;
  line-height: 1.55;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.announcement-date {
  flex-shrink: 0;
  color: #475569;
  font-size: 13px;
  font-weight: 500;
  white-space: nowrap;
  padding-top: 2px;
}

.announcement-detail-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 20px;
  font-weight: 700;
  color: #1e3a8a;
  margin-bottom: 10px;
}

.announcement-detail-time {
  color: #94a3b8;
  font-size: 13px;
  margin-bottom: 18px;
}

.announcement-detail-content {
  white-space: pre-wrap;
  line-height: 1.8;
  color: #334155;
}
</style>
