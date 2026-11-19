import { onMounted, onScopeDispose, ref } from 'vue'

export function useMediaQuery(query: string) {
  const matches = ref(false)
  let media: MediaQueryList | undefined
  const sync = () => { matches.value = Boolean(media?.matches) }
  onMounted(() => {
    if (!window.matchMedia) return
    media = window.matchMedia(query)
    sync()
    media.addEventListener('change', sync)
  })
  onScopeDispose(() => media?.removeEventListener('change', sync))
  return matches
}
