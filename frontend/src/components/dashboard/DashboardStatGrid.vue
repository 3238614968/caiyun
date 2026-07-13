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
            <div
              v-if="stat.diff !== 0"
              class="stat-diff"
              :class="stat.diff > 0 ? 'positive' : 'negative'"
            >
              <el-icon><component :is="stat.diff > 0 ? 'ArrowUp' : 'ArrowDown'" /></el-icon>
              {{ Math.abs(stat.diff) }}
            </div>
          </div>
        </div>
      </el-card>
    </el-col>
  </el-row>
</template>

<script setup lang="ts">
export interface DashboardStatItem {
  key: string
  label: string
  value: number | string
  diff: number
  icon: string
  color: string
}

defineProps<{
  stats: DashboardStatItem[]
}>()
</script>

<style scoped>
.stat-card {
  margin-bottom: 20px;
  border-radius: 16px;
  box-shadow: 0 8px 32px rgba(59, 130, 246, 0.1);
  border: 1px solid rgba(255, 255, 255, 0.6);
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  transition: all 0.3s ease;
}

.stat-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 12px 40px rgba(59, 130, 246, 0.15);
  background: rgba(255, 255, 255, 0.85);
}

.stat-content {
  display: flex;
  align-items: center;
}

.stat-icon {
  width: 60px;
  height: 60px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-right: 16px;
  box-shadow: 0 4px 15px rgba(0, 0, 0, 0.1);
}

.stat-icon .el-icon {
  font-size: 32px;
  color: white;
}

.stat-info {
  flex: 1;
}

.stat-value {
  font-size: 28px;
  font-weight: 800;
  color: #1d4ed8;
  margin-bottom: 4px;
  letter-spacing: 0.2px;
}

.stat-label {
  font-size: 14px;
  color: #2563eb;
  font-weight: 600;
  margin-bottom: 4px;
}

.stat-diff {
  font-size: 12px;
  display: flex;
  align-items: center;
}

.stat-diff.positive {
  color: #10b981;
}

.stat-diff.negative {
  color: #ef4444;
}
</style>
