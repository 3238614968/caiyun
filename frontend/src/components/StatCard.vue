<template>
  <el-card shadow="hover" class="stat-card">
    <div class="stat-content">
      <div class="stat-icon" :style="{ background: gradientColor }">
        <el-icon :size="32" color="#fff">
          <component :is="icon" />
        </el-icon>
      </div>
      <div class="stat-info">
        <div class="stat-value">{{ formattedValue }}</div>
        <div class="stat-label">{{ label }}</div>
        <div v-if="showDiff && diff !== 0" class="stat-diff" :class="diff > 0 ? 'positive' : 'negative'">
          <el-icon><component :is="diff > 0 ? 'ArrowUp' : 'ArrowDown'" /></el-icon>
          {{ Math.abs(diff) }}
        </div>
      </div>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  value: number | string
  label: string
  icon: string
  color?: string
  diff?: number
  showDiff?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  color: '#3b82f6',
  diff: 0,
  showDiff: true
})

const gradientColor = computed(() => {
  const colors: Record<string, string> = {
    blue: 'linear-gradient(135deg, #3b82f6 0%, #0ea5e9 100%)',
    green: 'linear-gradient(135deg, #10b981 0%, #34d399 100%)',
    orange: 'linear-gradient(135deg, #f59e0b 0%, #fbbf24 100%)',
    red: 'linear-gradient(135deg, #ef4444 0%, #f87171 100%)',
    purple: 'linear-gradient(135deg, #8b5cf6 0%, #a78bfa 100%)',
    cyan: 'linear-gradient(135deg, #06b6d4 0%, #22d3ee 100%)'
  }
  return colors[props.color] || colors.blue
})

const formattedValue = computed(() => {
  if (typeof props.value === 'number') {
    return props.value.toLocaleString()
  }
  return props.value
})
</script>

<style scoped>
.stat-card {
  border-radius: 16px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.5);
  backdrop-filter: blur(10px);
  background: rgba(255, 255, 255, 0.9);
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

.stat-info {
  flex: 1;
}

.stat-value {
  font-size: 28px;
  font-weight: bold;
  color: #333;
  margin-bottom: 4px;
}

.stat-label {
  font-size: 14px;
  color: #666;
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
