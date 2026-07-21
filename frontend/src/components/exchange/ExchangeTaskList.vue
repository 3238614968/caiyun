<template>
  <div class="task-section">
    <el-empty
      v-if="tasks.length === 0"
      description="暂无抢兑任务"
    />

    <template v-else-if="isMobile">
      <div class="mobile-card-list">
        <el-card
          v-for="task in tasks"
          :key="task.id"
          class="mobile-task-card"
          shadow="hover"
        >
          <div class="mobile-task-header">
            <div class="mobile-task-heading">
              <span class="mobile-task-title">{{ task.prize_name }}</span>
              <span class="mobile-task-account">{{ formatExchangeAccountLabel(task) }}</span>
            </div>
            <div class="tag-stack">
              <el-tag
                :type="task.task_type === 'long_term' ? 'warning' : 'primary'"
                size="small"
              >
                {{ task.task_type === 'long_term' ? '长期' : '固定' }}
              </el-tag>
              <el-tag
                :type="taskStatus(task).type"
                size="small"
              >
                {{ taskStatus(task).label }}
              </el-tag>
            </div>
          </div>

          <div class="mobile-task-info">
            <div class="mobile-task-item">
              <span class="label">抢兑计划</span>
              <span class="value">{{ formatTaskSchedule(task) }}</span>
            </div>
            <div class="mobile-task-item">
              <span class="label">下次执行</span>
              <span class="value">{{ formatNextRun(task.next_run_at) }}</span>
            </div>
            <div class="mobile-task-item">
              <span class="label">执行进度</span>
              <span class="value">{{ formatTaskProgress(task) }}</span>
            </div>
            <div
              v-if="isAdmin"
              class="mobile-task-item"
            >
              <span class="label">所属用户</span>
              <span class="value">用户 #{{ task.user_id }}</span>
            </div>
          </div>

          <div
            v-if="task.last_result || task.skip_reason"
            class="mobile-result-card"
          >
            <div
              v-if="task.last_result"
              class="result-heading"
            >
              <el-tag
                :type="formatExchangeResult(task.last_result).type"
                size="small"
              >
                {{ formatExchangeResult(task.last_result).label }}
              </el-tag>
              <span class="result-message">{{ formatResultMessage(task.last_result) }}</span>
            </div>
            <div
              v-if="task.skip_reason"
              class="skip-message"
            >
              <span>跳过原因：</span>{{ task.skip_reason }}
            </div>
          </div>

          <div class="mobile-task-actions">
            <el-tooltip
              v-if="!canManage(task)"
              content="其他用户任务仅支持查看"
              placement="top"
            >
              <span class="button-tooltip-wrap">
                <el-button
                  size="small"
                  type="primary"
                  disabled
                >立即抢兑</el-button>
              </span>
            </el-tooltip>
            <el-button
              v-else
              size="small"
              type="primary"
              :disabled="!canExecute(task)"
              @click="$emit('execute', task.id)"
            >
              {{ task.status === 'running' ? '执行中' : '立即抢兑' }}
            </el-button>
            <el-button
              size="small"
              type="danger"
              :disabled="!canManage(task)"
              @click="$emit('delete', task.id)"
            >
              删除
            </el-button>
          </div>
        </el-card>
      </div>
    </template>

    <div
      v-else
      class="desktop-task-table"
    >
      <el-table
        :data="tasks"
        border
        stripe
        table-layout="auto"
        row-key="id"
      >
        <el-table-column
          label="商品与账号"
          min-width="230"
        >
          <template #default="{ row }">
            <div class="primary-cell">
              <el-tooltip
                :content="row.prize_name"
                placement="top"
                :show-after="400"
              >
                <span class="primary-title">{{ row.prize_name }}</span>
              </el-tooltip>
              <span class="secondary-text">{{ formatExchangeAccountLabel(row) }}</span>
              <span
                v-if="isAdmin"
                class="owner-text"
              >用户 #{{ row.user_id }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column
          label="类型与状态"
          width="128"
          align="center"
        >
          <template #default="{ row }">
            <div class="tag-stack center">
              <el-tag
                :type="row.task_type === 'long_term' ? 'warning' : 'primary'"
                size="small"
              >
                {{ row.task_type === 'long_term' ? '长期' : '固定' }}
              </el-tag>
              <el-tag
                :type="taskStatus(row).type"
                size="small"
              >
                {{ taskStatus(row).label }}
              </el-tag>
            </div>
          </template>
        </el-table-column>

        <el-table-column
          label="调度计划"
          min-width="210"
        >
          <template #default="{ row }">
            <div class="schedule-cell">
              <span class="task-schedule">{{ formatTaskSchedule(row) }}</span>
              <span class="secondary-text">下次：{{ formatNextRun(row.next_run_at) }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column
          label="执行进度"
          width="150"
        >
          <template #default="{ row }">
            <div class="progress-cell">
              <span>{{ formatTaskProgress(row) }}</span>
              <span
                v-if="row.last_attempt_at"
                class="secondary-text"
              >最近：{{ formatNextRun(row.last_attempt_at) }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column
          label="最近执行结果"
          min-width="320"
        >
          <template #default="{ row }">
            <div
              v-if="row.last_result || row.skip_reason"
              class="result-cell"
            >
              <div
                v-if="row.last_result"
                class="result-heading"
              >
                <el-tag
                  :type="formatExchangeResult(row.last_result).type"
                  size="small"
                >
                  {{ formatExchangeResult(row.last_result).label }}
                </el-tag>
                <el-tooltip
                  :content="formatResultMessage(row.last_result)"
                  placement="top"
                  :show-after="400"
                >
                  <span class="result-message">{{ formatResultMessage(row.last_result) }}</span>
                </el-tooltip>
              </div>
              <el-tooltip
                v-if="row.skip_reason"
                :content="row.skip_reason"
                placement="top"
                :show-after="400"
              >
                <span class="skip-message">跳过：{{ row.skip_reason }}</span>
              </el-tooltip>
            </div>
            <span
              v-else
              class="text-gray"
            >暂无执行记录</span>
          </template>
        </el-table-column>

        <el-table-column
          label="操作"
          width="176"
          align="center"
        >
          <template #default="{ row }">
            <div class="action-cell">
              <el-tooltip
                v-if="!canManage(row)"
                content="其他用户任务仅支持查看"
                placement="top"
              >
                <span class="button-tooltip-wrap">
                  <el-button
                    size="small"
                    type="primary"
                    disabled
                  >立即抢兑</el-button>
                </span>
              </el-tooltip>
              <el-button
                v-else
                size="small"
                type="primary"
                :disabled="!canExecute(row)"
                @click="$emit('execute', row.id)"
              >
                {{ row.status === 'running' ? '执行中' : '立即抢兑' }}
              </el-button>
              <el-button
                size="small"
                type="danger"
                :disabled="!canManage(row)"
                @click="$emit('delete', row.id)"
              >
                删除
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { TagProps } from 'element-plus'
import type { ExchangeTask } from '@/api/exchange'
import { formatExchangeResult } from '@/utils/exchange-result'

const props = withDefaults(defineProps<{
  isMobile: boolean
  tasks: ExchangeTask[]
  isAdmin?: boolean
  currentUserId?: number
}>(), {
  isAdmin: false,
  currentUserId: 0
})

defineEmits<{
  execute: [id: number]
  delete: [id: number]
}>()

const cycleLabelMap: Record<string, string> = { daily: '每日', weekly: '每周', monthly: '每月', once: '仅一次' }
const calendarPolicyMap: Record<string, string> = { all: '每天', workday: '工作日', holiday: '节假日' }
const weekdayLabels = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']
const statusMap: Record<string, { label: string; type: TagProps['type'] }> = {
  pending: { label: '待执行', type: 'info' },
  running: { label: '执行中', type: 'warning' },
  completed: { label: '已完成', type: 'success' },
  failed: { label: '失败', type: 'danger' },
  cancelled: { label: '已取消', type: 'info' },
  canceled: { label: '已取消', type: 'info' }
}

const taskStatus = (task: ExchangeTask) => statusMap[task.status] || { label: task.status || '未知', type: 'info' as const }

const formatExchangeAccountLabel = (task: ExchangeTask) => {
  const rule = task.exchange_rule || task.exchange_account
  const account = rule?.account as { remark?: string; phone?: string } | undefined
  const label = [rule?.remark, account?.remark, rule?.phone, account?.phone]
    .find((value) => typeof value === 'string' && value.trim())
  return label || `规则 ${task.exchange_rule_id || task.exchange_account_id || '-'}`
}

const formatTime = (value?: string) => value ? value.slice(0, 5) : '规则时间'

const formatTaskSchedule = (task: ExchangeTask) => {
  const restockTimes = task.restock_times
    ? String(task.restock_times).split(',').filter(Boolean).map(item => item.slice(0, 5)).join(' / ')
    : ''
  const time = task.custom_cron ? `Cron ${task.custom_cron}` : restockTimes || formatTime(task.scheduled_exchange_time)
  if (task.task_type !== 'long_term') return time
  const cycle = task.restock_cycle || 'daily'
  let suffix = cycleLabelMap[cycle] || cycle
  if (cycle === 'weekly' && Number.isInteger(task.restock_weekday)) suffix += ` ${weekdayLabels[task.restock_weekday as number] || ''}`
  if (cycle === 'monthly' && task.restock_day_of_month) suffix += ` ${task.restock_day_of_month}日`
  const policy = calendarPolicyMap[task.calendar_policy || 'all'] || task.calendar_policy || '每天'
  return `${time} · ${suffix} · ${policy}`
}

const formatNextRun = (value?: string) => {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return `${date.getMonth() + 1}-${String(date.getDate()).padStart(2, '0')} ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`
}

const formatTaskProgress = (task: ExchangeTask) => {
  const attempted = Number(task.attempted_count || 0)
  const maxAttempts = Number(task.max_attempts || 0)
  const success = Number(task.success_count || 0)
  const failed = Number(task.fail_count || 0)
  const limit = maxAttempts > 0 ? ` / ${maxAttempts}` : ''
  return `尝试 ${attempted}${limit} · 成功 ${success} · 失败 ${failed}`
}

const formatResultMessage = (message?: string) => {
  if (!message) return '-'
  const visible = message
    .split(' | ')
    .map(item => item.trim())
    .filter(Boolean)
    .filter(item => !/^(http_status|code|result_code|result|desc|sub_msg|trace_id|body)=/i.test(item))
  return visible.join('；') || message.trim()
}

// 管理员可以查看全站任务，但只能操作自己的任务；缺少当前用户 ID 时默认只读。
const canManage = (task: ExchangeTask) => (
  !props.isAdmin ||
  (props.currentUserId > 0 && task.user_id === props.currentUserId)
)
const isTerminalTaskStatus = (status?: string) => (
  status === 'completed' || status === 'cancelled' || status === 'canceled'
)
const canExecute = (task: ExchangeTask) => {
  if (!canManage(task) || task.status === 'running' || isTerminalTaskStatus(String(task.status))) return false
  return formatExchangeResult(task.last_result).type !== 'success'
}
</script>

<style scoped>
.task-section { min-width: 0; padding-top: 0; display: flex; flex-direction: column; gap: 10px; }
.desktop-task-table { width: 100%; min-width: 0; overflow: hidden; border-radius: 14px; }
.desktop-task-table :deep(.el-table) { width: 100%; }
.desktop-task-table :deep(.el-table__cell) { padding: 10px 0; vertical-align: top; }
.primary-cell, .schedule-cell, .progress-cell, .result-cell { min-width: 0; display: flex; flex-direction: column; gap: 5px; }
.primary-title { display: block; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: #0f172a; font-weight: 700; }
.secondary-text { color: #64748b; font-size: 12px; line-height: 1.45; }
.owner-text { width: fit-content; color: #2563eb; font-size: 11px; padding: 1px 7px; border-radius: 999px; background: #eff6ff; }
.tag-stack { display: flex; flex-wrap: wrap; gap: 6px; }
.tag-stack.center { justify-content: center; }
.task-schedule { color: #334155; font-size: 13px; line-height: 1.55; word-break: break-word; }
.text-gray { color: #94a3b8; }
.result-heading { min-width: 0; display: flex; align-items: flex-start; gap: 8px; }
.result-heading :deep(.el-tag) { flex: 0 0 auto; }
.result-message { min-width: 0; color: #334155; font-size: 13px; line-height: 1.55; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; overflow: hidden; word-break: break-word; }
.skip-message { color: #d97706; font-size: 12px; line-height: 1.5; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; overflow: hidden; word-break: break-word; }
.action-cell { display: flex; align-items: center; justify-content: center; gap: 8px; flex-wrap: wrap; }
.action-cell :deep(.el-button + .el-button) { margin-left: 0; }
.button-tooltip-wrap { display: inline-flex; }
.mobile-card-list { display: grid; gap: 12px; }
.mobile-task-card { border-radius: 18px; border: 1px solid rgba(226,232,240,.9); background: rgba(255,255,255,.92); box-shadow: 0 12px 28px rgba(37,99,235,.08); }
.mobile-task-card :deep(.el-card__body) { padding: 16px; }
.mobile-task-header { display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; margin-bottom: 14px; }
.mobile-task-heading { min-width: 0; display: flex; flex-direction: column; gap: 4px; }
.mobile-task-title { color: #0f172a; font-size: 15px; font-weight: 700; line-height: 1.45; word-break: break-word; }
.mobile-task-account { color: #64748b; font-size: 12px; }
.mobile-task-info { display: flex; flex-direction: column; gap: 9px; }
.mobile-task-item { display: grid; grid-template-columns: 72px minmax(0, 1fr); gap: 8px; font-size: 13px; align-items: start; }
.mobile-task-item .label { color: #64748b; font-weight: 600; }
.mobile-task-item .value { color: #334155; line-height: 1.55; word-break: break-word; }
.mobile-result-card { margin-top: 13px; padding: 11px 12px; border: 1px solid #e2e8f0; border-radius: 12px; background: #f8fafc; display: flex; flex-direction: column; gap: 8px; }
.mobile-task-actions { margin-top: 14px; display: flex; gap: 8px; justify-content: flex-end; flex-wrap: wrap; }
.mobile-task-actions :deep(.el-button + .el-button) { margin-left: 0; }
@media (max-width: 1180px) {
  .desktop-task-table { overflow-x: auto; }
  .desktop-task-table :deep(.el-table) { min-width: 980px; }
}
@media (max-width: 520px) {
  .mobile-task-actions { display: grid; grid-template-columns: 1fr 1fr; }
  .mobile-task-actions :deep(.el-button), .button-tooltip-wrap :deep(.el-button) { width: 100%; margin: 0; }
}
</style>
