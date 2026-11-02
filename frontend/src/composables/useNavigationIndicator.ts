import { nextTick, onMounted, onScopeDispose, ref, watch, type Ref } from 'vue'

export function useNavigationIndicator(container: Ref<HTMLElement | undefined>, active: Ref<string>) {
  const style = ref<Record<string, string>>({ opacity: '0' })
  let observer: ResizeObserver | undefined
  let frame = 0
  const measure = () => {
    cancelAnimationFrame(frame)
    frame = requestAnimationFrame(() => {
      const root = container.value
      const item = root?.querySelector<HTMLElement>('.el-menu-item.is-active')
      if (!root || !item) { style.value = { opacity: '0' }; return }
      const bounds = root.getBoundingClientRect()
      const target = item.getBoundingClientRect()
      style.value = {
        opacity: '1', width: `${target.width}px`, height: `${target.height}px`,
        transform: `translate3d(${target.left - bounds.left + root.scrollLeft}px, ${target.top - bounds.top + root.scrollTop}px, 0)`
      }
    })
  }
  watch(active, () => { void nextTick(measure) }, { flush: 'post' })
  onMounted(() => {
    observer = new ResizeObserver(measure)
    if (container.value) observer.observe(container.value)
    measure()
  })
  onScopeDispose(() => { observer?.disconnect(); cancelAnimationFrame(frame) })
  return { indicatorStyle: style, measureIndicator: measure }
}
