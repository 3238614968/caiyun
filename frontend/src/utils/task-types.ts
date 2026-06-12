export const taskTypeNameMap: Record<string, string> = {
  signin: '签到',
  task_expansion_reward: '翻倍奖励',
  cloud_multiple: '云朵翻倍',
  wechat: '微信',
  wxdraw: '微信抽奖',
  tasklist: '任务列表',
  invitefriends: '邀请好友',
  shake: '摇一摇',
  receive: '领取云朵',
  messagepush: '消息推送',
  revivalreward: '复活卡奖励',
  backupgift: '备份礼包',
  after_task: '收尾',
  todaycloud: '今日云朵',
  aicloud: 'AI云朵',
  redpacket: 'AI红包',
  cloudbattle: '云朵大战',
  cloudphone: '云手机红包',
  exchange: '兑换',
  store: '月卡兑换',
  garden: '果园',
  blindbox: '盲盒',
  all: '全部任务',
  trigger_all: '全部任务'
}

const orderedTaskTypes = [
  'signin',
  'task_expansion_reward',
  'cloud_multiple',
  'wechat',
  'wxdraw',
  'tasklist',
  'invitefriends',
  'shake',
  'receive',
  'messagepush',
  'revivalreward',
  'backupgift',
  'after_task',
  'todaycloud',
  'aicloud',
  'redpacket',
  'cloudbattle',
  'cloudphone',
  'exchange',
  'store',
  'garden',
  'blindbox'
] as const

export function getTaskTypeName(type?: string | null, fallbackName?: string | null): string {
  const normalizedType = `${type ?? ''}`.trim()
  const normalizedFallback = `${fallbackName ?? ''}`.trim()
  if (!normalizedType) {
    return normalizedFallback || '-'
  }
  return taskTypeNameMap[normalizedType] || normalizedFallback || normalizedType
}

export const taskTypeOptions = [
  { label: '全部', value: '' },
  ...orderedTaskTypes.map(value => ({
    label: getTaskTypeName(value),
    value
  }))
]
