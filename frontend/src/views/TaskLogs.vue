<template>
  <div class="task-logs-container">
    <el-card shadow="hover">
      <template #header>
        <div class="card-header">
          <span>运行日志</span>
        </div>
      </template>

      <!-- 筛选栏 -->
      <el-form :inline="true" :model="searchForm" class="search-form">
        <el-form-item label="任务类型">
          <el-select v-model="searchForm.taskType" placeholder="请选择" clearable>
            <el-option label="全部" value="" />
            <el-option label="签到" value="signin" />
            <el-option label="微信" value="wechat" />
            <el-option label="摇一摇" value="shake" />
            <el-option label="今日云朵" value="todaycloud" />
            <el-option label="AI云朵" value="aicloud" />
            <el-option label="盲盒" value="blindbox" />
            <el-option label="红包" value="redpacket" />
            <el-option label="商店" value="store" />
            <el-option label="花园" value="garden" />
          </el-select>
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="searchForm.status" placeholder="请选择" clearable>
            <el-option label="全部" value="" />
            <el-option label="成功" value="success" />
            <el-option label="失败" value="failed" />
            <el-option label="执行中" value="pending" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handleSearch">搜索</el-button>
          <el-button @click="handleReset">重置</el-button>
        </el-form-item>
      </el-form>

      <!-- 日志列表 -->
            <div class="logs-shell" v-loading="loading">
