import { useCallback, useEffect, useState } from 'react'

// A request belongs to its key. Abort plus the active guard prevent older
// requests (and StrictMode's cleanup) from replacing a newer selection.
export function useResource(key, load, refreshInterval = 0) {
  const [revision, setRevision] = useState(0)
  const [state, setState] = useState({ key: null, revision: -1, data: null, error: null, updatedAt: null })
  const retry = useCallback(() => setRevision((value) => value + 1), [])

  useEffect(() => {
    if (!key) return
    const controller = new AbortController()
    let active = true
    load(controller.signal)
      .then((data) => {
        if (active) setState({ key, revision, data, error: null, updatedAt: new Date() })
      })
      .catch((error) => {
        if (active && error.name !== 'AbortError') setState((previous) => ({
          key, revision, error, data: previous.key === key ? previous.data : null,
          updatedAt: previous.key === key ? previous.updatedAt : null,
        }))
      })
    return () => { active = false; controller.abort() }
  }, [key, load, revision])

  useEffect(() => {
    if (!key || !refreshInterval) return
    const timer = setInterval(() => {
      if (document.visibilityState === 'visible' && navigator.onLine) retry()
    }, refreshInterval)
    const resume = () => { if (document.visibilityState === 'visible' && navigator.onLine) retry() }
    window.addEventListener('online', resume)
    document.addEventListener('visibilitychange', resume)
    return () => {
      clearInterval(timer)
      window.removeEventListener('online', resume)
      document.removeEventListener('visibilitychange', resume)
    }
  }, [key, refreshInterval, retry])

  const pending = Boolean(key) && (state.key !== key || state.revision !== revision)
  if (state.key !== key) return { data: null, error: null, pending, updatedAt: null, retry }
  return { ...state, error: pending ? null : state.error, pending, retry }
}
