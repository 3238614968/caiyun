<template>
  <el-popover
    placement="bottom-end"
    :width="400"
    trigger="click"
    popper-class="notification-popover"
  >
    <template #reference>
      <el-badge :value="unreadCount" :hidden="unreadCount === 0" class="notification-badge">
        <el-button type="text" class="header-btn">
          <el-icon :size="20"><Bell /></el-icon>
        </el-button>
      </el-badge>
    </template>

    <div class="notification-center">
      <div class="notification-header">
        <span class="header-title">通知中心</span>
        <div class="header-actions">
          <el-button type="text" size="small" @click="markAllRead" v-if="unreadCount > 0">
            全部已读
          </el-button>
          <el-button type="text" size="small" @click="clearAll">
            清空
          </el-button>
        </div>
      </div>

      <div class="notification-list">
        <div
          v-for="notification in notifications"
          :key="notification.id"
          class="notification-item"
          :class="{ unread: !notification.read }"
          @click="handleNotificationClick(notification)"
        >
          <div class="notification-icon" :class="notification.level">
            <el-icon>
              <component :is="getNotificationIcon(notification.level)" />
            </el-icon>
          </div>
          <div class="notification-content">
            <div class="notification-title">{{ notification.title }}</div>
            <div class="notification-message">{{ notification.message }}</div>
            <div class="notification-time">{{ formatTime(notification.timestamp) }}</div>
          </div>
          <div v-if="!notification.read" class="notification-dot"></div>
        </div>

        <el-empty v-if="notifications.length === 0" description="暂无通知" :image-size="80" />
      </div>

      <div class="notification-footer">
        <el-button type="text" @click="viewAllNotifications">查看全部</el-button>
      </div>
    </div>
  </el-popover>

  <el-dialog v-model="historyVisible" title="通知历史" width="720px">
    <el-table :data="notifications" border stripe max-height="500">
      <el-table-column label="时间" width="170">
        <template #default="{ row }">
          {{ new Date(row.timestamp).toLocaleString('zh-CN') }}
        </template>
      </el-table-column>
      <el-table-column prop="title" label="标题" min-width="160" />
      <el-table-column prop="message" label="内容" min-width="260" show-overflow-tooltip />
      <el-table-column label="级别" width="100">
        <template #default="{ row }">
          <el-tag :type="row.level === 'error' ? 'danger' : row.level">
            {{ row.level }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <el-tag :type="row.read ? 'info' : 'primary'">{{ row.read ? '已读' : '未读' }}</el-tag>
        </template>
      </el-table-column>
    </el-table>
    <template #footer>
      <el-button @click="historyVisible = false">关闭</el-button>
      <el-button type="primary" @click="markAllRead" :disabled="unreadCount === 0">全部标记已读</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { Bell, CircleCheck, Warning, InfoFilled, Close } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { wsClient, type WsMessage } from '../api/websocket'

interface Notification {
  id: string
  level: 'success' | 'error' | 'warning' | 'info'
  title: string
  message: string
  timestamp: string
  read: boolean
  account_id?: number
  task_type?: string
}

const notifications = ref<Notification[]>([])
const historyVisible = ref(false)
const STORAGE_KEY = 'caiyun_notifications'

// 未读数量
const unreadCount = computed(() => {
  return notifications.value.filter(n => !n.read).length
})

// 获取通知图标
const getNotificationIcon = (level: string) => {
  const icons = {
    success: CircleCheck,
    error: Close,
    warning: Warning,
    info: InfoFilled
  }
  return icons[level as keyof typeof icons] || InfoFilled
}

// 格式化时间
const formatTime = (timestamp: string) => {
  const date = new Date(timestamp)
  const now = new Date()
  const diff = now.getTime() - date.getTime()
  const minutes = Math.floor(diff / 60000)
  const hours = Math.floor(diff / 3600000)
  const days = Math.floor(diff / 86400000)

  if (minutes < 1) return '刚刚'
  if (minutes < 60) return `${minutes}分钟前`
  if (hours < 24) return `${hours}小时前`
  if (days < 7) return `${days}天前`
  return date.toLocaleDateString('zh-CN')
}

// 标记全部已读
const markAllRead = () => {
  notifications.value.forEach(n => {
    n.read = true
  })
  ElMessage.success('已标记全部为已读')
}

// 清空通知
const clearAll = () => {
  notifications.value = []
  ElMessage.success('已清空所有通知')
}

// 点击通知
const handleNotificationClick = (notification: Notification) => {
  notification.read = true
}

// 查看全部通知
const viewAllNotifications = () => {
  historyVisible.value = true
}

const handleTaskSummary = (msg: WsMessage) => {
  const data = msg.data
  const phone = data.phone ? ` [${data.phone}]` : ''
  const gained = data.total_gained > 0 ? `，获得 ${data.total_gained} 云朵` : ''

  notifications.value.unshift({
    id: `summary_${Date.now()}_${data.account_id}`,
    level: 'info',
    title: `任务汇总${phone}`,
    message: `共执行 ${data.task_count} 个任务${gained}，当前云朵: ${data.cloud_count}`,
    timestamp: new Date().toISOString(),
    read: false,
    account_id: data.account_id
  })

  if (notifications.value.length > 50) {
    notifications.value = notifications.value.slice(0, 50)
  }
}

onMounted(() => {
  try {
    const cached = localStorage.getItem(STORAGE_KEY)
    if (cached) {
      const parsed = JSON.parse(cached)
      if (Array.isArray(parsed)) {
        notifications.value = parsed.slice(0, 200)
      }
    }
  } catch (error) {
    console.warn('[通知中心] 读取本地缓存失败', error)
  }

  // 只监听任务汇总通知，不监听单个任务完成通知
  wsClient.on('task_summary', handleTaskSummary)
})

onUnmounted(() => {
  wsClient.off('task_summary', handleTaskSummary)
})

watch(
  notifications,
  (value) => {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(value.slice(0, 200)))
    } catch (error) {
      console.warn('[通知中心] 保存本地缓存失败', error)
    }
  },
  { deep: true }
)

