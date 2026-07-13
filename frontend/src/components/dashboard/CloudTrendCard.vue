<template>
  <el-card
    shadow="hover"
    class="chart-card"
  >
    <template #header>
      <div class="card-header">
        <span>云朵趋势</span>
        <el-radio-group
          :model-value="days"
          size="small"
          @change="handleDaysChange"
        >
          <el-radio-button :label="7">
            7天
          </el-radio-button>
          <el-radio-button :label="14">
            14天
          </el-radio-button>
          <el-radio-button :label="30">
            30天
          </el-radio-button>
        </el-radio-group>
      </div>
    </template>

    <div
      ref="trendChartRef"
      class="trend-chart"
      data-testid="cloud-trend-chart"
    >
      <svg
        v-if="chartModel.points.length > 0"
        class="trend-svg"
        :viewBox="`0 0 ${chartModel.bounds.width} ${chartModel.bounds.height}`"
        role="img"
        aria-label="云朵趋势图"
      >
        <defs>
          <linearGradient
            :id="gradientID"
            x1="0%"
            y1="0%"
            x2="0%"
            y2="100%"
          >
            <stop
              offset="0%"
              stop-color="#3b82f6"
              stop-opacity="0.34"
            />
            <stop
              offset="100%"
              stop-color="#3b82f6"
              stop-opacity="0.08"
            />
          </linearGradient>
        </defs>

        <g class="grid-layer">
          <g
            v-for="tick in chartModel.yTicks"
            :key="`tick-${tick.value}-${tick.y}`"
          >
            <line
              :x1="chartModel.bounds.left"
              :x2="chartModel.bounds.width - chartModel.bounds.right"
              :y1="tick.y"
              :y2="tick.y"
              class="grid-line"
            />
            <text
              :x="chartModel.bounds.left - 12"
              :y="tick.y + 4"
              class="axis-text axis-text--y"
              text-anchor="end"
            >
              {{ formatCloudCount(tick.value) }}
            </text>
          </g>
        </g>

        <g class="axis-layer">
          <text
            :x="chartModel.bounds.left"
            :y="chartModel.bounds.top - 10"
            class="axis-title"
          >
            云朵数
          </text>

          <line
            :x1="chartModel.bounds.left"
            :x2="chartModel.bounds.width - chartModel.bounds.right"
            :y1="chartModel.bounds.height - chartModel.bounds.bottom"
            :y2="chartModel.bounds.height - chartModel.bounds.bottom"
            class="axis-line"
          />
        </g>

        <line
          v-if="activePoint"
          :x1="activePoint.x"
          :x2="activePoint.x"
          :y1="chartModel.bounds.top"
          :y2="chartModel.bounds.height - chartModel.bounds.bottom"
          class="hover-line"
        />

        <path
          v-if="chartModel.areaPath"
          :d="chartModel.areaPath"
          :fill="`url(#${gradientID})`"
        />
        <path
          v-if="chartModel.linePath"
          :d="chartModel.linePath"
          class="trend-line"
        />

        <g
          v-for="point in chartModel.points"
          :key="`${point.date}-${point.index}`"
          class="point-group"
        >
          <circle
            :cx="point.x"
            :cy="point.y"
            r="18"
            class="point-hit"
            tabindex="0"
            :aria-label="`${formatTrendDate(point.date)} 云朵 ${formatCloudCount(point.cloud_count)}`"
            @mouseenter="activatePoint(point.index)"
            @mousemove="activatePoint(point.index)"
            @focus="activatePoint(point.index)"
            @mouseleave="deactivatePoint"
            @blur="deactivatePoint"
          />
          <circle
            :cx="point.x"
            :cy="point.y"
            :r="point.index === emphasisIndex ? 6.5 : 5"
            :class="['trend-point', point.index === emphasisIndex ? 'trend-point--active' : '']"
          />
          <text
            v-if="shouldRenderXLabel(point.index)"
            :x="point.x"
            :y="chartModel.bounds.height - 10"
            class="axis-text axis-text--x"
            text-anchor="middle"
          >
            {{ formatTrendDate(point.date, true) }}
          </text>
        </g>
      </svg>

      <div
        v-else
        class="trend-empty"
      >
        <strong>暂无趋势数据</strong>
        <span>请先执行任务或等待采样写入。</span>
      </div>

      <div
        v-if="activePoint"
        class="trend-tooltip"
        :style="tooltipStyle"
      >
        <div class="trend-tooltip__date">
          {{ formatTrendDate(activePoint.date) }}
        </div>
        <div class="trend-tooltip__row">
          <span class="trend-tooltip__label">
            <i class="trend-tooltip__dot" />
            云朵数
          </span>
          <strong>{{ formatCloudCount(activePoint.cloud_count) }}</strong>
        </div>
        <div class="trend-tooltip__row trend-tooltip__row--secondary">
          <span>较前日</span>
          <strong :class="trendDiffClass(activePoint.cloud_diff)">{{ formatTrendDiff(activePoint.cloud_diff) }}</strong>
        </div>
        <div class="trend-tooltip__meta">
          {{ activePoint.has_data === false ? '补齐值' : '采样值' }}
        </div>
      </div>
    </div>

    <div
      class="trend-summary"
      data-testid="cloud-trend-summary"
    >
      <span>当前展示 {{ days }} 天</span>
      <strong>最新云朵 {{ latestSummary.cloud }}</strong>
      <span :class="latestSummary.diffClass">较前日 {{ latestSummary.diff }}</span>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
  buildTrendChartGeometry,
  DEFAULT_TREND_BOUNDS,
  formatCloudCount,
  formatTrendDate,
  formatTrendDiff,
  type TrendPoint
} from '@/utils/cloud-trend'

