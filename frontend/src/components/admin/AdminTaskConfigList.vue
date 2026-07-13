<template>
  <div
    v-loading="loading"
    class="responsive-data-shell"
  >
    <el-table
      v-if="!isMobile"
      :data="configs"
      stripe
      style="width: 100%"
    >
      <el-table-column
        prop="sort_order"
        label="序号"
        width="70"
      />
      <el-table-column
        prop="task_name"
        label="任务名称"
        width="120"
      />
      <el-table-column
        prop="task_type"
        label="任务类型"
        width="140"
      >
        <template #default="{ row }">
          <el-tag size="small">
            {{ getTaskTypeName(row.task_type, row.task_name) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column
        label="状态"
        width="160"
      >
        <template #default="{ row }">
          <div class="task-status-cell">
            <el-switch
              :model-value="row.is_enabled"
              @change="(value: string | number | boolean) => $emit('toggle', row, Boolean(value))"
            />
            <span :class="row.is_enabled ? 'status-on' : 'status-off'">
              {{ row.is_enabled ? '已上架' : '已下架' }}
            </span>
          </div>
        </template>
      </el-table-column>
      <el-table-column
        prop="updated_at"
        label="更新时间"
        min-width="180"
      >
        <template #default="{ row }">
          {{ formatDate(row.updated_at) }}
        </template>
      </el-table-column>
    </el-table>

    <div
      v-else
      class="mobile-admin-list"
    >
      <el-empty
        v-if="configs.length === 0"
        description="暂无任务配置"
      />
      <template v-else>
        <el-card
          v-for="row in configs"
          :key="row.task_type"
          class="mobile-admin-card"
          shadow="never"
        >
          <div class="mobile-admin-card-head">
            <div>
              <div class="mobile-admin-card-title">
                {{ row.task_name }}
              </div>
              <div class="mobile-admin-card-meta">
                #{{ row.sort_order }}
              </div>
            </div>
            <el-tag size="small">
              {{ getTaskTypeName(row.task_type, row.task_name) }}
            </el-tag>
          </div>
          <div class="mobile-admin-card-grid">
            <div class="mobile-admin-card-row">
              <span class="mobile-admin-card-label">任务类型</span>
              <span class="mobile-admin-card-value">{{ getTaskTypeName(row.task_type, row.task_name) }}</span>
            </div>
            <div class="mobile-admin-card-row">
              <span class="mobile-admin-card-label">更新时间</span>
              <span class="mobile-admin-card-value">{{ formatDate(row.updated_at) }}</span>
            </div>
          </div>
          <div class="mobile-admin-card-footer">
            <div class="task-status-cell">
              <el-switch
                :model-value="row.is_enabled"
                @change="(value: string | number | boolean) => $emit('toggle', row, Boolean(value))"
              />
              <span :class="row.is_enabled ? 'status-on' : 'status-off'">{{ row.is_enabled ? '已上架' : '已下架' }}</span>
            </div>
          </div>
        </el-card>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { TaskConfig } from '@/api/account'
import { getTaskTypeName } from '@/utils/task-types'

defineProps<{
  isMobile: boolean
  loading: boolean
  configs: TaskConfig[]
  formatDate: (date?: string) => string
}>()

defineEmits<{
  toggle: [row: TaskConfig, enabled: boolean]
}>()
</script>

<style scoped>
.responsive-data-shell { min-height: 120px; }
.task-status-cell { display: inline-flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.status-on { color: #10b981; font-size: 13px; font-weight: 600; }
.status-off { color: #ef4444; font-size: 13px; font-weight: 600; }
.mobile-admin-list { display: grid; gap: 12px; }
.mobile-admin-card { border-radius: 18px; border: 1px solid rgba(226, 232, 240, 0.9); background: rgba(255, 255, 255, 0.94); box-shadow: 0 14px 28px rgba(37, 99, 235, 0.08); }
.mobile-admin-card :deep(.el-card__body) { padding: 16px; }
.mobile-admin-card-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; margin-bottom: 12px; }
.mobile-admin-card-title { font-size: 15px; font-weight: 700; color: #0f172a; line-height: 1.4; word-break: break-word; }
.mobile-admin-card-meta { margin-top: 4px; font-size: 12px; color: #64748b; }
.mobile-admin-card-grid { display: grid; gap: 10px; }
.mobile-admin-card-row { display: grid; grid-template-columns: 84px minmax(0, 1fr); gap: 10px; align-items: start; }
.mobile-admin-card-label { font-size: 12px; color: #64748b; font-weight: 600; }
.mobile-admin-card-value { font-size: 13px; color: #334155; word-break: break-word; }
.mobile-admin-card-footer { margin-top: 14px; padding-top: 12px; border-top: 1px solid rgba(226, 232, 240, 0.8); }
@media (max-width: 768px) {
  .mobile-admin-card-row { grid-template-columns: 78px minmax(0, 1fr); }
  .task-status-cell { align-items: flex-start; }
}
</style>
