<template>
  <div class="task-section">
    <template v-if="isMobile">
      <div class="mobile-card-list">
        <el-card v-for="task in tasks" :key="task.id" class="mobile-task-card" shadow="hover">
          <div class="mobile-task-header">
            <span class="mobile-task-title">{{ task.prize_name }}</span>
            <el-tag :type="task.task_type === 'long_term' ? 'warning' : 'primary'" size="small">
              {{ task.task_type === 'long_term' ? '长期' : '固定' }}
            </el-tag>
          </div>
          <div class="mobile-task-info">
            <div class="mobile-task-item">
              <span class="label">兑换账号：</span>
              <span class="value">{{ task.exchange_account?.remark || '账号' + task.exchange_account_id }}</span>
            </div>
            <div v-if="task.last_result" class="mobile-task-item">
              <span class="label">执行结果：</span>
              <el-tag :type="formatExchangeResult(task.last_result).type" size="small">
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
            <el-button size="small" type="danger" @click="$emit('delete', task.id)">删除</el-button>
          </div>
        </el-card>
      </div>
    </template>

    <el-table v-else :data="tasks" border stripe>
      <el-table-column prop="prize_name" label="商品名称" min-width="150" />
      <el-table-column label="兑换账号" min-width="120">
        <template #default="{ row }">
          {{ row.exchange_account?.remark || '账号' + row.exchange_account_id }}
        </template>
      </el-table-column>
      <el-table-column label="任务类型" width="100">
        <template #default="{ row }">
          <el-tag :type="row.task_type === 'long_term' ? 'warning' : 'primary'" size="small">
            {{ row.task_type === 'long_term' ? '长期' : '固定' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="执行结果" min-width="200">
        <template #default="{ row }">
          <div v-if="row.last_result" class="task-result">
            <el-tag :type="formatExchangeResult(row.last_result).type" size="small">
              {{ formatExchangeResult(row.last_result).label }}
            </el-tag>
          </div>
          <span v-else class="text-gray">-</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="180" fixed="right">
        <template #default="{ row }">
          <el-button
            size="small"
            type="primary"
            :disabled="row.last_result && row.last_result.includes('成功')"
            @click="$emit('execute', row.id)"
          >
            立即抢兑
          </el-button>
          <el-button size="small" type="danger" @click="$emit('delete', row.id)">删除</el-button>
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
</script>

<style scoped>
.task-section { padding-top:0; display:flex; flex-direction:column; gap:10px; }
.task-result { display:flex; align-items:center; gap:8px; flex-wrap:wrap; }
.text-gray { color:#94a3b8; }
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