interface TrendSummary {
  cloud: string
  diff: string
  diffClass?: string
}

const props = defineProps<{
  days: number
  trendData: TrendPoint[]
  latestSummary: TrendSummary
}>()

const emit = defineEmits<{
  (e: 'update:days', value: number): void
}>()

const trendChartRef = ref<HTMLElement>()
const chartSize = ref({
  width: DEFAULT_TREND_BOUNDS.width,
  height: DEFAULT_TREND_BOUNDS.height
})
const activePointIndex = ref<number | null>(null)
const gradientID = `cloud-trend-gradient-${Math.random().toString(36).slice(2, 8)}`

let resizeObserver: ResizeObserver | null = null
let detachResize: (() => void) | null = null

const chartModel = computed(() =>
  buildTrendChartGeometry(props.trendData || [], chartSize.value.width, chartSize.value.height)
)

const emphasisIndex = computed(() => {
  if (activePointIndex.value !== null) {
    return activePointIndex.value
  }
  return Math.max(chartModel.value.points.length - 1, 0)
})

const activePoint = computed(() => {
  if (activePointIndex.value === null) {
    return null
  }
  return chartModel.value.points[activePointIndex.value] ?? null
})

const tooltipStyle = computed(() => {
  if (!activePoint.value) {
    return {}
  }

  const tooltipWidth = 190
  const tooltipHeight = 112
  const left = Math.min(
    Math.max(activePoint.value.x - tooltipWidth / 2, 12),
    chartModel.value.bounds.width - tooltipWidth - 12
  )
  const top = Math.min(
    Math.max(activePoint.value.y - tooltipHeight - 18, 12),
    chartModel.value.bounds.height - tooltipHeight - 12
  )

  return {
    left: `${left}px`,
    top: `${top}px`
  }
})

const handleDaysChange = (value: number | string | boolean | undefined) => {
  activePointIndex.value = null
  emit('update:days', Number(value))
}

const trendDiffClass = (value?: number) => {
  const diff = Number(value || 0)
  if (diff > 0) return 'positive'
  if (diff < 0) return 'negative'
  return ''
}

const shouldRenderXLabel = (index: number) => {
  const { xLabelStep, points } = chartModel.value
  return index === 0 || index === points.length - 1 || index % xLabelStep === 0
}

const activatePoint = (index: number) => {
  activePointIndex.value = index
}

const deactivatePoint = () => {
  activePointIndex.value = null
}

const syncChartSize = () => {
  const element = trendChartRef.value
  if (!element) return

  const rect = element.getBoundingClientRect()
  if (rect.width > 0) {
    chartSize.value = {
      width: Math.round(rect.width),
      height: Math.round(rect.height || DEFAULT_TREND_BOUNDS.height)
    }
  }
}

