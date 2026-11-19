import { describe, expect, it } from 'vitest'
import { formatNotificationMessage } from './notification-message'

describe('customer notification messages', () => {
  it('cleans the reported cached notification without losing its product or account', () => {
    const raw = '小海 · QQ音乐绿钻会员月卡：您的云朵数不足 | http_status=200 | code=1001 | body={"code":1001,"msg":"您的云朵数不足"} | 滑块识别offset=376 confidence=0.8511 method=内置多特征NCC peak_margin=0.5681 solver=builtin solve_ms=93.76 session=bound-h5 资格 exchangeCode=0 | build=20261112-085058-current commit=ded8d5487b04-dirty built_at=2026-11-12T00:50:58Z'
    const display = formatNotificationMessage(raw, true)
    expect(display).toBe('小海 · QQ音乐绿钻会员月卡：云朵不足，兑换失败')
    expect(formatNotificationMessage(display, true)).toBe(display)
  })

  it('handles inline diagnostics and never treats successful recognition as exchange success', () => {
    expect(formatNotificationMessage('月卡：您的云朵数不足；滑块识别offset=376 confidence=0.8511；build=current', true)).toBe('月卡：云朵不足，兑换失败')
    expect(formatNotificationMessage('滑块识别offset=376 confidence=0.8511 资格 exchangeCode=0', true)).toBe('执行结果待确认')
    expect(formatNotificationMessage('月卡：活动异常 Error Code：GK | code=610', true)).toBe('月卡：活动验证未通过，请稍后重试')
    expect(formatNotificationMessage('活动异常 Error Code：GK | code=610', true)).toBe('活动验证未通过，请稍后重试')
    expect(formatNotificationMessage('月卡：兑换资格校验未通过：exchangeCode=2303（需开启自动备份） | build=current', true)).toBe('月卡：兑换条件未满足：需开启自动备份')
  })

  it('keeps business progress and already concise messages intact', () => {
    expect(formatNotificationMessage('共执行 3 个任务，获得 20 云朵，当前云朵: 200')).toBe('共执行 3 个任务，获得 20 云朵，当前云朵: 200')
    expect(formatNotificationMessage('COC3：服务端完成记录 0/3，抽奖次数未减少 | build=fixture')).toBe('COC3：服务端完成记录 0/3，抽奖次数未减少')
    expect(formatNotificationMessage('小海 · 月卡：兑换成功', true)).toBe('小海 · 月卡：兑换成功')
  })
})
