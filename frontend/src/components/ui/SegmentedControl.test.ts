import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import SegmentedControl from './SegmentedControl.vue'

describe('segmented control', () => {
  beforeEach(() => vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} }))
  afterEach(() => vi.unstubAllGlobals())
  const options = [{ label: '全部', value: '' }, { label: '成功', value: 'success' }, { label: '失败', value: 'failed' }]
  it('emits the exact selected value and reflects the controlled selection', async () => {
    const wrapper = mount(SegmentedControl, { props: { modelValue: '', options } })
    await wrapper.findAll('button')[2].trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([['failed']])
    await wrapper.setProps({ modelValue: 'failed' })
    expect(wrapper.findAll('button')[2].attributes('aria-pressed')).toBe('true')
    expect(wrapper.findAll('button')[0].attributes('aria-pressed')).toBe('false')
    wrapper.unmount()
  })
  it('supports keyboard navigation, wrapping and Home/End', async () => {
    const wrapper = mount(SegmentedControl, { props: { modelValue: '', options } })
    await wrapper.trigger('keydown', { key: 'ArrowLeft' })
    await wrapper.trigger('keydown', { key: 'ArrowRight' })
    await wrapper.trigger('keydown', { key: 'End' })
    await wrapper.trigger('keydown', { key: 'Home' })
    expect(wrapper.emitted('update:modelValue')).toEqual([['failed'], ['success'], ['failed'], ['']])
    wrapper.unmount()
  })
})