// 暴露方法供外部调用
defineExpose({
  addNotification: (notification: Omit<Notification, 'id' | 'read'>) => {
    notifications.value.unshift({
      ...notification,
      id: Date.now().toString(),
      read: false
    })
    if (notifications.value.length > 50) {
      notifications.value = notifications.value.slice(0, 50)
    }
  }
})
</script>

<style scoped>
.notification-badge {
  cursor: pointer;
}

.header-btn {
  color: #666;
  padding: 8px;
  border-radius: 8px;
  transition: all 0.3s;
}

.header-btn:hover {
  background: rgba(59, 130, 246, 0.1);
  color: #3b82f6;
}

.notification-center {
  max-height: 500px;
  display: flex;
  flex-direction: column;
}

.notification-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px;
  border-bottom: 1px solid #e4e7ed;
}

.header-title {
  font-size: 16px;
  font-weight: 600;
  color: #333;
}

.header-actions {
  display: flex;
  gap: 8px;
}

.notification-list {
  flex: 1;
  overflow-y: auto;
  max-height: 400px;
}

.notification-item {
  display: flex;
  padding: 12px 16px;
  border-bottom: 1px solid #f0f0f0;
  cursor: pointer;
  transition: all 0.3s;
  position: relative;
}

.notification-item:hover {
  background: rgba(59, 130, 246, 0.05);
}

.notification-item.unread {
  background: rgba(59, 130, 246, 0.08);
}

.notification-icon {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-right: 12px;
  flex-shrink: 0;
}

.notification-icon.success {
  background: linear-gradient(135deg, #10b981 0%, #34d399 100%);
  color: #fff;
}

.notification-icon.error {
  background: linear-gradient(135deg, #ef4444 0%, #f87171 100%);
  color: #fff;
}

.notification-icon.warning {
  background: linear-gradient(135deg, #f59e0b 0%, #fbbf24 100%);
  color: #fff;
}

.notification-icon.info {
  background: linear-gradient(135deg, #3b82f6 0%, #0ea5e9 100%);
  color: #fff;
}

.notification-content {
  flex: 1;
  min-width: 0;
}

.notification-title {
  font-size: 14px;
  font-weight: 600;
  color: #333;
  margin-bottom: 4px;
}

.notification-message {
  font-size: 13px;
  color: #666;
  margin-bottom: 4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.notification-time {
  font-size: 12px;
  color: #999;
}

.notification-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #f56c6c;
  position: absolute;
  top: 16px;
  right: 16px;
}

.notification-footer {
  padding: 12px 16px;
  border-top: 1px solid #e4e7ed;
  text-align: center;
}

.notification-list::-webkit-scrollbar {
  width: 6px;
}

.notification-list::-webkit-scrollbar-thumb {
  background: #c0c4cc;
  border-radius: 3px;
}

.notification-list::-webkit-scrollbar-track {
  background: transparent;
}
</style>

<style>
.notification-popover {
  padding: 0 !important;
  border-radius: 12px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.1);
}
</style>
