<template>
  <div
    v-loading="loading"
    class="responsive-data-shell"
  >
    <div class="task-catalog-toolbar">
      <el-input
        v-model="query"
        placeholder="搜索任务名称或类型…"
        aria-label="搜索任务"
        clearable
        :prefix-icon="Search"
      /><SegmentedControl
        v-model="filter"
        :options="[{ label: '全部', value: '', count: configs.length }, { label: '已上架', value: 'enabled', count: configs.filter(item => item.is_enabled).length }, { label: '已下架', value: 'disabled' }]"
        label="任务上架状态"
      />
    </div>
    <el-table
      v-if="!isMobile"
      :data="filteredConfigs"
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
              :loading="pendingCodes?.includes(row.task_type)"
              :aria-label="`${row.is_enabled ? '下架' : '上架'}${row.task_name}`"
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
          v-for="row in filteredConfigs"
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
                :loading="pendingCodes?.includes(row.task_type)"
                :aria-label="`${row.is_enabled ? '下架' : '上架'}${row.task_name}`"
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
import { computed, ref } from 'vue'
import { Search } from '@element-plus/icons-vue'
import SegmentedControl from '@/components/ui/SegmentedControl.vue'
const query = ref('')
const filter = ref<string | number>('')
import type { TaskConfig } from '@/api/account'
import { getTaskTypeName } from '@/utils/task-types'

const props = defineProps<{
  isMobile: boolean
  loading: boolean
  configs: TaskConfig[]
  pendingCodes?: string[]
  formatDate: (date?: string) => string
}>()

const filteredConfigs = computed(() => props.configs.filter(item => (!filter.value || (filter.value === 'enabled' ? item.is_enabled : !item.is_enabled)) && `${item.task_name} ${item.task_type}`.toLowerCase().includes(query.value.trim().toLowerCase())))

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
.mobile-admin-card { border-radius: 18px; border: 1px solid rgba(226, 232, 240, 0.9); background: rgba(255, 255, 255, 0.94); box-shadow: 0 14px 28px rgba(15, 23, 42, 0.08); }
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
