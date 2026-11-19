import { describe, expect, it } from 'vitest'

import { getTaskTypeName, taskTypeOptions } from './task-types'

describe('getTaskTypeName', () => {
  it('maps known backend task type to Chinese display name', () => {
    expect(getTaskTypeName('cloud_multiple')).toBe('云朵翻倍')
    expect(getTaskTypeName('exchange')).toBe('兑换')
    expect(getTaskTypeName('mail_mutual')).toBe('139邮箱账号互发')
    expect(getTaskTypeName('mutual_assist')).toBe('多账号活动互助')
    expect(getTaskTypeName('hidden_rewards')).toBe('隐藏活动奖励')
    expect(getTaskTypeName('makewish_exchange')).toBe('许愿AI豆兑换')
  })

  it('uses fallback or original type for unknown values', () => {
    expect(getTaskTypeName('new_task', '新任务')).toBe('新任务')
    expect(getTaskTypeName('new_task')).toBe('new_task')
  })

  it('returns fallback placeholder for empty values', () => {
    expect(getTaskTypeName('', '备用名称')).toBe('备用名称')
    expect(getTaskTypeName(null)).toBe('-')
  })
})

describe('taskTypeOptions', () => {
  it('includes all Migu activities in the existing log filter format', () => {
    const expected = {
      migu_login: '咪咕登录即送',
      migu_signin: '咪咕每日签到',
      migu_new_user: '咪咕新人礼',
      migu_community: '咪咕浏览社区',
      migu_mobile: '咪咕前往移动APP',
      migu_noon: '咪咕饭点打卡',
      migu_night: '咪咕晚安打卡',
      migu_song: '咪咕听歌与领奖',
      migu_assets: '咪咕金币与音符'
    }
    for (const [value, label] of Object.entries(expected)) {
      expect(getTaskTypeName(value)).toBe(label)
      expect(taskTypeOptions.filter(option => option.value === value)).toEqual([{ value, label }])
    }
  })

  it('keeps COC families visible in log filters with distinct names', () => {
    const expected = {
      coc3: '中国移动权益市场',
      coc_web6: '中国移动7日打卡与网龄成长',
      coc3_scan: 'COC3活动目录扫描',
      activity_collect: '活动中心收藏'
    }
    for (const [value, label] of Object.entries(expected)) {
      expect(getTaskTypeName(value)).toBe(label)
      expect(taskTypeOptions.filter(option => option.value === value)).toEqual([{ value, label }])
    }
  })
  it('includes the October task additions', () => {
    for (const code of ['annual_bill', 'luckybag', 'redcavalry', 'fun_ai_timed_lottery', 'tasklist_gp', 'redeem_code']) {
      expect(getTaskTypeName(code)).not.toBe(code)
      expect(taskTypeOptions.some(option => option.value === code)).toBe(true)
    }
  })
  it('includes every newly registered activity with a localized label', () => {
    for (const code of ['notice_switch', 'student_perks', 'prize_center', 'mcloud_day', 'meitu_backup', 'red_invite', 'unloading_1t', 'rafflecode', 'family_circle', 'ai_store', 'album_backup_report', 'fun_ai_mail', 'upgrade_gift', 'lantern', 'liveflower', 'cloudphone', 'shake']) {
      expect(getTaskTypeName(code)).not.toBe(code)
      expect(taskTypeOptions.some(option => option.value === code)).toBe(true)
    }
  })
  it('contains the all option and keeps cloud_multiple localized', () => {
    expect(taskTypeOptions[0]).toEqual({ label: '全部', value: '' })
    expect(taskTypeOptions).toContainEqual({ label: '云朵翻倍', value: 'cloud_multiple' })
  })
})
