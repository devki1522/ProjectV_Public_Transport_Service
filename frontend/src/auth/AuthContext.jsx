import { useCallback, useEffect, useMemo, useState } from 'react'
import { fetchSession, loginWithCredentials, logoutSession } from '../api/auth.js'
import { sanitizeRedirectPath, useRouter } from '../lib/routerState.js'
import { AuthContext, hasAdminRole } from './authState.js'

function needsAuthSession(pathname) {
  return pathname === '/login' || pathname === '/unauthorized' || pathname === '/admin' || pathname.startsWith('/admin/')
}

export function AuthProvider({ children }) {
  const { pathname, search, navigate } = useRouter()
  const [state, setState] = useState(() => ({
    status: needsAuthSession(pathname) ? 'initializing' : 'idle',
    user: null,
    expiresAtUtc: null,
    csrfToken: null,
  }))

  const restoreSession = useCallback(async (signal) => {
    try {
      const data = await fetchSession(signal)
      setState({
        status: 'authenticated',
        user: data.user,
        expiresAtUtc: data.expiresAtUtc ?? null,
        csrfToken: data.csrfToken,
      })
      return data
    } catch (error) {
      if (error.name === 'AbortError') return null
      setState({
        status: 'unauthenticated',
        user: null,
        expiresAtUtc: null,
        csrfToken: null,
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
        })
      })
      .catch((error) => {
        if (!active || error.name === 'AbortError') return
        setState({
          status: 'unauthenticated',
          user: null,
          expiresAtUtc: null,
          csrfToken: null,
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
    })
    return data
  }, [])

  const clearSession = useCallback(() => {
    setState({
      status: 'unauthenticated',
      user: null,
      expiresAtUtc: null,
      csrfToken: null,
    })
  }, [])

  const logout = useCallback(async () => {
    const token = state.csrfToken
    clearSession()
    await logoutSession(token)
    navigate('/login', { replace: true })
  }, [state.csrfToken, clearSession, navigate])

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

  const value = useMemo(() => ({
    status: state.status,
    isInitializing: state.status === 'initializing' || state.status === 'idle',
    isAuthenticated: state.status === 'authenticated' && state.user !== null,
    isAdmin: state.status === 'authenticated' && hasAdminRole(state.user),
    user: state.user,
    expiresAtUtc: state.expiresAtUtc,
    csrfToken: state.csrfToken,
    login,
    logout,
    clearSession,
    restoreSession,
    handleApiError,
  }), [state, login, logout, clearSession, restoreSession, handleApiError])

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
