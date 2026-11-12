import { useEffect } from 'react'
import { LoadingRows } from '../../components/DataState.jsx'
import { useAuth } from '../../auth/authState.js'
import { sanitizeRedirectPath, useRouter } from '../../lib/routerState.js'

export function ProtectedAdminRoute({ children }) {
  const { isInitializing, isAuthenticated, isAdmin } = useAuth()
  const { pathname, search, navigate } = useRouter()

  useEffect(() => {
    if (isInitializing) return
    if (!isAuthenticated) {
      const next = sanitizeRedirectPath(`${pathname}${search}`, '/admin')
      navigate(`/login?next=${encodeURIComponent(next)}`, { replace: true })
      return
    }
    if (!isAdmin) {
      navigate('/unauthorized', { replace: true })
    }
  }, [isInitializing, isAuthenticated, isAdmin, pathname, search, navigate])

  if (isInitializing) {
    return (
      <div className="app-shell">
        <main className="main-content" style={{ marginTop: 32 }}>
          <LoadingRows label="Restoring administrator session" />
        </main>
      </div>
    )
  }

  if (!isAuthenticated || !isAdmin) {
    return null
  }

  return children
}
