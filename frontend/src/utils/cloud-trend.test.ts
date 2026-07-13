import {
  buildTrendChartGeometry,
  formatCloudCount,
  formatTrendDate,
  formatTrendDiff,
  getTrendYAxisRange
} from './cloud-trend'

describe('cloud trend utils', () => {
  it('formats dates and diff labels', () => {
    expect(formatTrendDate('2026-06-20T00:00:00Z', true)).toBe('06-20')
    expect(formatCloudCount(12345)).toBe('12,345')
    expect(formatTrendDiff(120)).toBe('+120')
    expect(formatTrendDiff(-36)).toBe('-36')
    expect(formatTrendDiff(0)).toBe('0')
  })

  it('calculates padded y-axis range', () => {
    expect(getTrendYAxisRange([])).toEqual({ min: 0, max: 100 })
    expect(getTrendYAxisRange([
      { date: '2026-06-18', cloud_count: 1200 },
      { date: '2026-06-19', cloud_count: 1300 },
      { date: '2026-06-20', cloud_count: 1250 }
    ])).toEqual({ min: 1174, max: 1326 })
  })

  it('builds chart geometry with reduced x labels for long ranges', () => {
    const trendData = Array.from({ length: 30 }, (_, index) => ({
      date: `2026-06-${`${index + 1}`.padStart(2, '0')}`,
      cloud_count: 1000 + index * 20
    }))

    const geometry = buildTrendChartGeometry(trendData, 900, 350)

    expect(geometry.points).toHaveLength(30)
    expect(geometry.linePath.startsWith('M ')).toBe(true)
    expect(geometry.areaPath.endsWith('Z')).toBe(true)
    expect(geometry.yTicks).toHaveLength(5)
    expect(geometry.xLabelStep).toBe(5)
    expect(geometry.points[0].x).toBeLessThan(geometry.points[29].x)
  })
})