<el-table v-if="!isMobile" :data="logList" stripe style="width: 100%">
        <el-table-column prop="account.phone" label="手机号" width="150" />
        <el-table-column prop="task_type" label="任务类型" width="120">
          <template #default="{ row }">
            <el-tag>{{ getTaskTypeName(row.task_type) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)">
              {{ getStatusName(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="cloud_gained" label="获得云朵" width="100">
          <template #default="{ row }">
            <span v-if="row.cloud_gained > 0" style="color: #67c23a">+{{ row.cloud_gained }}</span>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column prop="message" label="执行结果" show-overflow-tooltip />
        <el-table-column prop="created_at" label="执行时间" width="180">
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100" fixed="right">
          <template #default="{ row }">
            <el-button type="primary" link @click="handleViewDetail(row)">详情</el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 分页 -->

        <div v-else class="mobile-log-list">
          <el-empty v-if="logList.length === 0" description="暂无日志" />
          <template v-else>
            <el-card
              v-for="row in logList"
              :key="row.id"
              class="mobile-log-card"
              shadow="never"
            >
              <div class="mobile-log-head">
                <div>
                  <div class="mobile-log-title">{{ row.account?.phone || '-' }}</div>
                  <div class="mobile-log-meta">{{ getTaskTypeName(row.task_type) }}</div>
                </div>
                <el-tag :type="getStatusType(row.status)">{{ getStatusName(row.status) }}</el-tag>
              </div>
              <div class="mobile-log-grid">
                <div class="mobile-log-row">
                  <span class="mobile-log-label">任务类型</span>
                  <span class="mobile-log-value">{{ getTaskTypeName(row.task_type) }}</span>
                </div>
                <div class="mobile-log-row">
                  <span class="mobile-log-label">获得云朵</span>
                  <span class="mobile-log-value" :class="{ 'positive': row.cloud_gained > 0 }">{{ row.cloud_gained > 0 ? `+${row.cloud_gained}` : '-' }}</span>
                </div>
                <div class="mobile-log-row">
                  <span class="mobile-log-label">执行时间</span>
                  <span class="mobile-log-value">{{ formatDate(row.created_at) }}</span>
                </div>
                <div class="mobile-log-row full">
                  <span class="mobile-log-label">执行结果</span>
                  <span class="mobile-log-value multiline">{{ row.message || '-' }}</span>
                </div>
              </div>
              <div class="mobile-log-actions">
                <el-button type="primary" plain @click="handleViewDetail(row)">查看详情</el-button>
              </div>
            </el-card>
          </template>
        </div>
            </div>
      <el-pagination
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.pageSize"
        :page-sizes="[20, 50, 100]"
        :total="pagination.total"
        layout="total, sizes, prev, pager, next, jumper"
        @size-change="handleSizeChange"
        @current-change="handleCurrentChange"
        style="margin-top: 20px"
      />
    </el-card>

    <!-- 详情对话框 -->
    <el-dialog v-model="detailVisible" title="日志详情" width="600px">
      <el-descriptions :column="1" border>
        <el-descriptions-item label="手机号">{{ currentLog.account?.phone }}</el-descriptions-item>
        <el-descriptions-item label="任务类型">
          {{ getTaskTypeName(currentLog.task_type) }}
        </el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag :type="getStatusType(currentLog.status)">
            {{ getStatusName(currentLog.status) }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="获得云朵">
          {{ currentLog.cloud_gained > 0 ? `+${currentLog.cloud_gained}` : '-' }}
        </el-descriptions-item>
        <el-descriptions-item label="执行时间">
          {{ formatDate(currentLog.created_at) }}
        </el-descriptions-item>
        <el-descriptions-item label="执行结果">
          <pre style="margin: 0; white-space: pre-wrap;">{{ currentLog.message }}</pre>
        </el-descriptions-item>
      </el-descriptions>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import { getTaskLogs, type TaskLog } from '../api/task'

const loading = ref(false)
const detailVisible = ref(false)
const logList = ref<TaskLog[]>([])
const currentLog = ref<TaskLog>({} as TaskLog)
const viewportWidth = ref(typeof window !== 'undefined' ? window.innerWidth : 1440)
const isMobile = computed(() => viewportWidth.value <= 768)

const syncViewport = () => {
  viewportWidth.value = window.innerWidth
}

const searchForm = reactive({
  taskType: '',
  status: ''
})

const pagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0
})

// 任务类型名称映射
const taskTypeNames: Record<string, string> = {
  signin: '签到',
  wechat: '微信',
  shake: '摇一摇',
  todaycloud: '今日云朵',
  aicloud: 'AI云朵',
  blindbox: '盲盒',
  redpacket: '红包',
  store: '商店',
  garden: '花园',
  cloudphone: '云朵手机',
  cloudbattle: '云朵大战',
  invitefriends: '邀请好友',
  messagepush: '消息推送',
  backupgift: '备份礼包',
  exchange: '兑换',
  tasklist: '任务列表'
}

// 状态名称映射
const statusNames: Record<string, string> = {
  success: '成功',
  failed: '失败',
  pending: '执行中'
}

// 获取任务类型名称
const getTaskTypeName = (type: string) => {
  return taskTypeNames[type] || type
}

// 获取状态名称
const getStatusName = (status: string) => {
  return statusNames[status] || status
}

// 获取状态类型
const getStatusType = (status: string) => {
  const types: Record<string, any> = {
    success: 'success',
    failed: 'danger',
    pending: 'warning'
  }
  return types[status] || 'info'
}

// 加载日志列表
const loadLogs = async () => {
  loading.value = true
  try {
    const params: any = {
      page: pagination.page,
      page_size: pagination.pageSize
    }
    if (searchForm.taskType) params.task_type = searchForm.taskType
    if (searchForm.status) params.status = searchForm.status

    const data = await getTaskLogs(undefined, pagination.page, pagination.pageSize)
    logList.value = data.task_logs
    pagination.total = data.total
  } catch (error) {
    ElMessage.error('加载日志列表失败')
  } finally {
    loading.value = false
  }
}

// 搜索
const handleSearch = () => {
  pagination.page = 1
  loadLogs()
}

// 重置
const handleReset = () => {
  searchForm.taskType = ''
  searchForm.status = ''
  pagination.page = 1
  loadLogs()
}

// 分页大小变化
const handleSizeChange = (size: number) => {
  pagination.pageSize = size
  loadLogs()
}

// 当前页变化
const handleCurrentChange = (page: number) => {
  pagination.page = page
  loadLogs()
}

// 查看详情
const handleViewDetail = (row: TaskLog) => {
  currentLog.value = row
  detailVisible.value = true
}

// 格式化日期
const formatDate = (date: string) => {
  return new Date(date).toLocaleString('zh-CN')
}

onMounted(() => {
  syncViewport()
  window.addEventListener('resize', syncViewport)
  loadLogs()
})

onUnmounted(() => {
  window.removeEventListener('resize', syncViewport)
})
</script>

<style scoped>
.task-logs-container {
  padding: 20px;
  background: linear-gradient(135deg, #f0f9ff 0%, #e0f2fe 100%);
  min-height: calc(100vh - 140px);
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.search-form {
  margin-bottom: 20px;
  padding: 20px;
  background: rgba(255, 255, 255, 0.8);
  border-radius: 12px;
  backdrop-filter: blur(10px);
}

:deep(.el-tag--success) {
  background: linear-gradient(135deg, #10b981 0%, #34d399 100%);
  border: none;
  color: #fff;
}

:deep(.el-tag--danger) {
  background: linear-gradient(135deg, #ef4444 0%, #f87171 100%);
  border: none;
  color: #fff;
}

:deep(.el-tag--warning) {
  background: linear-gradient(135deg, #f59e0b 0%, #fbbf24 100%);
  border: none;
  color: #fff;
}

:deep(.el-card) {
  border-radius: 16px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.5);
  backdrop-filter: blur(10px);
  background: rgba(255, 255, 255, 0.9);
}

.logs-shell { min-height: 140px; }
.mobile-log-list { display: grid; gap: 12px; }
.mobile-log-card { border-radius: 18px; border: 1px solid rgba(226, 232, 240, 0.9); background: rgba(255, 255, 255, 0.94); box-shadow: 0 14px 28px rgba(37, 99, 235, 0.08); }
.mobile-log-card :deep(.el-card__body) { padding: 16px; }
.mobile-log-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; margin-bottom: 12px; }
.mobile-log-title { font-size: 15px; font-weight: 700; color: #0f172a; }
.mobile-log-meta { margin-top: 4px; font-size: 12px; color: #64748b; }
.mobile-log-grid { display: grid; gap: 10px; }
.mobile-log-row { display: grid; grid-template-columns: 76px minmax(0, 1fr); gap: 10px; align-items: start; }
.mobile-log-row.full { grid-template-columns: 1fr; }
.mobile-log-label { font-size: 12px; color: #64748b; font-weight: 600; }
.mobile-log-value { font-size: 13px; color: #334155; word-break: break-word; }
.mobile-log-value.positive { color: #059669; font-weight: 700; }
.mobile-log-value.multiline { line-height: 1.6; }
.mobile-log-actions { margin-top: 14px; display: flex; }
.mobile-log-actions :deep(.el-button) { width: 100%; margin: 0; }
@media (max-width: 768px) {
  .logs-shell :deep(.el-table) { display: none; }
}
</style>
