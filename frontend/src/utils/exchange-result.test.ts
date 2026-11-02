import { describe, expect, it } from 'vitest'

import { formatExchangeResult } from './exchange-result'

describe('formatExchangeResult', () => {
  it('does not mistake scheduler coordination for a redemption receipt', () => {
    const message = '10:00 同一账号同系列已安排任务「星巴克」，跳过「星巴克」，避免同月重复兑换'
    expect(formatExchangeResult(message, 'pending').label).toBe('本次已跳过')
    expect(formatExchangeResult('本月音乐类会员已有任务正在抢兑或已锁定，跳过本次执行', 'failed').label).toBe('本次已跳过')
  })

  it('ignores build metadata when classifying business results', () => {
    expect(formatExchangeResult('兑换成功 | build=Token修复 commit=fixture built_at=unknown', 'success').label).toBe('兑换成功')
    expect(formatExchangeResult('兑换资格查询失败，未提交兑换: code=505', 'failed').label).toBe('资格查询失败')
  })
  it('keeps the upstream GK rejection distinct from solver success', () => {
    expect(formatExchangeResult('活动异常 Error Code：GK | code=610 | offset=18', 'failed').label).toBe('活动服务拒绝（610/GK）')
  })

  it('distinguishes failed device authentication from an expired login', () => {
    expect(formatExchangeResult('设备认证失败，未提交兑换: request timeout', 'failed')).toEqual({
      label: '设备认证未完成',
      type: 'warning'
    })
  })

  it('reports eligibility restrictions separately from login or network failures', () => {
    expect(formatExchangeResult('兑换资格校验未通过：exchangeCode=2303（需开启自动备份）', 'failed').label).toBe('兑换资格未满足')
  })

  it('reports weak matches and rejected verification without claiming stock failure', () => {
    for (const message of ['滑块识别置信度不足，未提交兑换', '验证失败；confidence=0.5613']) {
      expect(formatExchangeResult(message, 'failed').label).toBe('滑块验证未通过')
    }
    expect(formatExchangeResult('奖品单日已耗尽；confidence=0.8627', 'failed').label).toBe('库存已抢完')
  })
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
