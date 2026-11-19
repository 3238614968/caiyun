<template>
  <div
    ref="root"
    class="segmented-control"
    role="group"
    :aria-label="label"
    @keydown="handleKey"
  >
    <span
      class="segment-indicator"
      :style="indicator"
      aria-hidden="true"
    />
    <button
      v-for="option in options"
      :key="option.value"
      type="button"
      :aria-pressed="modelValue === option.value"
      @click="$emit('update:modelValue', option.value)"
    >
      {{ option.label }}<span
        v-if="option.count !== undefined"
        class="segment-count"
      >{{ option.count }}</span>
    </button>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onMounted, onScopeDispose, ref, watch } from 'vue'

type Value = string | number
const props = withDefaults(defineProps<{ modelValue: Value; options: { label: string; value: Value; count?: number }[]; label?: string }>(), { label: '切换视图' })
const emit = defineEmits<{ 'update:modelValue': [value: Value] }>()
const root = ref<HTMLElement>()
const indicator = ref<Record<string, string>>({ opacity: '0' })
let observer: ResizeObserver | undefined
const measure = () => {
  const active = root.value?.querySelector<HTMLElement>('[aria-pressed="true"]')
  if (!active) { indicator.value = { opacity: '0' }; return }
  indicator.value = { opacity: '1', width: `${active.offsetWidth}px`, transform: `translateX(${active.offsetLeft}px)` }
}
watch([() => props.modelValue, () => props.options], () => { void nextTick(measure) }, { deep: true })
onMounted(() => {
  observer = new ResizeObserver(measure)
  if (root.value) observer.observe(root.value)
  measure()
})
onScopeDispose(() => observer?.disconnect())
const handleKey = (event: KeyboardEvent) => {
  if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key) || !props.options.length) return
  event.preventDefault()
  const index = props.options.findIndex(option => option.value === props.modelValue)
  const target = event.key === 'Home' ? 0 : event.key === 'End' ? props.options.length - 1 : (index + (event.key === 'ArrowRight' ? 1 : -1) + props.options.length) % props.options.length
  emit('update:modelValue', props.options[target].value)
  root.value?.querySelectorAll('button')[target]?.focus()
}
</script>

<style scoped>
.segmented-control { display: inline-flex; align-items: center; gap: 2px; position: relative; padding: 4px; border: 1px solid var(--line); border-radius: 10px; background: var(--surface-subtle); max-width: 100%; isolation: isolate; }
.segmented-control button { position: relative; z-index: 1; border: 0; background: none; border-radius: 7px; padding: 7px 13px; font: inherit; font-size: 12px; line-height: 1.4; font-weight: 500; color: var(--text-secondary); cursor: pointer; white-space: nowrap; transition: color .18s; }
.segmented-control button:hover, .segmented-control button[aria-pressed="true"] { color: var(--text); }
.segment-indicator { position: absolute; left: 0; top: 4px; bottom: 4px; border-radius: 7px; background: var(--surface); box-shadow: 0 1px 4px #17251c14, 0 0 0 1px #17251c06; transition: transform .28s cubic-bezier(.2,.8,.2,1), width .28s cubic-bezier(.2,.8,.2,1); }
.segment-count { margin-left: 7px; opacity: .55; font-variant-numeric: tabular-nums; }
</style>
