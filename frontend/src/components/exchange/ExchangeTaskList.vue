<template>
  <div class="task-section">
    <template v-if="isMobile">
      <div class="mobile-card-list">
        <el-card
          v-for="task in tasks"
          :key="task.id"
          class="mobile-task-card"
          shadow="hover"
        >
          <div class="mobile-task-header">
            <span class="mobile-task-title">{{ task.prize_name }}</span>
            <el-tag
              :type="task.task_type === 'long_term' ? 'warning' : 'primary'"
              size="small"
            >
              {{ task.task_type === 'long_term' ? '长期' : '固定' }}
            </el-tag>
          </div>
          <div class="mobile-task-info">
            <div class="mobile-task-item">
              <span class="label">抢兑账号：</span>
              <span class="value">{{ formatExchangeAccountLabel(task) }}</span>
            </div>
            <div class="mobile-task-item">
              <span class="label">抢兑配置：</span>
              <span class="value">{{ formatTaskSchedule(task) }}</span>
            </div>
            <div class="mobile-task-item">
              <span class="label">下次执行：</span>
              <span class="value">{{ formatNextRun(task.next_run_at) }}</span>
            </div>
            <div
              v-if="task.skip_reason"
              class="mobile-task-item"
            >
              <span class="label">跳过原因：</span>
              <span class="value warning-text">{{ task.skip_reason }}</span>
            </div>
            <div
              v-if="task.last_result"
              class="mobile-task-item"
            >
              <span class="label">执行结果：</span>
              <el-tag
                :type="formatExchangeResult(task.last_result).type"
                size="small"
              >
                {{ formatExchangeResult(task.last_result).label }}
              </el-tag>
            </div>
          </div>
          <div class="mobile-task-actions">
            <el-button
              size="small"
              type="primary"
              :disabled="task.last_result && task.last_result.includes('成功')"
              @click="$emit('execute', task.id)"
            >
              立即抢兑
            </el-button>
            <el-button
              size="small"
              type="danger"
              @click="$emit('delete', task.id)"
            >
              删除
            </el-button>
          </div>
        </el-card>
      </div>
    </template>

    <el-table
      v-else
      :data="tasks"
      border
      stripe
    >
      <el-table-column
        prop="prize_name"
        label="商品名称"
        min-width="150"
      />
      <el-table-column
        label="抢兑账号"
        min-width="120"
      >
        <template #default="{ row }">
          {{ formatExchangeAccountLabel(row) }}
        </template>
      </el-table-column>
      <el-table-column
        label="任务类型"
        width="100"
      >
        <template #default="{ row }">
          <el-tag
            :type="row.task_type === 'long_term' ? 'warning' : 'primary'"
            size="small"
          >
            {{ row.task_type === 'long_term' ? '长期' : '固定' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column
        label="抢兑配置"
        min-width="180"
      >
        <template #default="{ row }">
          <span class="task-schedule">{{ formatTaskSchedule(row) }}</span>
        </template>
      </el-table-column>
      <el-table-column
        label="下次执行"
        min-width="150"
      >
        <template #default="{ row }">
          <span class="task-schedule">{{ formatNextRun(row.next_run_at) }}</span>
        </template>
      </el-table-column>
      <el-table-column
        label="跳过原因"
        min-width="180"
      >
        <template #default="{ row }">
          <span
            v-if="row.skip_reason"
            class="warning-text"
          >{{ row.skip_reason }}</span>
          <span
            v-else
            class="text-gray"
          >-</span>
        </template>
      </el-table-column>
      <el-table-column
        label="执行结果"
        min-width="200"
      >
        <template #default="{ row }">
          <div
            v-if="row.last_result"
            class="task-result"
          >
            <el-tag
              :type="formatExchangeResult(row.last_result).type"
              size="small"
            >
              {{ formatExchangeResult(row.last_result).label }}
            </el-tag>
          </div>
          <span
            v-else
            class="text-gray"
          >-</span>
        </template>
      </el-table-column>
      <el-table-column
        label="操作"
        width="180"
        fixed="right"
      >
        <template #default="{ row }">
          <el-button
            size="small"
            type="primary"
            :disabled="row.last_result && row.last_result.includes('成功')"
            @click="$emit('execute', row.id)"
          >
            立即抢兑
          </el-button>
          <el-button
            size="small"
            type="danger"
            @click="$emit('delete', row.id)"
          >
            删除
          </el-button>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup lang="ts">
import { formatExchangeResult } from '@/utils/exchange-result'

defineProps<{
  isMobile: boolean
  tasks: any[]
}>()

defineEmits<{
  execute: [id: number]
  delete: [id: number]
}>()

const cycleLabelMap: Record<string, string> = {
  daily: '每日',
  weekly: '每周',
  monthly: '每月',
  once: '仅一次'
}
const calendarPolicyMap: Record<string, string> = {
  all: '每天',
  workday: '工作日',
  holiday: '节假日'
}
const weekdayLabels = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']

const formatExchangeAccountLabel = (task: any) => {
  const rule = task?.exchange_rule || task?.exchange_account
  const account = rule?.account
  const label = [rule?.remark, account?.remark, rule?.phone, account?.phone]
    .find((value) => typeof value === 'string' && value.trim())
  return label || `规则${task?.exchange_rule_id || task?.exchange_account_id || ''}`
}

const formatTime = (value?: string) => {
  if (!value) return '规则时间'
  return value.slice(0, 5)
}

const formatTaskSchedule = (task: any) => {
  const restockTimes = task.restock_times ? String(task.restock_times).split(',').filter(Boolean).map((item: string) => item.slice(0, 5)).join('/') : ''
  const time = task.custom_cron ? `Cron ${task.custom_cron}` : restockTimes || formatTime(task.scheduled_exchange_time)
  if (task.task_type !== 'long_term') return time
  const cycle = task.restock_cycle || 'daily'
  let suffix = cycleLabelMap[cycle] || cycle
  if (cycle === 'weekly' && Number.isInteger(task.restock_weekday)) {
    suffix += ` ${weekdayLabels[task.restock_weekday] || ''}`
  }
  if (cycle === 'monthly' && task.restock_day_of_month) {
    suffix += ` ${task.restock_day_of_month}日`
  }
  const policy = calendarPolicyMap[task.calendar_policy || 'all'] || task.calendar_policy || '每天'
  return `${time} · ${suffix} · ${policy}`
}

const formatNextRun = (value?: string) => {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return `${date.getMonth() + 1}-${String(date.getDate()).padStart(2, '0')} ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`
}
</script>

<style scoped>
.task-section { padding-top:0; display:flex; flex-direction:column; gap:10px; }
.task-result { display:flex; align-items:center; gap:8px; flex-wrap:wrap; }
.task-schedule { color:#475569; font-size:13px; }
.text-gray { color:#94a3b8; }
.warning-text { color:#d97706; font-size:12px; }
.mobile-card-list { display:grid; gap:12px; }
.mobile-task-card { border-radius:18px; border:1px solid rgba(255,255,255,.78); background: rgba(255,255,255,.84); box-shadow:0 12px 28px rgba(37,99,235,.08); }
.mobile-task-card :deep(.el-card__body) { padding:16px; }
.mobile-task-header { display:flex; justify-content:space-between; align-items:flex-start; gap:10px; margin-bottom:12px; }
.mobile-task-title { font-size:15px; font-weight:700; color:#0f172a; }
.mobile-task-info { display:flex; flex-direction:column; gap:8px; margin-bottom:12px; }
.mobile-task-item { display:flex; gap:8px; font-size:13px; }
.mobile-task-item .label { min-width:68px; color:#64748b; flex-shrink:0; }
.mobile-task-item .value { color:#334155; flex:1; word-break:break-word; }
.mobile-task-actions { display:flex; gap:8px; justify-content:flex-end; flex-wrap:wrap; }
@media (max-width: 520px) {
  .mobile-task-actions { flex-direction:column; }
  .mobile-task-actions :deep(.el-button) { width:100%; }
}
</style>
