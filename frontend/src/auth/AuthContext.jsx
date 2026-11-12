import { useCallback, useEffect, useMemo, useState } from 'react'
import { fetchSession, loginWithCredentials, logoutSession } from '../api/auth.js'
import { sanitizeRedirectPath, useRouter } from '../lib/routerState.js'
import { AuthContext, canAdminister } from './authState.js'

function needsAuthSession(pathname) {
  return pathname === '/login' || pathname === '/unauthorized' || pathname === '/admin' || pathname.startsWith('/admin/')
}

export function AuthProvider({ children }) {
  const { pathname, search, navigate } = useRouter()
  const [logoutPending, setLogoutPending] = useState(false)
  const [logoutError, setLogoutError] = useState(null)
  const [state, setState] = useState(() => ({
    status: needsAuthSession(pathname) ? 'initializing' : 'idle',
    user: null,
    expiresAtUtc: null,
    csrfToken: null,
    isAdmin: false,
  }))

  const restoreSession = useCallback(async (signal) => {
    try {
      const data = await fetchSession(signal)
      setState({
        status: 'authenticated',
        user: data.user,
        expiresAtUtc: data.expiresAtUtc ?? null,
        csrfToken: data.csrfToken,
        isAdmin: canAdminister(data),
      })
      return data
    } catch (error) {
      if (error.name === 'AbortError') return null
      setState({
        status: 'unauthenticated',
        user: null,
        expiresAtUtc: null,
        csrfToken: null,
        isAdmin: false,
      })
      return null
    }
  }, [])

  useEffect(() => {
    if (!needsAuthSession(pathname)) {
      return undefined
    }
    if (state.status !== 'idle' && state.status !== 'initializing') {
      return undefined
    }
    const controller = new AbortController()
    let active = true
    fetchSession(controller.signal)
      .then((data) => {
        if (!active) return
        setState({
          status: 'authenticated',
          user: data.user,
          expiresAtUtc: data.expiresAtUtc ?? null,
          csrfToken: data.csrfToken,
          isAdmin: canAdminister(data),
        })
      })
      .catch((error) => {
        if (!active || error.name === 'AbortError') return
        setState({
          status: 'unauthenticated',
          user: null,
          expiresAtUtc: null,
          csrfToken: null,
          isAdmin: false,
        })
      })
    return () => {
      active = false
      controller.abort()
    }
  }, [pathname, state.status])

  const login = useCallback(async ({ username, password }) => {
    const data = await loginWithCredentials({ username, password })
    setState({
      status: 'authenticated',
      user: data.user,
      expiresAtUtc: data.expiresAtUtc ?? null,
      csrfToken: data.csrfToken,
      isAdmin: canAdminister(data),
    })
    return data
  }, [])

  const clearSession = useCallback(() => {
    setState({
      status: 'unauthenticated',
      user: null,
      expiresAtUtc: null,
      csrfToken: null,
      isAdmin: false,
    })
  }, [])

  const logout = useCallback(async () => {
    if (logoutPending) return
    setLogoutPending(true)
    setLogoutError(null)
    try {
      await logoutSession(state.csrfToken)
      clearSession()
      navigate('/login', { replace: true })
    } catch (error) {
      setLogoutError(error.message)
    } finally {
      setLogoutPending(false)
    }
  }, [state.csrfToken, clearSession, navigate, logoutPending])

  const handleApiError = useCallback((error) => {
    if (!error) return
    if (error.status === 401) {
      clearSession()
      if (pathname !== '/login') {
        const current = sanitizeRedirectPath(`${pathname}${search}`, '/admin')
        navigate(`/login?next=${encodeURIComponent(current)}`, { replace: true })
      }
      return
    }
    if (error.status === 403) {
      navigate('/unauthorized', { replace: true })
    }
  }, [clearSession, navigate, pathname, search])

  useEffect(() => {
    if (state.status !== 'authenticated' || !state.expiresAtUtc) return undefined
    const remaining = Date.parse(state.expiresAtUtc) - Date.now()
    if (!Number.isFinite(remaining)) return undefined
    const expire = () => {
      if (needsAuthSession(pathname)) handleApiError({ status: 401 })
      else clearSession()
    }
    if (remaining <= 0) {
      expire()
      return undefined
    }
    const timer = setTimeout(expire, Math.min(remaining, 2_147_483_647))
    return () => clearTimeout(timer)
  }, [state.status, state.expiresAtUtc, handleApiError, pathname, clearSession])

  const value = useMemo(() => ({
    status: state.status,
    isInitializing: state.status === 'initializing' || state.status === 'idle',
    isAuthenticated: state.status === 'authenticated' && state.user !== null,
    isAdmin: state.status === 'authenticated' && state.isAdmin,
    user: state.user,
    expiresAtUtc: state.expiresAtUtc,
    csrfToken: state.csrfToken,
    login,
    logout,
    logoutPending,
    logoutError,
    clearSession,
    restoreSession,
    handleApiError,
  }), [state, login, logout, logoutPending, logoutError, clearSession, restoreSession, handleApiError])

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
