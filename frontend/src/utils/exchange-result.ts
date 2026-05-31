import type { TagProps } from 'element-plus'

export interface ExchangeResultDisplay {
  label: string
  type: TagProps['type']
}

const cleanResultMessage = (message?: string) => {
  return `${message ?? ''}`
    .replace(/\s+/g, ' ')
    .trim()
}

export const formatExchangeResult = (message?: string, status?: string): ExchangeResultDisplay => {
  const raw = cleanResultMessage(message)
  const lower = raw.toLowerCase()

  if (!raw) {
    return { label: status === 'success' ? '成功' : '-', type: status === 'success' ? 'success' : 'info' }
  }

  if (status === 'success' || raw.includes('兑换成功') || raw.includes('成功')) {
    return { label: '兑换成功', type: 'success' }
  }

  if (raw.includes('重复兑奖') || raw.includes('重复兑换') || raw.includes('已兑换') || raw.includes('今日已兑换') || raw.includes('本月已兑换')) {
    return { label: '已兑换过', type: 'warning' }
  }

  if (raw.includes('未登录') || raw.includes('登录') || raw.includes('JWT') || raw.includes('Token') || raw.includes('账号已失效') || raw.includes('认证为空')) {
    return { label: '账号登录失效', type: 'danger' }
  }

  if (raw.includes('无库存') || raw.includes('库存不足') || raw.includes('已兑完') || raw.includes('已耗尽') || raw.includes('单日已耗尽')) {
    return { label: '库存已抢完', type: 'warning' }
  }

  if (raw.includes('奖品不在线') || raw.includes('不在线') || raw.includes('已下架')) {
    return { label: '商品暂不可兑', type: 'warning' }
  }

  if (raw.includes('云朵不足') || raw.includes('余额不足')) {
    return { label: '云朵不足', type: 'warning' }
  }

  if (raw.includes('商品ID') || raw.includes('prizeId') || raw.includes('http_status=404') || lower.includes('404 not found')) {
    return { label: '商品信息需更新', type: 'warning' }
  }

  if (raw.includes('活动异常') || raw.includes('Error Code')) {
    return { label: '活动异常', type: 'warning' }
  }

  if (raw.includes('请求失败') || raw.includes('请求返回异常') || raw.includes('超时') || lower.includes('timeout')) {
    return { label: '网络异常', type: 'danger' }
  }

  if (raw.includes('错误') || raw.includes('失败')) {
    return { label: '兑换失败', type: 'danger' }
  }

  const brief = raw.split('|')[0]?.trim() || raw
  return { label: brief.length > 12 ? `${brief.slice(0, 12)}...` : brief, type: status === 'failed' ? 'danger' : 'info' }
}
