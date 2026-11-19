<template>
  <div class="daily-summary">
    <div class="summary-toolbar">
      <label for="task-summary-date">日期</label>
      <el-date-picker
        id="task-summary-date"
        :model-value="date"
        type="date"
        value-format="YYYY-MM-DD"
        :clearable="false"
        aria-label="汇总日期"
        @update:model-value="emit('update:date', String($event))"
      />
      <el-button @click="emit('update:date', taskLogDate())">
        今天
      </el-button>
    </div>
    <p class="summary-note">
      每个账号当天汇总一行，云朵与音符分别统计确认到账的数量。
    </p>
    <div
      v-loading="loading"
      :aria-busy="loading"
    >
      <el-table
        v-if="!isMobile"
        :data="accounts"
        stripe
      >
        <el-table-column
          label="账号"
          min-width="180"
        >
          <template #default="{ row }">
            <div>{{ maskPhone(row.phone) }}</div>
            <small
              v-if="row.remark"
              class="account-remark"
            >{{ row.remark }}</small>
          </template>
        </el-table-column>
        <el-table-column
          label="已确认收益"
          width="140"
        >
          <template #default="{ row }">
            <div>
              <span class="summary-gain summary-cloud-gain">{{ row.cloud_gained > 0 ? '+' : '' }}{{ row.cloud_gained }}</span><span> 云朵</span>
            </div>
            <div>
              <span class="summary-gain summary-note-gain">{{ row.migu_notes_gained > 0 ? '+' : '' }}{{ row.migu_notes_gained || 0 }}</span><span> 音符</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column
          label="任务结果"
          min-width="300"
        >
          <template #default="{ row }">
            <span>成功 {{ row.success_count }} 项</span>
            <el-button
              v-if="row.failed_count"
              link
              type="danger"
              @click="showDetails(row, 'failed')"
            >
              失败 {{ row.failed_count }} 项
            </el-button>
            <el-button
              v-if="row.pending_count"
              link
              type="warning"
              @click="showDetails(row, 'pending')"
            >
              待确认 {{ row.pending_count }} 项
            </el-button>
            <span v-if="row.skipped_count"> · 已跳过 {{ row.skipped_count }} 项</span>
          </template>
        </el-table-column>
        <el-table-column
          label="最近执行"
          width="180"
        >
          <template #default="{ row }">
            {{ new Date(row.last_executed_at).toLocaleString('zh-CN') }}
          </template>
        </el-table-column>
        <el-table-column
          label="操作"
          width="115"
        >
          <template #default="{ row }">
            <el-button
              link
              type="primary"
              @click="showDetails(row)"
            >
              查看明细
            </el-button>
          </template>
        </el-table-column>
        <template #empty>
          当天还没有执行记录
        </template>
      </el-table>
      <template v-else>
        <el-empty
          v-if="!accounts.length"
          description="当天还没有执行记录"
        />
        <div
          v-for="account in accounts"
          :key="account.account_id"
          class="summary-account"
        >
          <div class="summary-account-head">
            <span>{{ maskPhone(account.phone) }}</span>
            <div class="summary-account-gains">
              <span class="summary-gain summary-cloud-gain">{{ account.cloud_gained > 0 ? '+' : '' }}{{ account.cloud_gained }} 云朵</span>
              <span class="summary-gain summary-note-gain">{{ account.migu_notes_gained > 0 ? '+' : '' }}{{ account.migu_notes_gained || 0 }} 音符</span>
            </div>
          </div>
          <div class="summary-account-results">
            <span>成功 {{ account.success_count }} 项</span>
            <el-button
              v-if="account.failed_count"
              link
              type="danger"
              @click="showDetails(account, 'failed')"
            >
              失败 {{ account.failed_count }} 项
            </el-button>
            <el-button
              v-if="account.pending_count"
              link
              type="warning"
              @click="showDetails(account, 'pending')"
            >
              待确认 {{ account.pending_count }} 项
            </el-button>
            <el-button
              link
              type="primary"
              @click="showDetails(account)"
            >
              查看明细
            </el-button>
          </div>
        </div>
      </template>
    </div>
    <el-pagination
      v-if="total > pageSize"
      v-model:current-page="page"
      :page-size="pageSize"
      :total="total"
      :small="isMobile"
      layout="total, prev, pager, next"
      @current-change="load"
    />
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { getTaskDailySummary, type TaskDailySummary } from '@/api/task'
import { useLatestRequest } from '@/composables/useLatestRequest'
import { taskLogDate } from '@/utils/task-log-status'
import { wsClient } from '@/api/websocket'
import { createCoalescedRefresh } from '@/utils/coalesced-refresh'

const props = defineProps<{ date: string; isMobile: boolean }>()
const emit = defineEmits<{
  (event: 'update:date', value: string): void
  (event: 'detail', accountID: number, date: string, status: string): void
}>()
const accounts = ref<TaskDailySummary[]>([])
const page = ref(1)
const pageSize = 20
const total = ref(0)
const { loading, execute } = useLatestRequest()
const maskPhone = (phone: string) => phone.replace(/1[3-9]\d{9}/g, value => `${value.slice(0, 3)}****${value.slice(-4)}`) || '-'
const showDetails = (account: TaskDailySummary, status = '') => emit('detail', account.account_id, props.date, status)
const load = () => execute(() => getTaskDailySummary(props.date, page.value, pageSize), data => {
  accounts.value = data.accounts
  total.value = data.total
}, () => ElMessage.error('加载每日汇总失败，请稍后重试'))
watch(() => props.date, () => { page.value = 1; accounts.value = []; total.value = 0; load() })
const liveRefresh = createCoalescedRefresh(load)
onMounted(() => { load(); wsClient.on('task_summary', liveRefresh.trigger) })
onUnmounted(() => { liveRefresh.dispose(); wsClient.off('task_summary', liveRefresh.trigger) })
defineExpose({ refresh: load })
</script>

<style scoped>
.summary-toolbar, .summary-account-head, .summary-account-results { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.summary-toolbar :deep(.el-date-editor) { width: 170px; }
.summary-note, .account-remark { color: var(--el-text-color-secondary); font-size: 13px; }
.summary-note { margin: 18px 0; }
.summary-gain { color: var(--el-color-success); font-weight: 600; }
.summary-account { padding: 18px 0; border-bottom: 1px solid var(--el-border-color-lighter); }
.summary-account-head { justify-content: space-between; margin-bottom: 10px; }
.summary-account-gains { display: flex; gap: 12px; flex-wrap: wrap; }
.summary-account-results { gap: 10px; font-size: 13px; }
.summary-account-results :deep(.el-button + .el-button) { margin-left: 0; }
.el-table :deep(.el-button) { margin-left: 12px; }
.el-pagination { margin-top: 20px; }
</style>
