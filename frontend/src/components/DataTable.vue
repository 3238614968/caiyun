<template>
  <div class="data-table-wrapper">
    <el-table
      :data="data"
      v-loading="loading"
      stripe
      :border="border"
      style="width: 100%"
      @selection-change="handleSelectionChange"
    >
      <el-table-column v-if="selectable" type="selection" width="55" />
      <slot />
    </el-table>

    <div v-if="showPagination" class="pagination-wrapper">
      <el-pagination
        v-model:current-page="currentPage"
        v-model:page-size="pageSize"
        :page-sizes="pageSizes"
        :total="total"
        :layout="layout"
        @size-change="handleSizeChange"
        @current-change="handleCurrentChange"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  data: any[]
  loading?: boolean
  total: number
  page?: number
  pageSize?: number
  pageSizes?: number[]
  selectable?: boolean
  border?: boolean
  showPagination?: boolean
  layout?: string
}

const props = withDefaults(defineProps<Props>(), {
  loading: false,
  page: 1,
  pageSize: 10,
  pageSizes: () => [10, 20, 50, 100],
  selectable: false,
  border: false,
  showPagination: true,
  layout: 'total, sizes, prev, pager, next, jumper'
})

const emit = defineEmits<{
  (e: 'update:page', page: number): void
  (e: 'update:pageSize', pageSize: number): void
  (e: 'change', page: number, pageSize: number): void
  (e: 'selection-change', selection: any[]): void
}>()

const currentPage = computed({
  get: () => props.page,
  set: (val) => emit('update:page', val)
})

const pageSize = computed({
  get: () => props.pageSize,
  set: (val) => emit('update:pageSize', val)
})

const handleSizeChange = (val: number) => {
  emit('update:pageSize', val)
  emit('change', currentPage.value, val)
}

const handleCurrentChange = (val: number) => {
  emit('update:page', val)
  emit('change', val, pageSize.value)
}

const handleSelectionChange = (selection: any[]) => {
  emit('selection-change', selection)
}
</script>

<style scoped>
.data-table-wrapper {
  background: rgba(255, 255, 255, 0.9);
  backdrop-filter: blur(10px);
  border-radius: 16px;
  padding: 20px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.5);
}

.pagination-wrapper {
  margin-top: 20px;
  display: flex;
  justify-content: flex-end;
}

:deep(.el-table) {
  background: transparent;
}

:deep(.el-table th) {
  background: rgba(59, 130, 246, 0.05);
  color: #1e293b;
  font-weight: 600;
}

:deep(.el-table--striped .el-table__body tr.el-table__row--striped td) {
  background: rgba(59, 130, 246, 0.02);
}
</style>
