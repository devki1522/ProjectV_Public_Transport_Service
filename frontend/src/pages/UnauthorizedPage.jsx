import { useEffect } from 'react'
import { ArrowLeft, BusFront, LogOut, ShieldAlert } from 'lucide-react'
import { useAuth } from '../auth/authState.js'
import { Link } from '../lib/router.jsx'

export function UnauthorizedPage() {
  const { isAuthenticated, user, logout } = useAuth()

  useEffect(() => {
    document.title = 'Access denied · Transit Administration'
  }, [])

  return (
    <div className="app-shell">
      <header className="site-header">
        <Link className="brand" to="/">
          <span className="brand-mark"><BusFront size={24} aria-hidden="true" /></span>
          <span>Transit Administration{' '}<span className="brand-region">Waterloo Region</span></span>
        </Link>
        <Link className="admin-back-link" to="/"><ArrowLeft size={16} aria-hidden="true" />Public schedules</Link>
      </header>

      <main id="main-content" className="auth-main" tabIndex={-1}>
        <section className="auth-card" aria-labelledby="unauthorized-heading">
          <div className="auth-card-header">
            <span className="auth-badge auth-badge-error"><ShieldAlert size={18} aria-hidden="true" /></span>
            <div>
              <p className="auth-eyebrow">Transit Administration</p>
              <h1 id="unauthorized-heading">Access denied</h1>
            </div>
          </div>

          <p className="auth-description">
            {isAuthenticated && user
              ? `Signed in as ${user.username}, which does not have the TransitAdministrator role required for administrator access.`
              : 'You do not have permission to view the Transit Administration portal.'}
          </p>

          <div className="auth-actions">
            <Link className="button button-primary" to="/">Return to public schedules</Link>
            {isAuthenticated ? (
              <button className="button button-secondary" type="button" onClick={logout}>
                <LogOut size={16} aria-hidden="true" />Sign out
              </button>
            ) : (
              <Link className="button button-secondary" to="/login">Administrator sign in</Link>
            )}
          </div>
        </section>
      </main>
    </div>
  )
}
