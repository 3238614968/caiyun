<template>
  <div class="dashboard-container">
    <DashboardStatGrid :stats="stats" />

    <el-row
      :gutter="20"
      style="margin-top: 20px"
    >
      <!-- 首页公告列表 -->
      <el-col :span="24">
        <AnnouncementPanel
          :announcements="announcements"
          :loading="announcementLoading"
          :unread-count="unreadAnnouncementCount"
          :is-read="isAnnouncementRead"
          @mark-all-read="markAllAnnouncementsRead"
          @open="openAnnouncement"
        />
      </el-col>
    </el-row>

    <el-row
      :gutter="20"
      style="margin-top: 20px"
    >
      <!-- 趋势图 -->
      <el-col :span="16">
        <CloudTrendCard
          :days="trendDays"
          :trend-data="dashboardData.trend_data"
          :latest-summary="latestTrendSummary"
          @update:days="handleTrendDaysChange"
        />
      </el-col>

      <!-- 账号排名 -->
      <el-col :span="8">
        <AccountCloudRankingCard
          :ranking="topRanking"
          :is-admin="isAdmin"
        />
      </el-col>
    </el-row>

    <el-row
      v-if="isAdmin"
      :gutter="20"
      style="margin-top: 20px"
    >
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
      <div
        v-if="currentAnnouncement"
        class="announcement-detail"
      >
        <div class="announcement-detail-title">
          {{ currentAnnouncement.title }}
          <el-tag
            v-if="currentAnnouncement.is_top"
            type="danger"
            size="small"
          >
            置顶
          </el-tag>
          <el-tag
            v-if="currentAnnouncement.is_popup"
            type="warning"
            size="small"
          >
            弹窗
          </el-tag>
        </div>
        <div class="announcement-detail-time">
          发布时间：{{ formatDateTime(currentAnnouncement.created_at) }}
        </div>
        <div class="announcement-detail-content">
          {{ currentAnnouncement.content }}
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted, onUnmounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { getDashboard, getTrendData, type DashboardData } from '../api/task'
import AnnouncementPanel from '../components/dashboard/AnnouncementPanel.vue'
import AccountCloudRankingCard, { type AccountCloudRankingRow } from '../components/dashboard/AccountCloudRankingCard.vue'
import DashboardStatGrid, { type DashboardStatItem } from '../components/dashboard/DashboardStatGrid.vue'
import TaskStatusMonitor from '../components/TaskStatusMonitor.vue'
import { isOperationUpdatedMessage, wsClient, type WsMessage } from '../api/websocket'
import { getAdminDashboard, type AdminDashboardData } from '../api/account'
import { useAuthStore } from '../store/auth'
import { getAnnouncements, type Announcement } from '../api/announcement'

const CloudTrendCard = defineAsyncComponent(() => import('../components/dashboard/CloudTrendCard.vue'))
const authStore = useAuthStore()
const isAdmin = computed(() => authStore.user?.role === 'admin')

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

const stats = ref<DashboardStatItem[]>([
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

const accountRanking = ref<AccountCloudRankingRow[]>([])

// 只显示前10名
const topRanking = computed(() => accountRanking.value.slice(0, 10))
const readAnnouncementStorageKey = computed(() => `readAnnouncements:${authStore.user?.id || 'guest'}`)
const unreadAnnouncementCount = computed(() => {
  return announcements.value.filter(item => !isAnnouncementRead(item.id)).length
})
const latestTrendSummary = computed(() => {
  const points = dashboardData.trend_data || []
  const latest = points[points.length - 1]
  const diff = Number(latest?.cloud_diff || 0)
  return {
    cloud: latest ? formatCloudCount(latest.cloud_count) : '-',
    diff: formatTrendDiff(diff),
    diffClass: diff > 0 ? 'positive' : diff < 0 ? 'negative' : ''
  }
})

const formatCloudCount = (value: number | string) => {
  const count = Number(value)
  if (Number.isNaN(count)) {
    return String(value)
  }
  return count.toLocaleString('zh-CN')
}

const formatTrendDiff = (value?: number) => {
  const count = Number(value || 0)
  if (count === 0) return '0'
  return `${count > 0 ? '+' : ''}${formatCloudCount(count)}`
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
  } catch (error) {
    ElMessage.error('加载趋势数据失败')
  }
}

const handleTrendDaysChange = (value: number) => {
  trendDays.value = Number(value)
  loadTrendData()
}

// WebSocket推送：任务汇总到达时自动刷新仪表盘数据
const handleSummaryRefresh = () => {
  // 延迟1秒刷新，等数据库写入完成
  setTimeout(() => {
    loadDashboardData()
    loadTrendData()
  }, 1000)
}

// Operation terminal events carry no private payload and let the dashboard
// refresh immediately instead of waiting for the next manual navigation.
const handleOperationRefresh = (msg: WsMessage) => {
  if (!isOperationUpdatedMessage(msg)) return
  if (['succeeded', 'failed', 'canceled'].includes(msg.data.status)) {
    handleSummaryRefresh()
  }
}

onMounted(() => {
  loadReadAnnouncementIDs()
  loadDashboardData()
  loadTrendData()
  loadAnnouncements()
  wsClient.on('task_summary', handleSummaryRefresh)
  wsClient.on('operation.updated', handleOperationRefresh)
})

onUnmounted(() => {
  wsClient.off('task_summary', handleSummaryRefresh)
  wsClient.off('operation.updated', handleOperationRefresh)
})
</script>

<style scoped>
.dashboard-container {
  padding: 20px;
  background: transparent;
  min-height: calc(100vh - 140px);
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
