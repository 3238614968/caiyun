<template>
  <div class="task-manager">
    <div class="task-toolbar">
      <div class="task-toolbar-tip">
        可直接选择云盘账号和商品创建抢兑任务
      </div>
      <div class="task-toolbar-actions">
        <el-button @click="resetFilters">
          重置筛选
        </el-button>
        <el-button
          type="primary"
          @click="$emit('add')"
        >
          新建抢兑任务
        </el-button>
      </div>
    </div>

    <div class="task-filter-panel">
      <el-input
        :model-value="filters.keyword"
        clearable
        placeholder="账号/备注模糊筛选"
        @update:model-value="updateFilters({ keyword: $event || '' })"
      />
      <el-select
        :model-value="filters.status"
        clearable
        placeholder="任务状态"
        @update:model-value="updateFilters({ status: $event || '' })"
      >
        <el-option
          label="待执行"
          value="pending"
        />
        <el-option
          label="运行中"
          value="running"
        />
        <el-option
          label="已完成"
          value="completed"
        />
        <el-option
          label="失败"
          value="failed"
        />
      </el-select>
      <el-select
        :model-value="filters.active"
        clearable
        placeholder="账号状态"
        @update:model-value="updateFilters({ active: $event ?? null })"
      >
        <el-option
          label="可用账号"
          :value="true"
        />
        <el-option
          label="不可用账号"
          :value="false"
        />
      </el-select>
      <el-input-number
        :model-value="filters.minCloud"
        :min="0"
        controls-position="right"
        placeholder="最低云朵"
        @update:model-value="updateFilters({ minCloud: $event ?? null })"
      />
      <el-input-number
        :model-value="filters.maxCloud"
        :min="0"
        controls-position="right"
        placeholder="最高云朵"
        @update:model-value="updateFilters({ maxCloud: $event ?? null })"
      />
    </div>

    <ExchangeTaskList
      :is-mobile="isMobile"
      :tasks="tasks"
      :is-admin="isAdmin"
      :current-user-id="currentUserId"
      @execute="$emit('execute', $event)"
      @delete="$emit('delete', $event)"
    />
  </div>
</template>

<script setup lang="ts">
import ExchangeTaskList from '@/components/exchange/ExchangeTaskList.vue'
import type { ExchangeTask } from '@/api/exchange'
import { createDefaultExchangeTaskFilters, type ExchangeTaskFilterState } from '@/composables/exchange/useExchangeTaskFilters'

const filters = defineModel<ExchangeTaskFilterState>('filters', { required: true })

defineProps<{
  isMobile: boolean
  tasks: ExchangeTask[]
  isAdmin?: boolean
  currentUserId?: number
}>()

defineEmits<{
  add: []
  execute: [id: number]
  delete: [id: number]
}>()

const updateFilters = (patch: Partial<ExchangeTaskFilterState>) => {
  filters.value = {
    ...filters.value,
    ...patch
  }
}

const resetFilters = () => {
  filters.value = createDefaultExchangeTaskFilters()
}
</script>

<style scoped>
.task-manager { min-width:0; display:flex; flex-direction:column; gap:14px; }
.task-filter-panel { min-width:0; display:grid; grid-template-columns:minmax(220px, 1.4fr) repeat(4, minmax(150px, 1fr)); gap:10px; padding:10px 12px; border-radius:14px; background:rgba(255,255,255,.72); border:1px solid rgba(226,232,240,.86); }
.task-filter-panel > * { width:100%; min-width:0; }
.task-toolbar { display:flex; align-items:center; justify-content:space-between; gap:12px; padding:10px 12px; border-radius:14px; background:rgba(239,246,255,.68); border:1px solid rgba(191,219,254,.76); }
.task-toolbar-tip { color:#64748b; font-size:13px; }
.task-toolbar-actions { display:flex; align-items:center; gap:10px; }
@media (max-width: 1280px) {
  .task-filter-panel { grid-template-columns:repeat(2, minmax(0, 1fr)); }
}
@media (max-width: 900px) {
  .task-filter-panel { grid-template-columns:1fr; }
}
@media (max-width: 768px) {
  .task-toolbar { flex-direction:column; align-items:stretch; }
  .task-toolbar-actions { justify-content:flex-end; }
}
</style>
