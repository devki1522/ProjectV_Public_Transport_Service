import { createContext, useContext } from 'react'

export const RouterContext = createContext(null)

/**
 * Validates an internal redirect target to prevent open redirects.
 * Only relative application paths beginning with a single "/" are accepted.
 */
export function sanitizeRedirectPath(rawPath, fallback = '/admin') {
  if (typeof rawPath !== 'string') return fallback
  const trimmed = rawPath.trim()
  if (!trimmed.startsWith('/') || trimmed.startsWith('//') || trimmed.includes('\\')) {
    return fallback
  }
  for (let i = 0; i < trimmed.length; i += 1) {
    const code = trimmed.charCodeAt(i)
    if (code < 0x20 || code === 0x7f) return fallback
  }
  try {
    const parsed = new URL(trimmed, 'https://transit.local')
    if (parsed.origin !== 'https://transit.local') return fallback
    const normalized = `${parsed.pathname}${parsed.search}${parsed.hash}`
    if (parsed.pathname === '/login' || parsed.pathname === '/unauthorized') {
      return fallback
    }
    return normalized
  } catch {
    return fallback
  }
}

export function useRouter() {
  const ctx = useContext(RouterContext)
  if (!ctx) {
    throw new Error('useRouter must be used inside RouterProvider')
  }
  return ctx
}
