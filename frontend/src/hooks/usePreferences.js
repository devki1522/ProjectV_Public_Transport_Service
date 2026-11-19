import { useCallback, useEffect, useState } from 'react'

export function useStoredPreference(key, fallback) {
  const [value, setValue] = useState(() => {
    try { return JSON.parse(localStorage.getItem(key)) ?? fallback } catch { return fallback }
  })
  const update = useCallback((next) => setValue((previous) => {
    const resolved = typeof next === 'function' ? next(previous) : next
    try { localStorage.setItem(key, JSON.stringify(resolved)) } catch { /* Storage can be unavailable in private sessions. */ }
    return resolved
  }), [key])
  return [value, update]
}

export function useOnline() {
  const [online, setOnline] = useState(() => navigator.onLine)
  useEffect(() => {
    const update = () => setOnline(navigator.onLine)
    window.addEventListener('online', update)
    window.addEventListener('offline', update)
    return () => { window.removeEventListener('online', update); window.removeEventListener('offline', update) }
  }, [])
  return online
}
