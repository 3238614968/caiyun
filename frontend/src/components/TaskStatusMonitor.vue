<template>
  <el-card shadow="hover" class="task-status-card">
    <template #header>
      <div class="card-header">
        <span>任务执行状态</span>
        <el-button type="text" size="small" @click="refreshStatus">
          <el-icon><Refresh /></el-icon>
          刷新
        </el-button>
      </div>
    </template>

    <!-- 队列状态 -->
    <div class="queue-status">
      <div class="status-item">
        <div class="status-label">队列长度</div>
        <div class="status-value queue-length">{{ queueStatus.queue_length }}</div>
      </div>
      <div class="status-item">
        <div class="status-label">活跃Worker</div>
        <div class="status-value">{{ queueStatus.active_workers }}</div>
      </div>
      <div class="status-item">
        <div class="status-label">待处理任务</div>
        <div class="status-value">{{ queueStatus.pending_tasks }}</div>
      </div>
      <div class="status-item">
        <div class="status-label">成功率</div>
        <div class="status-value success-rate">
          {{ successRate }}%
        </div>
      </div>
    </div>

    <!-- 执行中的任务 -->
    <div v-if="activeTasks.length > 0" class="active-tasks">
      <div class="section-title">执行中的任务</div>
      <div v-for="task in activeTasks" :key="`${task.account_id}_${task.task_type}`" class="task-item">
        <div class="task-info">
          <div class="task-name">{{ getTaskTypeName(task.task_type) }}</div>
          <div class="task-account">账号ID: {{ task.account_id }}</div>
        </div>
        <div class="task-progress">
          <el-progress
            :percentage="task.progress * 100"
            :status="getProgressStatus(task.status)"
            :stroke-width="8"
          />
          <div class="task-message">{{ task.message || '执行中...' }}</div>
        </div>
      </div>
    </div>

    <!-- 空状态 -->
    <el-empty v-else description="暂无执行中的任务" :image-size="100" />
  </el-card>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { getQueueStatus, getTaskStatus, type QueueStatus, type TaskStatus } from '../api/task'
import { wsClient, type WsMessage } from '../api/websocket'
import { getTaskTypeName } from '../utils/task-types'

const queueStatus = ref<QueueStatus>({
  queue_length: 0,
  active_workers: 0,
  pending_tasks: 0,
  completed_tasks: 0,
  successful_tasks: 0,
  failed_tasks: 0
})

const taskStatuses = ref<TaskStatus[]>([])

// 执行中的任务
const activeTasks = computed(() => {
  const list = Array.isArray(taskStatuses.value) ? taskStatuses.value : []
  return list.filter(
    task => task.status === 'running' || task.status === 'pending' || task.status === 'retrying'
  )
})

// 成功率
const successRate = computed(() => {
  const total = queueStatus.value.completed_tasks
  if (total === 0) return 0
  return ((queueStatus.value.successful_tasks / total) * 100).toFixed(1)
})


const getProgressStatus = (status: string) => {
  if (status === 'success') return 'success'
  if (status === 'failed') return 'exception'
  return undefined
}

// 手动刷新（保留按钮功能）
const refreshStatus = async () => {
  try {
    const [queueData, taskData] = await Promise.all([
      getQueueStatus(),
      getTaskStatus()
    ])
    queueStatus.value = queueData
    taskStatuses.value = Array.isArray(taskData) ? taskData : []
  } catch (error) {
    console.error('刷新状态失败:', error)
    taskStatuses.value = []
  }
}

// WebSocket消息处理
const handleTaskComplete = (msg: WsMessage) => {
  const data = msg.data
  // 更新或添加到任务状态列表
  const idx = taskStatuses.value.findIndex(
    t => t.account_id === data.account_id && t.task_type === data.task_type
  )
  const item: TaskStatus = {
    account_id: data.account_id,
    task_type: data.task_type,
    status: data.status,
    progress: data.status === 'success' || data.status === 'failed' ? 1 : 0.5,
    message: data.message
  }
  if (idx >= 0) {
    taskStatuses.value[idx] = item
  } else {
    taskStatuses.value.unshift(item)
    // 保留最近50条
    if (taskStatuses.value.length > 50) {
      taskStatuses.value = taskStatuses.value.slice(0, 50)
    }
  }

  // 更新计数
  queueStatus.value.completed_tasks++
  if (data.status === 'success') {
    queueStatus.value.successful_tasks++
  } else if (data.status === 'failed') {
    queueStatus.value.failed_tasks++
  }
}

const handleTaskSummary = (msg: WsMessage) => {
  // 汇总到达时，减少pending
  if (queueStatus.value.pending_tasks > 0) {
    queueStatus.value.pending_tasks--
  }
  if (queueStatus.value.queue_length > 0) {
    queueStatus.value.queue_length--
  }
}

onMounted(() => {
  // 初始加载一次
  refreshStatus()
  // 监听WebSocket推送
  wsClient.on('task_complete', handleTaskComplete)
  wsClient.on('task_summary', handleTaskSummary)
})

onUnmounted(() => {
  wsClient.off('task_complete', handleTaskComplete)
  wsClient.off('task_summary', handleTaskSummary)
})
</script>

<style scoped>
.task-status-card {
  border-radius: 16px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.5);
  background: rgba(255, 255, 255, 0.95);
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.queue-status {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 20px;
  margin-bottom: 24px;
  padding: 20px;
  background: rgba(59, 130, 246, 0.05);
  border-radius: 12px;
}

.status-item {
  text-align: center;
}

.status-label {
  font-size: 14px;
  color: #666;
  margin-bottom: 8px;
}

.status-value {
  font-size: 24px;
  font-weight: bold;
  color: #333;
}

.queue-length {
  color: #3b82f6;
}

.success-rate {
  color: #10b981;
}

.active-tasks {
  margin-top: 20px;
}

.section-title {
  font-size: 16px;
  font-weight: 600;
  color: #333;
  margin-bottom: 16px;
}

.task-item {
  padding: 16px;
  margin-bottom: 12px;
  background: rgba(255, 255, 255, 0.8);
  border-radius: 12px;
  border: 1px solid rgba(0, 0, 0, 0.05);
}

.task-info {
  display: flex;
  justify-content: space-between;
  margin-bottom: 12px;
}

.task-name {
  font-size: 15px;
  font-weight: 600;
  color: #333;
}

.task-account {
  font-size: 13px;
  color: #999;
}

.task-progress {
  margin-top: 8px;
}

.task-message {
  font-size: 12px;
  color: #666;
  margin-top: 8px;
}

:deep(.el-progress-bar__outer) {
  border-radius: 10px;
  background-color: #f0f0f0;
}

:deep(.el-progress-bar__inner) {
  border-radius: 10px;
}
</style>
