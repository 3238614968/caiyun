import { describe, expect, it } from 'vitest'

import { getTaskTypeName, taskTypeOptions } from './task-types'

describe('getTaskTypeName', () => {
  it('maps known backend task type to Chinese display name', () => {
    expect(getTaskTypeName('cloud_multiple')).toBe('云朵翻倍')
    expect(getTaskTypeName('exchange')).toBe('兑换')
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
  it('contains the all option and keeps cloud_multiple localized', () => {
    expect(taskTypeOptions[0]).toEqual({ label: '全部', value: '' })
    expect(taskTypeOptions).toContainEqual({ label: '云朵翻倍', value: 'cloud_multiple' })
  })
})
