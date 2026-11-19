// Older notifications live in localStorage and do not pass through API DTOs.
// Clean them when loading as well as when receiving a new event.
export const formatNotificationMessage = (message: string, exchange = false): string => {
  const diagnosticTail = /(?:^|[|;；]\s*)(?:http_status|code|result_code|result|desc|sub_msg|trace_id|body|raw_body|response_body|build|commit|built_at|offset|confidence|method|peak_margin|solver|solve_ms|session|exchangeCode)\s*=|[，,；;]?\s*滑块识别\s*offset\s*=/i
  const readable = message.replace(/exchangeCode\s*=\s*[a-z0-9_-]+\s*[（(]([^）)]+)[）)]/gi, '$1')
  const clean = readable.split(diagnosticTail)[0].trim().replace(/[，,；;|]+$/, '').trim()
  if (!exchange) return clean || '执行结果待确认'

  // Keep the account and product prefix attached by NotificationCenter.
  const firstColon = clean.indexOf('：')
  const colon = firstColon >= 0 && !/Error\s*Code/i.test(clean.slice(0, firstColon)) ? firstColon : -1
  const prefix = colon >= 0 ? clean.slice(0, colon + 1) : ''
  let result = colon >= 0 ? clean.slice(colon + 1).trim() : clean
  if (/云朵数不足|云朵数量不足|云朵不足|云朵不够|余额不足/.test(result)) {
    result = '云朵不足，兑换失败'
  } else if (/Error\s*Code\s*[：:]\s*GK/i.test(result)) {
    result = '活动验证未通过，请稍后重试'
  } else if (/滑块识别置信度|滑块验证码识别失败|滑块匹配存在歧义/.test(result)) {
    result = '验证未通过，请重试'
  } else if (result.includes('兑换资格校验未通过')) {
    result = result.replace('兑换资格校验未通过', '兑换条件未满足')
  } else if (/请求失败|请求返回异常|读取响应失败|timeout|deadline exceeded/i.test(result)) {
    result = '网络异常，请稍后重试'
  }
  return prefix + (result || '执行结果待确认')
}
