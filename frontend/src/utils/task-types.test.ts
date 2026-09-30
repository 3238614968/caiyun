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
  it('includes every newly registered activity with a localized label', () => {
    for (const code of ['notice_switch', 'student_perks', 'prize_center', 'mcloud_day', 'meitu_backup', 'red_invite', 'unloading_1t', 'rafflecode', 'family_circle', 'ai_store', 'album_backup_report', 'fun_ai_mail', 'upgrade_gift']) {
      expect(getTaskTypeName(code)).not.toBe(code)
      expect(taskTypeOptions.some(option => option.value === code)).toBe(true)
    }
  })
  it('contains the all option and keeps cloud_multiple localized', () => {
    expect(taskTypeOptions[0]).toEqual({ label: '全部', value: '' })
    expect(taskTypeOptions).toContainEqual({ label: '云朵翻倍', value: 'cloud_multiple' })
  })
})
