import { onScopeDispose, ref } from 'vue'

/** Keep existing data visible, and never let an older read replace a newer one. */
export function useLatestRequest() {
  const loading = ref(false)
  let version = 0
  let disposed = false

  const execute = async <T>(request: () => Promise<T>, apply: (value: T) => void, onError: (error: unknown) => void) => {
    const current = ++version
    loading.value = true
    try {
      const value = await request()
      if (!disposed && current === version) apply(value)
    } catch (error) {
      if (!disposed && current === version) onError(error)
    } finally {
      if (!disposed && current === version) loading.value = false
    }
  }

  onScopeDispose(() => { disposed = true; version++ })
  return { loading, execute }
}
