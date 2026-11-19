const lockName = 'caiyun:session-refresh'

// Web Locks serialize refreshes across tabs. IndexedDB provides the same
// exclusion on HTTP origins where Web Locks are unavailable. This dedicated
// database stores no credentials; an open read/write transaction is the lock.
export async function withSessionRefreshLock<T>(action: () => Promise<T>): Promise<T> {
  if (navigator.locks) {
    return navigator.locks.request(lockName, action)
  }
  return withDatabaseLock(action)
}

async function withDatabaseLock<T>(action: () => Promise<T>): Promise<T> {
  const db = await new Promise<IDBDatabase>((resolve, reject) => {
    let blocked = false
    const request = indexedDB.open(lockName, 1)
    request.onupgradeneeded = () => request.result.createObjectStore('mutex')
    request.onsuccess = () => {
      if (blocked) request.result.close()
      else resolve(request.result)
    }
    request.onerror = () => reject(request.error)
    request.onblocked = () => {
      blocked = true
      reject(new Error('Session coordination is blocked'))
    }
  })
  try {
    return await new Promise<T>((resolve, reject) => {
      const transaction = db.transaction('mutex', 'readwrite')
      const store = transaction.objectStore('mutex')
      let started = false
      let finished = false
      let failed = false
      let result: T
      let failure: unknown

      // Keep a request pending until the network operation settles, so the
      // transaction cannot auto-commit and admit another tab mid-refresh.
      // Closing/crashing the tab automatically releases the transaction.
      const hold = () => {
        const request = store.get('lock')
        request.onsuccess = () => {
          if (!started) {
            started = true
            void Promise.resolve().then(action).then(value => {
              result = value
            }, error => {
              failed = true
              failure = error
            }).finally(() => {
              finished = true
            })
          }
          if (!finished) hold()
        }
      }
      transaction.oncomplete = () => failed ? reject(failure) : resolve(result)
      transaction.onabort = () => reject(transaction.error || new Error('Session coordination aborted'))
      transaction.onerror = () => reject(transaction.error)
      hold()
    })
  } finally {
    db.close()
  }
}
