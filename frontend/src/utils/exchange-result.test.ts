import { describe, expect, it } from 'vitest'

import { formatExchangeResult } from './exchange-result'

describe('formatExchangeResult', () => {
  it('normalizes successful exchange messages', () => {
    expect(formatExchangeResult('兑换成功', 'failed')).toEqual({
      label: '兑换成功',
      type: 'success'
    })
  })

  it('detects monthly duplicate exchange messages', () => {
    expect(formatExchangeResult('重复兑奖', 'failed')).toEqual({
      label: '已兑换过',
      type: 'warning'
    })
    expect(formatExchangeResult('本月已兑换同系列商品', 'failed')).toEqual({
      label: '已兑换过',
      type: 'warning'
    })
  })

  it('maps authentication and stock errors to concise labels', () => {
    expect(formatExchangeResult('JWT Token 已过期', 'failed')).toEqual({
      label: '账号登录失效',
      type: 'danger'
    })
    expect(formatExchangeResult('库存不足', 'failed')).toEqual({
      label: '库存已抢完',
      type: 'warning'
    })
  })

  it('falls back to compact unknown message', () => {
    expect(formatExchangeResult('需要人工确认的特殊返回内容', 'failed')).toEqual({
      label: '需要人工确认的特殊返回内...',
      type: 'danger'
    })
  })
})
