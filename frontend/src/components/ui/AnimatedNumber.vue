<template>
  <span
    class="animated-number"
    :aria-label="formattedTarget"
  ><span aria-hidden="true">{{ display }}</span></span>
</template>

<script setup lang="ts">
import { computed, onScopeDispose, ref, watch } from 'vue'

const props = defineProps<{ value: number | string }>()
const current = ref(typeof props.value === 'number' ? props.value : 0)
const formattedTarget = computed(() => typeof props.value === 'number' ? props.value.toLocaleString('zh-CN') : props.value)
const display = computed(() => typeof props.value === 'number' ? Math.round(current.value).toLocaleString('zh-CN') : props.value)
let frame = 0
watch(() => props.value, value => {
  cancelAnimationFrame(frame)
  if (typeof value !== 'number' || !Number.isFinite(value)) return
  if (document.documentElement.dataset.reduceMotion === 'true' || window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) {
    current.value = value
    return
  }
  const from = current.value
  const start = performance.now()
  const tick = (now: number) => {
    const progress = Math.min((now - start) / 520, 1)
    current.value = from + (value - from) * (1 - Math.pow(1 - progress, 3))
    if (progress < 1) frame = requestAnimationFrame(tick)
  }
  frame = requestAnimationFrame(tick)
})
onScopeDispose(() => cancelAnimationFrame(frame))
</script>

<style scoped>
.animated-number { font-variant-numeric: tabular-nums; letter-spacing: -.045em; }
</style>
