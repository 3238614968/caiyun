<template>
  <el-dialog
    v-model="visible"
    class="command-dialog"
    :show-close="false"
    :width="'min(580px, calc(100vw - 24px))'"
    top="18vh"
    aria-label="快捷导航"
    @closed="query = ''; selected = 0"
    @opened="input?.focus()"
  >
    <div class="command-search">
      <el-icon><Search /></el-icon>
      <input
        ref="input"
        v-model="query"
        placeholder="搜索页面、功能与操作…"
        aria-label="搜索页面、功能与操作"
        role="combobox"
        aria-autocomplete="list"
        :aria-expanded="true"
        aria-controls="command-results"
        :aria-activedescendant="`command-${selected}`"
        @keydown="handleKey"
      >
      <button
        type="button"
        class="command-escape"
        @click="visible = false"
      >
        Esc
      </button>
    </div>
    <div
      id="command-results"
      class="command-results"
      role="listbox"
      aria-label="快捷导航结果"
    >
      <span class="command-group-label">{{ query ? '搜索结果' : '快速前往' }}</span>
      <button
        v-for="(item, index) in filtered"
        :id="`command-${index}`"
        :key="item.id"
        type="button"
        role="option"
        :aria-selected="selected === index"
        class="command-item"
        @pointermove="selected = index"
        @click="choose(item)"
      >
        <span class="command-item-icon"><el-icon><component :is="item.icon" /></el-icon></span>
        <span class="command-item-copy"><strong>{{ item.label }}</strong><small>{{ item.description }}</small></span>
        <span class="command-item-key">{{ selected === index ? '↵' : item.shortcut }}</span>
      </button>
      <div
        v-if="!filtered.length"
        class="command-empty"
      >
        没有找到“{{ query }}”<span>试试搜索“账号”“兑换”或“设置”</span>
      </div>
    </div>
    <div class="command-footer">
      <span><kbd>↑</kbd><kbd>↓</kbd> 选择</span><span><kbd>↵</kbd> 打开</span><span class="command-brand">移动云盘 · 快捷导航</span>
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Search, DataLine, User, List, Shop, Setting, Plus, Bell, UserFilled } from '@element-plus/icons-vue'

const visible = defineModel<boolean>({ required: true })
const props = defineProps<{ isAdmin: boolean }>()
const emit = defineEmits<{ profile: []; settings: [] }>()
const router = useRouter()
const query = ref('')
const selected = ref(0)
const input = ref<HTMLInputElement>()
const items = computed(() => [
  { id: 'dashboard', label: '首页', description: '云朵余额、收益与账号动态', path: '/dashboard', icon: DataLine, shortcut: 'Alt 1' },
  { id: 'accounts', label: '账号管理', description: '管理账号、凭据与自动任务', path: '/accounts', icon: User, shortcut: 'Alt 2' },
  { id: 'logs', label: '运行日志', description: '查找任务记录与执行结果', path: '/logs', icon: List, shortcut: 'Alt 3' },
  { id: 'exchange', label: '兑换中心', description: '浏览商品、安排抢兑、查看奖品', path: '/exchange', icon: Shop, shortcut: 'Alt 4' },
  ...(props.isAdmin ? [{ id: 'admin', label: '管理员面板', description: '全局配置、用户与任务管理', path: '/admin', icon: Setting, shortcut: 'Alt 5' }, { id: 'announcements', label: '公告管理', description: '发布与维护站内通知', path: '/announcements', icon: Bell, shortcut: '' }] : []),
  { id: 'add-account', label: '添加账号', description: '通过 CK 或短信添加云盘账号', path: '/accounts?new=1', icon: Plus, shortcut: '' },
  { id: 'profile', label: '个人中心', description: '查看个人资料与服务状态', path: '', icon: UserFilled, shortcut: '' },
  { id: 'settings', label: '系统设置', description: '调整界面密度、动效与实时连接', path: '', icon: Setting, shortcut: '' }
])
type Item = typeof items.value[number]
const filtered = computed(() => items.value.filter(item => `${item.label}${item.description}${item.id}`.toLowerCase().includes(query.value.trim().toLowerCase())))
watch(query, () => { selected.value = 0 })
watch(visible, value => { if (value) void nextTick(() => input.value?.focus()) })
const choose = async (item: Item) => {
  visible.value = false
  await nextTick()
  if (item.path) void router.push(item.path)
  else if (item.id === 'profile') emit('profile')
  else emit('settings')
}
const handleKey = (event: KeyboardEvent) => {
  if (event.isComposing) return
  if (event.key === 'Enter' && filtered.value[selected.value]) { event.preventDefault(); void choose(filtered.value[selected.value]); return }
  if (!['ArrowDown', 'ArrowUp'].includes(event.key) || !filtered.value.length) return
  event.preventDefault()
  selected.value = (selected.value + (event.key === 'ArrowDown' ? 1 : -1) + filtered.value.length) % filtered.value.length
  void nextTick(() => document.getElementById(`command-${selected.value}`)?.scrollIntoView({ block: 'nearest' }))
}
</script>

<style>
.command-dialog.el-dialog { padding: 0; border-radius: 16px; overflow: hidden; }
.command-dialog .el-dialog__header { display: none; }
.command-dialog .el-dialog__body { padding: 0; overflow: hidden; }
.command-search { display: flex; align-items: center; gap: 12px; padding: 19px 20px; border-bottom: 1px solid var(--line); color: var(--text-muted); }
.command-search input { border: 0; outline: 0; background: none; width: 100%; font: inherit; color: var(--text); }
.command-escape { border: 1px solid var(--line); color: var(--text-muted); padding: 3px 6px; border-radius: 5px; background: var(--surface-subtle); font-size: 11px; cursor: pointer; }
.command-results { max-height: 400px; overflow: auto; padding: 8px; }
.command-group-label { display: block; padding: 10px 12px; color: var(--text-muted); font-size: 11px; }
.command-item { border: 0; display: flex; align-items: center; gap: 12px; width: 100%; padding: 11px 12px; border-radius: 8px; background: none; color: var(--text); text-align: left; cursor: pointer; transition: background .12s; }
.command-item[aria-selected="true"] { background: var(--surface-subtle); }
.command-item-icon { display: grid; place-items: center; width: 34px; height: 34px; border: 1px solid var(--line); border-radius: 8px; background: white; }
.command-item-copy { display: grid; gap: 4px; }
.command-item-copy strong { font-size: 13px; font-weight: 550; }
.command-item-copy small { color: var(--text-muted); font-size: 11px; }
.command-item-key { margin-left: auto; color: var(--text-muted); font-size: 12px; }
.command-footer { display: flex; gap: 16px; padding: 12px 20px; border-top: 1px solid var(--line); font-size: 11px; color: var(--text-muted); }
.command-footer kbd { padding: 2px 4px; background: var(--surface-subtle); border-radius: 3px; margin-right: 3px; font: inherit; }
.command-brand { margin-left: auto; }
.command-empty { padding: 34px 16px; text-align: center; color: var(--text-secondary); font-size: 13px; }
.command-empty span { display: block; margin-top: 9px; font-size: 11px; color: var(--text-muted); }
</style>
