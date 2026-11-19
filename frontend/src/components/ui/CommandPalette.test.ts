import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import CommandPalette from './CommandPalette.vue'

const { push } = vi.hoisted(() => ({ push: vi.fn() }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push }) }))
const dialog = { props: ['modelValue'], template: '<div v-if="modelValue"><slot /></div>' }

describe('workspace command palette', () => {
  beforeEach(() => push.mockReset())
  it('filters actions and opens the selected account creation flow', async () => {
    const wrapper = mount(CommandPalette, { props: { modelValue: true, isAdmin: false }, global: { stubs: { ElDialog: dialog } } })
    await wrapper.get('input').setValue('添加账号')
    expect(wrapper.findAll('[role="option"]')).toHaveLength(1)
    await wrapper.get('input').trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(push).toHaveBeenCalledWith('/accounts?new=1')
    expect(wrapper.emitted('update:modelValue')).toEqual([[false]])
    wrapper.unmount()
  })
  it('keeps administrative destinations out of ordinary users results', async () => {
    const wrapper = mount(CommandPalette, { props: { modelValue: true, isAdmin: false }, global: { stubs: { ElDialog: dialog } } })
    await wrapper.get('input').setValue('管理员')
    expect(wrapper.findAll('[role="option"]')).toHaveLength(0)
    await wrapper.get('input').trigger('keydown', { key: 'Enter' })
    expect(push).not.toHaveBeenCalled()
    wrapper.unmount()
  })
  it('navigates results with the arrow keys', async () => {
    const wrapper = mount(CommandPalette, { props: { modelValue: true, isAdmin: true }, global: { stubs: { ElDialog: dialog } } })
    await wrapper.get('input').trigger('keydown', { key: 'ArrowDown' })
    await wrapper.get('input').trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(push).toHaveBeenCalledWith('/accounts')
    wrapper.unmount()
  })
})
