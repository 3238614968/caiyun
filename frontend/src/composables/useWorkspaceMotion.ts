import { onMounted, onScopeDispose, watch, type Ref } from 'vue'

/** Put the preference on the document so teleported dialogs and popovers share it. */
export function useWorkspaceMotion(reduceMotion: Ref<boolean>) {
  let media: MediaQueryList | undefined
  const sync = () => {
    document.documentElement.dataset.reduceMotion = String(reduceMotion.value || media?.matches || false)
  }
  watch(reduceMotion, sync)
  onMounted(() => {
    document.body.classList.add('caiyun-workspace')
    media = window.matchMedia('(prefers-reduced-motion: reduce)')
    media.addEventListener('change', sync)
    sync()
  })
  onScopeDispose(() => {
    media?.removeEventListener('change', sync)
    document.body.classList.remove('caiyun-workspace')
    delete document.documentElement.dataset.reduceMotion
  })
}
