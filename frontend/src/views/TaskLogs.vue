<template>
  <div class="task-logs-container">
    <el-card shadow="hover">
      <template #header>
        <div class="card-header">
          <span>运行日志</span>
        </div>
      </template>

      <!-- 筛选栏 -->
      <el-form
        :inline="true"
        :model="searchForm"
        class="search-form"
      >
        <el-form-item label="任务类型">
          <el-select
            v-model="searchForm.taskType"
            placeholder="请选择"
            clearable
          >
            <el-option
              v-for="option in taskTypeOptions"
              :key="option.value || 'all'"
              :label="option.label"
              :value="option.value"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="状态">
          <el-select
            v-model="searchForm.status"
            placeholder="请选择"
            clearable
          >
            <el-option
              label="全部"
              value=""
            />
            <el-option
              label="成功"
              value="success"
            />
            <el-option
              label="失败"
              value="failed"
            />
            <el-option
              label="执行中"
              value="pending"
            />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button
            type="primary"
            @click="handleSearch"
          >
            搜索
          </el-button>
          <el-button @click="handleReset">
            重置
          </el-button>
        </el-form-item>
      </el-form>

      <!-- 日志列表 -->
      <div
        v-loading="loading"
        class="logs-shell"
      >
        <el-table
          v-if="!isMobile"
          :data="logList"
          stripe
          style="width: 100%"
        >
          <el-table-column
            prop="account.phone"
            label="手机号"
            width="150"
          />
          <el-table-column
            prop="task_type"
            label="任务类型"
            width="120"
          >
            <template #default="{ row }">
              <el-tag>{{ getTaskTypeName(row.task_type) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column
            prop="status"
            label="状态"
            width="100"
          >
            <template #default="{ row }">
              <el-tag :type="getStatusType(row.status)">
                {{ getStatusName(row.status) }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column
            prop="cloud_gained"
            label="获得云朵"
            width="100"
          >
            <template #default="{ row }">
              <span
                v-if="row.cloud_gained > 0"
                style="color: #67c23a"
              >+{{ row.cloud_gained }}</span>
              <span v-else>-</span>
            </template>
          </el-table-column>
          <el-table-column
            label="执行结果"
            min-width="340"
          >
            <template #default="{ row }">
              <div class="log-message-preview">
                {{ formatLogMessageForPreview(row) }}
              </div>
            </template>
          </el-table-column>
          <el-table-column
            prop="created_at"
            label="执行时间"
            width="180"
          >
            <template #default="{ row }">
              {{ formatDate(row.created_at) }}
            </template>
          </el-table-column>
          <el-table-column
            label="操作"
            width="100"
            fixed="right"
          >
            <template #default="{ row }">
              <el-button
                type="primary"
                link
                @click="handleViewDetail(row)"
              >
                详情
              </el-button>
            </template>
          </el-table-column>
        </el-table>

        <!-- 分页 -->

        <div
          v-else
          class="mobile-log-list"
        >
          <el-empty
            v-if="logList.length === 0"
            description="暂无日志"
          />
          <template v-else>
            <el-card
              v-for="row in logList"
              :key="row.id"
              class="mobile-log-card"
              shadow="never"
            >
              <div class="mobile-log-head">
                <div>
                  <div class="mobile-log-title">
                    {{ row.account?.phone || '-' }}
                  </div>
                  <div class="mobile-log-meta">
                    {{ getTaskTypeName(row.task_type) }}
                  </div>
                </div>
                <el-tag :type="getStatusType(row.status)">
                  {{ getStatusName(row.status) }}
                </el-tag>
              </div>
              <div class="mobile-log-grid">
                <div class="mobile-log-row">
                  <span class="mobile-log-label">任务类型</span>
                  <span class="mobile-log-value">{{ getTaskTypeName(row.task_type) }}</span>
                </div>
                <div class="mobile-log-row">
                  <span class="mobile-log-label">获得云朵</span>
                  <span
                    class="mobile-log-value"
                    :class="{ 'positive': row.cloud_gained > 0 }"
                  >{{ row.cloud_gained > 0 ? `+${row.cloud_gained}` : '-' }}</span>
                </div>
                <div class="mobile-log-row">
                  <span class="mobile-log-label">执行时间</span>
                  <span class="mobile-log-value">{{ formatDate(row.created_at) }}</span>
                </div>
                <div class="mobile-log-row full">
                  <span class="mobile-log-label">执行结果</span>
                  <span class="mobile-log-value multiline">{{ formatLogMessageForPreview(row) }}</span>
                </div>
              </div>
              <div class="mobile-log-actions">
                <el-button
                  type="primary"
                  plain
                  @click="handleViewDetail(row)"
                >
                  查看详情
                </el-button>
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
        style="margin-top: 20px"
        @size-change="handleSizeChange"
        @current-change="handleCurrentChange"
      />
    </el-card>

    <!-- 详情对话框 -->
    <el-dialog
      v-model="detailVisible"
      title="日志详情"
      width="600px"
    >
      <el-descriptions
        :column="1"
        border
      >
        <el-descriptions-item label="手机号">
          {{ currentLog.account?.phone }}
        </el-descriptions-item>
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
          <div class="log-detail-message">
            {{ formatLogMessageForDetail(currentLog) }}
          </div>
        </el-descriptions-item>
      </el-descriptions>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import { getTaskLogs, type TaskLog } from '../api/task'
import { getTaskTypeName, taskTypeOptions } from '../utils/task-types'

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

// 状态名称映射
const statusNames: Record<string, string> = {
  success: '成功',
  failed: '失败',
  pending: '执行中'
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

    const data = await getTaskLogs(params)
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

const hiddenLogDetailPrefixes = ['http_status=', 'code=', 'result_code=', 'result=', 'desc=', 'sub_msg=', 'trace_id=', 'body=']
const exchangeTaskTypes = new Set(['exchange'])

type LabeledLogSegment = {
  key: string
  value: string
  raw: string
}

const getDefaultLogMessage = (status?: string) => {
  if (status === 'pending') return '\u6267\u884c\u4e2d'
  if (status === 'failed') return '\u6267\u884c\u5931\u8d25'
  if (status === 'success') return '\u5df2\u5b8c\u6210'
  return '-'
}

const normalizeLogText = (value: string) => {
  return value
    .replace(/\s+/g, ' ')
    .replace(/^[\s|:\uFF1A-]+/, '')
    .replace(/[\s|:\uFF1A-]+$/, '')
    .trim()
}

const getVisibleLogSegments = (message?: string) => {
  return (message || '')
    .split(' | ')
    .map(segment => normalizeLogText(segment))
    .filter(Boolean)
    .filter(segment => !hiddenLogDetailPrefixes.some(prefix => segment.toLowerCase().startsWith(prefix)))
}

const parseLabeledLogSegments = (message?: string): LabeledLogSegment[] => {
  return getVisibleLogSegments(message).map(segment => {
    const matched = segment.match(/^([^:\uFF1A]+)[:\uFF1A]\s*(.+)$/)
    if (!matched) {
      return { key: '', value: segment, raw: segment }
    }
    return {
      key: normalizeLogText(matched[1]),
      value: normalizeLogText(matched[2]),
      raw: segment
    }
  })
}

const getLabeledLogValue = (segments: LabeledLogSegment[], label: string) => {
  return segments.find(segment => segment.key === label)?.value || ''
}

const pickPrimaryLogSegment = (segments: string[]) => {
  return (
    segments.find(segment => /^(?:\u7ed3\u679c|\u539f\u56e0)[:\uFF1A]/.test(segment)) ||
    segments.find(segment => /(\u5931\u8d25|\u5f02\u5e38|\u672a\u5b8c\u6210|\u624b\u52a8\u5b8c\u6210|\u5df2\u9886\u53d6|\u5df2\u5151\u6362|\u6210\u529f)/.test(segment)) ||
    segments[0] ||
    ''
  )
}

const shortenText = (value: string, maxLength: number) => {
  return value.length > maxLength ? `${value.slice(0, maxLength)}...` : value
}

const isExchangeLog = (log?: Partial<TaskLog>) => {
  return exchangeTaskTypes.has(`${log?.task_type ?? ''}`.trim())
}

const normalizeExchangeResultText = (value: string, status?: string, detail = false) => {
  let text = normalizeLogText(value)
  if (!text) {
    if (status === 'success') return '\u5151\u6362\u6210\u529f'
    if (status === 'pending') return '\u7b49\u5f85\u5151\u6362'
    return getDefaultLogMessage(status)
  }

  text = text
    .replace(/^(?:\u7ed3\u679c|\u539f\u56e0)[:\uFF1A]\s*/, '')
    .replace(/^.*?(?:\u5931\u8d25|\u5f02\u5e38)[:\uFF1A]\s*/, '')
    .replace(/\s*Error Code:\s*[A-Z0-9_-]+/ig, '')

  text = normalizeLogText(text)

  if (/\u6d3b\u52a8\u5f02\u5e38/.test(text)) {
    return detail ? '\u6d3b\u52a8\u5f02\u5e38\uff0c\u8bf7\u7a0d\u540e\u91cd\u8bd5' : '\u6d3b\u52a8\u5f02\u5e38'
  }

  if (/\u5956\u54c1\u5df2\u5151\u5b8c|\u5df2\u5151\u5b8c|\u552e\u7f44|\u62a2\u7a7a|\u5e93\u5b58\u4e0d\u8db3/.test(text)) {
    return '\u5df2\u5151\u5b8c'
  }

  if (/\u672a\u5f00\u59cb|\u672a\u5230.*\u65f6\u95f4|\u6d3b\u52a8\u672a\u5f00\u59cb/.test(text)) {
    return '\u672a\u5f00\u59cb'
  }

  if (/\u5df2\u5151\u6362|\u5df2\u9886\u53d6|\u91cd\u590d\u9886\u53d6|\u91cd\u590d\u5151\u6362/.test(text)) {
    return '\u5df2\u5151\u6362'
  }

  if (/token|jwt/i.test(text) || /\u767b\u5f55\u5931\u6548|\u8d26\u53f7\u5931\u6548|\u91cd\u65b0\u767b\u5f55/.test(text)) {
    return detail ? '\u8d26\u53f7\u5df2\u5931\u6548\uff0c\u8bf7\u91cd\u65b0\u767b\u5f55' : '\u8d26\u53f7\u5931\u6548'
  }

  if (/\u98ce\u63a7|\u9650\u6d41|\u7cfb\u7edf\u7e41\u5fd9/.test(text)) {
    return detail ? text : shortenText(text, 12)
  }

  if (/\u5151\u6362\u6210\u529f|\u62a2\u5151\u6210\u529f/.test(text) || status === 'success') {
    return '\u5151\u6362\u6210\u529f'
  }

  if (status === 'pending') {
    return '\u7b49\u5f85\u5151\u6362'
  }

  return detail ? text : shortenText(text, 16)
}

const formatExchangeLogMessage = (log?: Partial<TaskLog>, detail = false) => {
  const segments = parseLabeledLogSegments(log?.message)
  const prizeName = getLabeledLogValue(segments, '\u5546\u54c1')
  const exchangeAccount = getLabeledLogValue(segments, '\u5151\u6362\u8d26\u53f7')
  const resultSource = getLabeledLogValue(segments, '\u7ed3\u679c') || pickPrimaryLogSegment(segments.map(segment => segment.raw))
  const resultText = normalizeExchangeResultText(resultSource, log?.status, detail)

  if (detail) {
    const lines: string[] = []
    if (prizeName) lines.push(`\u5546\u54c1\uff1a${prizeName}`)
    if (exchangeAccount) lines.push(`\u5151\u6362\u8d26\u53f7\uff1a${exchangeAccount}`)
    lines.push(`\u7ed3\u679c\uff1a${resultText}`)
    return lines.join('\n')
  }

  if (prizeName) {
    return `${shortenText(prizeName, 18)}\n${resultText}`
  }

  return resultText
}

const normalizeLogSegment = (segment: string, status?: string, detail = false) => {
  let value = normalizeLogText(segment)
  if (!value) return getDefaultLogMessage(status)

  value = value
    .replace(/^(?:\u7ed3\u679c|\u539f\u56e0)[:\uFF1A]\s*/, '')
    .replace(/^.*?(?:\u5931\u8d25|\u5f02\u5e38)[:\uFF1A]\s*/, '')
    .replace(/\s*Error Code:\s*[A-Z0-9_-]+/ig, '')

  value = normalizeLogText(value)

  if (/\u672c\u6708\u5df2\u9886\u53d6\u590d\u6d3b\u5361\u5956\u52b1/.test(value)) {
    return detail ? '\u672c\u6708\u5df2\u9886\u53d6\u590d\u6d3b\u5361\u5956\u52b1' : '\u672c\u6708\u5df2\u9886'
  }

  if (/\u5df2\u9886\u53d6\u590d\u6d3b\u5361\u5956\u52b1|\u9886\u53d6\u590d\u6d3b\u5361\u5956\u52b1\u6210\u529f/.test(value)) {
    return detail ? '\u5df2\u9886\u53d6\u590d\u6d3b\u5361\u5956\u52b1' : '\u5df2\u9886\u53d6'
  }

  if (/\u8bf7\u524d\u5f80\u79fb\u52a8\u4e91\u76d8\u624b\u52a8\u5b8c\u6210|\u624b\u52a8\u5b8c\u6210\u6bcf\u6708\u4efb\u52a1|\u672a\u5b8c\u6210/.test(value)) {
    return detail ? '\u9700\u524d\u5f80\u79fb\u52a8\u4e91\u76d8\u624b\u52a8\u5b8c\u6210' : '\u9700\u624b\u52a8\u5b8c\u6210'
  }

  if (/\u6d3b\u52a8\u5f02\u5e38\uff0c\u8bf7\u7a0d\u540e\u91cd\u8bd5/.test(value)) {
    return '\u6d3b\u52a8\u5f02\u5e38\uff0c\u8bf7\u7a0d\u540e\u91cd\u8bd5'
  }

  if (/\u5df2\u9886\u53d6|\u5df2\u7ecf\u9886\u53d6|\u91cd\u590d\u9886\u53d6/.test(value)) {
    return '\u5df2\u9886\u53d6'
  }

  if (/\u5151\u6362\u6210\u529f|\u62a2\u5151\u6210\u529f/.test(value)) {
    return detail ? '\u5151\u6362\u6210\u529f' : '\u5df2\u5151\u6362'
  }

  if (/\u6267\u884c\u6210\u529f|\u4efb\u52a1\u6267\u884c\u6210\u529f|\u9886\u53d6\u6210\u529f|\u5237\u65b0\u6210\u529f|\u540c\u6b65\u6210\u529f|\u66f4\u65b0\u6210\u529f/.test(value)) {
    return '\u5df2\u5b8c\u6210'
  }

  if (status === 'success') {
    return '\u5df2\u5b8c\u6210'
  }

  if (status === 'pending') {
    return '\u6267\u884c\u4e2d'
  }

  if (status === 'failed') {
    return detail ? value : shortenText(value, 20)
  }

  return detail ? value : shortenText(value, 20)
}

const formatLogMessage = (log?: Partial<TaskLog>, detail = false) => {
  if (isExchangeLog(log)) {
    return formatExchangeLogMessage(log, detail)
  }

  const segments = getVisibleLogSegments(log?.message)
  if (segments.length === 0) return getDefaultLogMessage(log?.status)
  return normalizeLogSegment(pickPrimaryLogSegment(segments), log?.status, detail)
}

const formatLogMessageForPreview = (log?: Partial<TaskLog>) => {
  return formatLogMessage(log, false)
}

const formatLogMessageForDetail = (log?: Partial<TaskLog>) => {
  return formatLogMessage(log, true)
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
.log-message-preview {
  white-space: pre-line;
  line-height: 1.7;
  color: #334155;
  word-break: break-word;
}

.log-detail-message {
  white-space: pre-line;
  line-height: 1.8;
  color: #334155;
  background: rgba(248, 250, 252, 0.95);
  border: 1px solid rgba(226, 232, 240, 0.9);
  border-radius: 12px;
  padding: 12px 14px;
  word-break: break-word;
}

@media (max-width: 768px) {
  .logs-shell :deep(.el-table) { display: none; }
}
</style>
