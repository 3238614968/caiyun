<template>
  <el-row :gutter="20">
    <el-col
      v-for="stat in stats"
      :key="stat.key"
      :span="6"
    >
      <el-card
        shadow="hover"
        class="stat-card"
      >
        <div class="stat-content">
          <div
            class="stat-icon"
            :style="{ background: stat.color }"
          >
            <el-icon><component :is="stat.icon" /></el-icon>
          </div>
          <div class="stat-info">
            <div class="stat-value">
              {{ stat.value }}
            </div>
            <div class="stat-label">
              {{ stat.label }}
            </div>
          </div>
        </div>
      </el-card>
    </el-col>
  </el-row>
</template>

<script setup lang="ts">
export interface AdminStatItem {
  key: string
  label: string
  value: unknown
  icon: string
  color: string
}

defineProps<{
  stats: AdminStatItem[]
}>()
</script>

<style scoped>
.stat-card { transition: transform 0.2s ease, box-shadow 0.2s ease; }
.stat-card:hover { transform: translateY(-2px); }
.stat-content { display: flex; align-items: center; gap: 16px; }
.stat-icon { width: 56px; height: 56px; border-radius: 14px; display: inline-flex; align-items: center; justify-content: center; color: #fff; font-size: 26px; box-shadow: 0 12px 24px rgba(15, 23, 42, 0.12); flex: 0 0 auto; }
.stat-info { min-width: 0; }
.stat-value { font-size: 28px; font-weight: 800; color: #0f172a; line-height: 1.1; word-break: break-word; }
.stat-label { margin-top: 4px; font-size: 13px; color: #64748b; font-weight: 600; }
@media (max-width: 768px) {
  :deep(.el-row) { gap: 12px 0; }
  :deep(.el-col) { max-width: 50%; flex: 0 0 50%; }
  .stat-card :deep(.el-card__body) { padding: 16px; }
  .stat-content { gap: 12px; }
  .stat-icon { width: 44px; height: 44px; border-radius: 12px; font-size: 21px; }
  .stat-value { font-size: 22px; }
}
@media (max-width: 480px) {
  :deep(.el-col) { max-width: 100%; flex: 0 0 100%; }
}
</style>
