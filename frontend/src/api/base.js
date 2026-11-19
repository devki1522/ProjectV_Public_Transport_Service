const rawVersion = (import.meta.env.VITE_API_VERSION || 'v1').trim().replace(/^\/+|\/+$/g, '')

export const API_VERSION = rawVersion || 'v1'
export const API_BASE = `/api/transport/${API_VERSION}`

export function apiPath(path) {
  const normalized = path.startsWith('/') ? path : `/${path}`
  return `${API_BASE}${normalized}`
}
