<template>
  <el-dialog
    v-model="visible"
    title="批量创建结果"
    :width="isMobile ? '95%' : '760px'"
    :close-on-click-modal="false"
  >
    <div
      v-if="display"
      class="result-dialog"
    >
      <div class="summary-row">
        <el-tag type="info">
          总计 {{ display.summary.total }} 项
        </el-tag>
        <el-tag type="success">
          成功 {{ display.summary.success }} 项
        </el-tag>
        <el-tag :type="display.summary.failed > 0 ? 'danger' : 'info'">
          失败 {{ display.summary.failed }} 项
        </el-tag>
      </div>

      <el-alert
        v-if="retryable"
        type="warning"
        :closable="false"
        show-icon
        title="可对失败项执行一键重试，系统会沿用当前商品和调度配置。"
      />

      <div class="result-list">
        <div
          v-for="row in display.rows"
          :key="row.key"
          class="result-item"
          :class="row.success ? 'is-success' : 'is-danger'"
        >
          <div class="result-item-header">
            <span class="result-icon">{{ row.icon }}</span>
            <span class="result-target">{{ row.targetTypeLabel }}：{{ row.targetLabel }}</span>
            <el-tag
              size="small"
              :type="row.success ? 'success' : 'danger'"
            >
              {{ row.success ? '成功' : '失败' }}
            </el-tag>
          </div>
          <div class="result-message">
            {{ row.message }}
          </div>
        </div>
      </div>
    </div>
    <template #footer>
      <el-button @click="visible = false">
        关闭
      </el-button>
      <el-button
        v-if="retryable"
        type="primary"
        :loading="retryLoading"
        @click="$emit('retry')"
      >
        重试失败项
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import type { BatchCreateTaskResultDisplay } from '@/utils/exchange-task-results'

const visible = defineModel<boolean>({ required: true })

defineProps<{
  isMobile: boolean
  display: BatchCreateTaskResultDisplay | null
  retryable: boolean
  retryLoading: boolean
}>()

defineEmits<{
  retry: []
}>()
</script>

<style scoped>
.result-dialog { display:flex; flex-direction:column; gap:14px; }
.summary-row { display:flex; flex-wrap:wrap; gap:10px; }
.result-list { display:flex; flex-direction:column; gap:10px; max-height:min(56vh, 520px); overflow:auto; padding-right:4px; }
.result-item { border-radius:16px; border:1px solid rgba(226,232,240,.92); background:rgba(248,250,252,.88); padding:14px; }
.result-item.is-success { border-color:rgba(134,239,172,.9); background:rgba(240,253,244,.92); }
.result-item.is-danger { border-color:rgba(252,165,165,.82); background:rgba(254,242,242,.94); }
.result-item-header { display:flex; align-items:flex-start; gap:8px; flex-wrap:wrap; margin-bottom:8px; }
.result-icon { font-size:16px; line-height:1.2; }
.result-target { flex:1; min-width:220px; color:#0f172a; font-weight:600; word-break:break-word; }
.result-message { color:#475569; font-size:13px; line-height:1.6; word-break:break-word; }
@media (max-width: 768px) {
  .result-item { padding:12px; border-radius:14px; }
  .result-target { min-width:0; }
}
</style>
