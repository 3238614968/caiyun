export const taskTypeNameMap: Record<string, string> = {
  signin: '签到',
  task_expansion_reward: '翻倍奖励',
  cloud_multiple: '云朵翻倍',
  wechat: '微信',
  wxdraw: '微信抽奖',
  tasklist: '任务列表',
  mail_mutual: '139邮箱账号互发',
  invitefriends: '邀请好友',
  shake: '摇一摇',
  receive: '领取云朵',
  messagepush: '消息推送',
  revivalreward: '复活卡奖励',
  backupgift: '备份礼包',
  token_pk: '算力大作战',
  make_wish: '许愿赢好礼',
  makewish_exchange: '许愿AI豆兑换',
  fun_ai: '趣玩AI抽奖',
  poster_activity: '校园海报活动',
  mutual_assist: '多账号活动互助',
  hidden_rewards: '隐藏活动奖励',
  notice_switch: '通知开关同步',
  student_perks: '学生认证福利',
  prize_center: '领奖专区盘点',
  mcloud_day: '会员日',
  meitu_backup: '美图备份好礼',
  red_invite: '红包邀请',
  unloading_1t: '1T新礼',
  rafflecode: '抽奖码',
  family_circle: '家庭圈任务',
  ai_store: 'AI Store作品保存',
  album_backup_report: '相册备份状态上报',
  fun_ai_mail: '趣玩AI邮箱版',
  upgrade_gift: '焕新权益',
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
  'mail_mutual',
  'invitefriends',
  'shake',
  'receive',
  'messagepush',
  'revivalreward',
  'backupgift',
  'token_pk',
  'make_wish',
  'makewish_exchange',
  'fun_ai',
  'poster_activity',
  'mutual_assist',
  'hidden_rewards',
  'notice_switch',
  'student_perks',
  'prize_center',
  'mcloud_day',
  'meitu_backup',
  'red_invite',
  'unloading_1t',
  'rafflecode',
  'family_circle',
  'ai_store',
  'album_backup_report',
  'fun_ai_mail',
  'upgrade_gift',
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
