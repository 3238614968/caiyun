export interface TrendPoint {
  date: string
  cloud_count: number
  cloud_diff?: number
  has_data?: boolean
}

export interface TrendChartBounds {
  width: number
  height: number
  top: number
  right: number
  bottom: number
  left: number
}

export interface TrendChartPoint extends TrendPoint {
  index: number
  x: number
  y: number
}

export interface TrendChartTick {
  value: number
  y: number
}

export interface TrendChartGeometry {
  bounds: TrendChartBounds
  points: TrendChartPoint[]
  linePath: string
  areaPath: string
  yTicks: TrendChartTick[]
  xLabelStep: number
}

export const DEFAULT_TREND_BOUNDS: TrendChartBounds = {
  width: 720,
  height: 350,
  top: 32,
  right: 20,
  bottom: 40,
  left: 72
}

export const formatTrendDate = (value: string, short = false) => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }

  const year = date.getFullYear()
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')

  return short ? `${month}-${day}` : `${year}-${month}-${day}`
}

export const formatCloudCount = (value: number | string) => {
  const count = Number(value)
  if (Number.isNaN(count)) {
    return String(value)
  }
  return count.toLocaleString('zh-CN')
}

export const formatTrendDiff = (value?: number) => {
  const count = Number(value || 0)
  if (count === 0) return '0'
  return `${count > 0 ? '+' : ''}${formatCloudCount(count)}`
}

export const getTrendYAxisRange = (trendData: TrendPoint[]) => {
  const values = trendData.map(item => Number(item.cloud_count)).filter(value => !Number.isNaN(value))
  if (values.length === 0) {
    return { min: 0, max: 100 }
  }

  const minValue = Math.min(...values)
  const maxValue = Math.max(...values)
  const spread = Math.max(maxValue - minValue, 1)
  const padding = Math.max(Math.ceil(spread * 0.14), Math.ceil(maxValue * 0.02), 10)

  return {
    min: Math.max(0, minValue - padding),
    max: maxValue + padding
  }
}

export const buildTrendChartGeometry = (
  trendData: TrendPoint[],
  width = DEFAULT_TREND_BOUNDS.width,
  height = DEFAULT_TREND_BOUNDS.height
): TrendChartGeometry => {
  const bounds: TrendChartBounds = {
    ...DEFAULT_TREND_BOUNDS,
    width: Math.max(Math.round(width), DEFAULT_TREND_BOUNDS.left + DEFAULT_TREND_BOUNDS.right + 120),
    height: Math.max(Math.round(height), DEFAULT_TREND_BOUNDS.top + DEFAULT_TREND_BOUNDS.bottom + 120)
  }

  const { min, max } = getTrendYAxisRange(trendData)
  const plotWidth = Math.max(bounds.width - bounds.left - bounds.right, 1)
  const plotHeight = Math.max(bounds.height - bounds.top - bounds.bottom, 1)
  const denominator = Math.max(max - min, 1)
  const bottomY = bounds.height - bounds.bottom
  const points = trendData.map((point, index) => {
    const x = trendData.length <= 1
      ? bounds.left + plotWidth / 2
      : bounds.left + (plotWidth * index) / Math.max(trendData.length - 1, 1)
    const normalizedValue = (Number(point.cloud_count) - min) / denominator
    const y = bottomY - normalizedValue * plotHeight

    return {
      ...point,
      index,
      x,
      y
    }
  })

  const linePath = points
    .map((point, index) => `${index === 0 ? 'M' : 'L'} ${point.x.toFixed(2)} ${point.y.toFixed(2)}`)
    .join(' ')

  const areaPath = points.length > 0
    ? [
        `M ${points[0].x.toFixed(2)} ${bottomY.toFixed(2)}`,
        ...points.map((point, index) => `${index === 0 ? 'L' : 'L'} ${point.x.toFixed(2)} ${point.y.toFixed(2)}`),
        `L ${points[points.length - 1].x.toFixed(2)} ${bottomY.toFixed(2)}`,
        'Z'
      ].join(' ')
    : ''

  const yTicks = Array.from({ length: 5 }, (_, index) => {
    const ratio = index / 4
    const value = max - ratio * (max - min)
    return {
      value: Math.round(value),
      y: bounds.top + plotHeight * ratio
    }
  })

  const xLabelStep = trendData.length <= 7 ? 1 : Math.ceil(trendData.length / 7)

  return {
    bounds,
    points,
    linePath,
    areaPath,
    yTicks,
    xLabelStep
  }
}