onMounted(() => {
  syncChartSize()

  if (typeof ResizeObserver !== 'undefined' && trendChartRef.value) {
    resizeObserver = new ResizeObserver(() => syncChartSize())
    resizeObserver.observe(trendChartRef.value)
  } else {
    const handler = () => syncChartSize()
    window.addEventListener('resize', handler)
    detachResize = () => window.removeEventListener('resize', handler)
  }
})

onUnmounted(() => {
  resizeObserver?.disconnect()
  detachResize?.()
})
</script>

<style scoped>
.chart-card {
  height: 100%;
}

.chart-card :deep(.el-card__body) {
  padding-bottom: 12px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  color: #1e40af;
  font-weight: 600;
}

.trend-chart {
  position: relative;
  height: 350px;
}

.trend-svg {
  width: 100%;
  height: 100%;
  overflow: visible;
}

.grid-line {
  stroke: rgba(148, 163, 184, 0.18);
  stroke-width: 1;
}

.axis-line {
  stroke: rgba(148, 163, 184, 0.55);
  stroke-width: 1;
}

.axis-title {
  fill: #64748b;
  font-size: 13px;
}

.axis-text {
  fill: #64748b;
  font-size: 12px;
}

.hover-line {
  stroke: rgba(148, 163, 184, 0.9);
  stroke-width: 1;
  stroke-dasharray: 4 4;
}

.trend-line {
  fill: none;
  stroke: #3b82f6;
  stroke-width: 4;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.point-hit {
  fill: transparent;
  cursor: pointer;
}

.point-hit:focus-visible {
  outline: none;
}

.trend-point {
  fill: #ffffff;
  stroke: #3b82f6;
  stroke-width: 3;
  transition: all 0.2s ease;
}

.trend-point--active {
  fill: #3b82f6;
  stroke: #ffffff;
  stroke-width: 4;
  filter: drop-shadow(0 0 10px rgba(59, 130, 246, 0.24));
}

.trend-tooltip {
  position: absolute;
  z-index: 2;
  min-width: 190px;
  padding: 12px 14px;
  border: 1px solid rgba(219, 234, 254, 0.95);
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.97);
  box-shadow: 0 14px 36px rgba(15, 23, 42, 0.14);
  pointer-events: none;
}

.trend-tooltip__date {
  margin-bottom: 10px;
  font-size: 14px;
  font-weight: 600;
  color: #475569;
}

.trend-tooltip__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  color: #1e293b;
  font-size: 14px;
}

.trend-tooltip__row strong {
  font-size: 24px;
  color: #0f172a;
}

.trend-tooltip__row--secondary {
  margin-top: 8px;
  color: #64748b;
  font-size: 12px;
}

.trend-tooltip__row--secondary strong {
  font-size: 13px;
}

.trend-tooltip__label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: #475569;
}

.trend-tooltip__dot {
  display: inline-block;
  width: 12px;
  height: 12px;
  border-radius: 999px;
  background: #3b82f6;
  box-shadow: 0 0 0 4px rgba(59, 130, 246, 0.14);
}

.trend-tooltip__meta {
  margin-top: 6px;
  color: #94a3b8;
  font-size: 12px;
}

.trend-empty {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  gap: 8px;
  border: 1px dashed rgba(148, 163, 184, 0.4);
  border-radius: 16px;
  background: linear-gradient(180deg, rgba(239, 246, 255, 0.72) 0%, rgba(248, 250, 252, 0.92) 100%);
  color: #64748b;
}

.trend-empty strong {
  color: #1d4ed8;
  font-size: 16px;
}

.trend-summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  margin-top: 10px;
  padding: 10px 12px;
  border-radius: 12px;
  background: rgba(239, 246, 255, .72);
  color: #475569;
  font-size: 13px;
}

.trend-summary strong {
  color: #1d4ed8;
  font-size: 15px;
}

.positive {
  color: #10b981;
  font-weight: 700;
}

.negative {
  color: #ef4444;
  font-weight: 700;
}
</style>
