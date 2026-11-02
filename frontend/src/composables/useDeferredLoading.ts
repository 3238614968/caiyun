import { onScopeDispose, ref, watch, type Ref } from 'vue'

/** Fast reads finish without flashing an overlay; slower ones still give feedback. */
export function useDeferredLoading(pending: Ref<boolean>, delay = 120) {
  const visible = ref(false)
  let timer: ReturnType<typeof setTimeout> | undefined
  watch(pending, value => {
    clearTimeout(timer)
    if (!value) visible.value = false
    else timer = setTimeout(() => { visible.value = true }, delay)
  }, { immediate: true, flush: 'sync' })
  onScopeDispose(() => clearTimeout(timer))
  return visible
}
