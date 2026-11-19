<template>
  <Transition name="activity-dock">
    <aside
      v-if="items.length && !dismissed"
      class="activity-dock"
      :class="{ expanded }"
      aria-label="后台任务状态"
    >
      <button
        class="activity-dock-trigger"
        type="button"
        :aria-expanded="expanded"
        @click="expanded = !expanded"
      >
        <span
          class="activity-orb"
          :class="{ busy: busyCount > 0 }"
        ><el-icon><Loading v-if="busyCount" /><Check v-else /></el-icon></span>
        <span class="activity-dock-copy"><strong>{{ busyCount ? `${busyCount} 个后台任务正在处理` : '后台任务已更新' }}</strong><small>{{ busyCount ? '可以继续操作，结果会自动通知你' : '展开查看最近的执行状态' }}</small></span>
        <el-icon
          class="activity-chevron"
          :class="{ expanded }"
        >
          <ArrowUp />
        </el-icon>
      </button>
      <div
        v-if="expanded"
        class="activity-detail-list"
      >
        <div
          v-for="item in items"
          :key="item.id"
          class="activity-detail"
        >
          <span
            class="status-dot"
            :class="item.status"
          /><span>{{ names[item.type] || '后台操作' }}</span><small>{{ statuses[item.status] || item.status }}</small>
        </div>
        <div class="activity-dock-actions">
          <router-link
            to="/logs"
            @click="expanded = false"
          >
            查看运行日志 ↗
          </router-link><button
            type="button"
            aria-label="隐藏任务状态"
            @click="dismissed = true"
          >
            隐藏状态
          </button>
        </div>
      </div>
    </aside>
  </Transition>
</template>

<script setup lang="ts">
import { computed, onMounted, onScopeDispose, ref } from 'vue'
import { ArrowUp, Check, Loading } from '@element-plus/icons-vue'
import { wsClient, isOperationUpdatedMessage, type WsMessage } from '@/api/websocket'

type Activity = { id: string; type: string; status: string; attempt: number; updatedAt: string; sequence: number }
const activities = ref<Activity[]>([])
const expanded = ref(false)
const dismissed = ref(false)
const items = computed(() => activities.value.slice(0, 5))
const busyCount = computed(() => activities.value.filter(item => ['queued', 'running', 'retrying'].includes(item.status)).length)
const names: Record<string, string> = { account_tasks: '云盘日常任务', account_task: '云盘日常任务', exchange: '云朵兑换', product_sync: '同步商品', monthly_exchange: '每月兑换', account_batch: '批量账号任务' }
const statuses: Record<string, string> = { queued: '排队中', running: '执行中', retrying: '等待重试', succeeded: '已完成', failed: '执行失败', canceled: '已取消' }
const accept = (operation: { operation_id: string; type: string; status: string; attempt_count?: number; updated_at?: string; sequence?: number }) => {
  const existing = activities.value.find(item => item.id === operation.operation_id)
  const attempt = Number(operation.attempt_count || 0)
  const updatedAt = operation.updated_at || ''
  const sequence = Number(operation.sequence || 0)
  if (existing && (attempt < existing.attempt || (attempt === existing.attempt && (sequence < existing.sequence || (updatedAt && existing.updatedAt && updatedAt < existing.updatedAt))))) return
  if (existing) {
    existing.status = operation.status
    existing.attempt = attempt
    existing.updatedAt = updatedAt
    existing.sequence = sequence
  } else {
    activities.value.unshift({ id: operation.operation_id, type: operation.type, status: operation.status, attempt, updatedAt, sequence })
    activities.value = activities.value.slice(0, 20)
    dismissed.value = false
  }
}
const onMessage = (message: WsMessage) => { if (isOperationUpdatedMessage(message)) accept(message.data) }
const onQueued = (event: Event) => { accept((event as CustomEvent).detail) }
onMounted(() => {
  wsClient.on('operation.updated', onMessage)
  window.addEventListener('caiyun:operation', onQueued)
})
onScopeDispose(() => {
  wsClient.off('operation.updated', onMessage)
  window.removeEventListener('caiyun:operation', onQueued)
})
</script>

<style scoped>
.activity-dock { position: fixed; bottom: 25px; right: 24px; width: 340px; max-width: calc(100vw - 30px); background: #fff; border: 1px solid var(--line-strong); border-radius: 13px; box-shadow: 0 12px 40px #409eff20; z-index: 100; overflow: hidden; }
.activity-dock-trigger { display: flex; align-items: center; gap: 11px; padding: 13px 15px; border: 0; background: transparent; width: 100%; text-align: left; cursor: pointer; }
.activity-orb { width: 31px; height: 31px; border-radius: 9px; display: grid; place-items: center; background: var(--accent-soft); color: var(--accent); }
.activity-orb.busy .el-icon { animation: activity-spin 1.3s linear infinite; }
.activity-dock-copy { display: grid; gap: 4px; }
.activity-dock-copy strong { font-size: 11px; font-weight: 550; color: var(--text); }
.activity-dock-copy small { font-size: 11px; color: var(--text-muted); }
.activity-chevron { margin-left: auto; color: var(--text-muted); transition: transform .25s; font-size: 11px; }
.activity-chevron.expanded { transform: rotate(180deg); }
.activity-detail-list { padding: 2px 16px 14px; animation: surface-enter .2s var(--ease-enter); }
.activity-detail { display: flex; align-items: center; gap: 9px; padding: 11px 0; border-top: 1px solid var(--line); color: var(--text-secondary); font-size: 11px; }
.activity-detail small { margin-left: auto; font-size: 11px; color: var(--text-muted); }
.activity-detail .status-dot { width: 5px; height: 5px; }
.status-dot.failed { background: var(--danger); }
.status-dot.queued { background: #b7a17a; }
.activity-dock-actions { display: flex; justify-content: space-between; padding-top: 10px; }
.activity-dock-actions a,.activity-dock-actions button { color: var(--text-muted); font-size: 11px; background: none; border: 0; text-decoration: none; cursor: pointer; }
.activity-dock-actions a:hover { color: var(--accent); }
.activity-dock-enter-active,.activity-dock-leave-active { transition: opacity .2s, translate .3s var(--ease-enter); }
.activity-dock-enter-from,.activity-dock-leave-to { opacity: 0; translate: 0 12px; }
@keyframes activity-spin { to { transform: rotate(360deg); } }
@media(max-width:768px) { .activity-dock { left: 50%; right: auto; transform: translateX(-50%); bottom: 86px; } }
</style>
