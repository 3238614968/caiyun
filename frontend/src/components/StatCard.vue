<template>
  <el-card
    shadow="hover"
    class="stat-card"
  >
    <div class="stat-content">
      <div
        class="stat-icon"
        :style="{ background: gradientColor }"
      >
        <el-icon
          :size="30"
          color="#fff"
        >
          <component :is="icon" />
        </el-icon>
      </div>
      <div class="stat-info">
        <div class="stat-value">
          {{ formattedValue }}
        </div>
        <div class="stat-label">
          {{ label }}
        </div>
        <div
          v-if="showDiff && diff !== 0"
          class="stat-diff"
          :class="diff > 0 ? 'positive' : 'negative'"
        >
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
    blue: 'linear-gradient(135deg, #2563eb 0%, #0ea5e9 100%)',
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
  height: 100%;
  border-radius: 22px;
  box-shadow: 0 14px 30px rgba(37, 99, 235, 0.08);
  border: 1px solid rgba(255, 255, 255, 0.72);
  backdrop-filter: blur(14px);
  background: rgba(255, 255, 255, 0.9);
}

.stat-content {
  display: flex;
  align-items: center;
  gap: 14px;
}

.stat-icon {
  width: 58px;
  height: 58px;
  border-radius: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  box-shadow: 0 12px 24px rgba(15, 23, 42, 0.12);
}

.stat-info {
  min-width: 0;
}

.stat-value {
  font-size: clamp(24px, 3vw, 30px);
  font-weight: 800;
  color: #0f172a;
  line-height: 1.05;
}

.stat-label {
  margin-top: 6px;
  font-size: 14px;
  color: #64748b;
}

.stat-diff {
  margin-top: 6px;
  font-size: 12px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.stat-diff.positive {
  color: #10b981;
}

.stat-diff.negative {
  color: #ef4444;
}

@media (max-width: 768px) {
  .stat-card {
    border-radius: 18px;
  }

  .stat-content {
    gap: 12px;
  }

  .stat-icon {
    width: 48px;
    height: 48px;
    border-radius: 14px;
  }

  .stat-value {
    font-size: 22px;
  }

  .stat-label {
    font-size: 13px;
  }
}
</style>
