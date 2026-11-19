<template>
  <section
    class="dashboard-stat-grid"
    :aria-label="label"
  >
    <article
      v-for="stat in displayStats"
      :key="stat.key"
      class="dashboard-stat-card"
    >
      <span
        class="dashboard-stat-icon"
        aria-hidden="true"
      ><el-icon><component :is="icons[stat.icon] || Document" /></el-icon></span>
      <dl class="dashboard-stat-data">
        <dt class="dashboard-stat-label">
          {{ stat.label }}
        </dt>
        <dd class="dashboard-stat-metric">
          <span class="dashboard-stat-amount">
            <span class="dashboard-stat-number">{{ stat.formattedValue }}</span><span
              v-if="stat.unit"
              class="dashboard-stat-unit"
            >{{ stat.unit }}</span>
          </span>
          <span
            v-if="stat.diff !== 0"
            class="dashboard-stat-diff"
            :class="stat.diff > 0 ? 'positive' : 'negative'"
          >
            <el-icon><component :is="stat.diff > 0 ? ArrowUp : ArrowDown" /></el-icon>
            {{ Math.abs(stat.diff).toLocaleString('zh-CN') }}
          </span>
        </dd>
      </dl>
    </article>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Cloudy, User, Document, CircleCheck, TrendCharts, ArrowUp, ArrowDown } from '@element-plus/icons-vue'

export interface DashboardStatItem {
  key: string
  label: string
  value: number | string
  diff: number
  icon: string
}

const props = withDefaults(defineProps<{ stats: DashboardStatItem[]; label?: string }>(), { label: '账号统计' })
const icons: Record<string, unknown> = { Cloudy, User, Document, CircleCheck, TrendCharts }
const units: Record<string, string> = { total_cloud: '朵', account_count: '个', today_gained: '朵', success_rate: '%', user_count: '人', active_tasks: '项' }
const displayStats = computed(() => props.stats.map(stat => {
  let formattedValue = typeof stat.value === 'number' ? stat.value.toLocaleString('zh-CN') : stat.value
  const unit = units[stat.key] || ''
  if (unit === '%' && formattedValue.endsWith('%')) formattedValue = formattedValue.slice(0, -1)
  return { ...stat, formattedValue, unit }
}))
</script>

<style scoped>
.dashboard-stat-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 16px; }
.dashboard-stat-card { position: relative; min-width: 0; padding: 18px 22px; border: 1px solid #ffffffd9; border-radius: 16px; background: linear-gradient(135deg, #fffffff2, #f3f9ffe6); box-shadow: 0 4px 18px #2672b50b; }
.dashboard-stat-data { margin: 0; }
.dashboard-stat-label { display: flex; align-items: center; min-height: 36px; padding-right: 50px; color: #667d96; font-size: 13px; font-weight: 500; line-height: 1.5; }
.dashboard-stat-icon { position: absolute; top: 18px; right: 22px; display: grid; place-items: center; width: 36px; height: 36px; border: 1px solid #d5eaff; border-radius: 11px; background: linear-gradient(135deg, #edf6ff, #e3f1ff); color: #409eff; }
.dashboard-stat-icon .el-icon { font-size: 22px; }
.dashboard-stat-metric { display: flex; align-items: baseline; justify-content: space-between; flex-wrap: wrap; gap: 8px; margin: 10px 0 0; }
.dashboard-stat-amount { display: flex; align-items: baseline; min-width: 0; max-width: 100%; gap: 6px; }
.dashboard-stat-number { min-width: 0; color: #2e6eac; font-size: 32px; font-weight: 600; line-height: 1.15; letter-spacing: -.025em; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
.dashboard-stat-unit { flex-shrink: 0; color: #8498ae; font-size: 12px; line-height: 1; }
.dashboard-stat-diff { display: inline-flex; align-items: center; gap: 3px; font-size: 12px; font-variant-numeric: tabular-nums; }
.dashboard-stat-diff.positive { color: #239373; }
.dashboard-stat-diff.negative { color: #cf6262; }
@media(max-width:1199px) { .dashboard-stat-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media(max-width:600px) {
  .dashboard-stat-grid { gap: 12px; }
  .dashboard-stat-card { padding: 14px 15px 16px; border-radius: 14px; }
  .dashboard-stat-label { min-height: 30px; padding-right: 36px; font-size: 12px; }
  .dashboard-stat-icon { top: 14px; right: 15px; width: 30px; height: 30px; border-radius: 9px; }
  .dashboard-stat-icon .el-icon { font-size: 18px; }
  .dashboard-stat-metric { margin-top: 10px; gap: 6px; }
  .dashboard-stat-number { font-size: clamp(24px, 6.9vw, 28px); }
  .dashboard-stat-unit { font-size: 11px; }
  .dashboard-stat-amount { gap: 4px; }
  .dashboard-stat-diff { font-size: 11px; }
}
</style>
