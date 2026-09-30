const dateFormatter = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit' })
const expiryFormatter = new Intl.DateTimeFormat('zh-CN', { timeZone: 'Asia/Shanghai', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false })

const activityNames: Record<string, string> = {
  National_NewLoginGif: '登录有礼', National_MCloudDay: '会员日', National_MakeWish: '许愿赢好礼', National_playAI: '趣玩 AI', National_PlayAISpecial: '校园海报', National_v13gift: '云盘焕新权益', sign_in_3: '云朵中心', newsign_139mail: '139 邮箱', National_OpenLuckybag: '月度福袋'
}

export function prizeActivityLabel(marketID: string, name: string): string {
  if (name && !/^National_|^sign_|^newsign_/i.test(name)) return name
  return activityNames[marketID] || activityNames[name] || '移动云盘活动'
}

export function prizeExpiryMillis(value: string): number | null {
  if (!value) return null
  const normalized = value.replace(' ', 'T')
  const zoned = /(?:Z|[+-]\d{2}:?\d{2})$/i.test(normalized) ? normalized : `${normalized}+08:00`
  const millis = Date.parse(zoned)
  return Number.isFinite(millis) ? millis : null
}

export function prizeExpiresToday(value: string, now = Date.now()): boolean {
  const millis = prizeExpiryMillis(value)
  if (millis === null) return false
  const dateKey = (timestamp: number) => dateFormatter.format(timestamp)
  return dateKey(millis) === dateKey(now)
}

export function formatPrizeExpiry(value: string): string {
  const millis = prizeExpiryMillis(value)
  if (millis === null) return '未提供到期时间'
  return expiryFormatter.format(millis)
}
