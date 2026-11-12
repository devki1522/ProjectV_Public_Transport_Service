import { useEffect } from 'react'
import { BusFront, ExternalLink, LayoutDashboard, LogOut, Route } from 'lucide-react'
import { useAuth } from '../../auth/authState.js'
import { Link } from '../../lib/router.jsx'
import { useRouter } from '../../lib/routerState.js'

export function AdminLayout({ title, children }) {
  const { user, logout } = useAuth()
  const { pathname } = useRouter()

  useEffect(() => {
    document.title = `${title} · Transit Administration`
  }, [title])

  return (
    <div className="app-shell">
      <a className="skip-link" href="#admin-main-content">Skip to administration content</a>
      <header className="site-header admin-header">
        <Link className="brand" to="/admin">
          <span className="brand-mark"><BusFront size={24} aria-hidden="true" /></span>
          <span>Transit Administration{' '}<span className="brand-region">Waterloo Region</span></span>
        </Link>

        <nav className="admin-nav" aria-label="Administrator navigation">
          <Link
            to="/admin"
            className={`admin-nav-link ${pathname === '/admin' ? 'is-active' : ''}`}
            aria-current={pathname === '/admin' ? 'page' : undefined}
          >
            <LayoutDashboard size={16} aria-hidden="true" />
            <span>Dashboard</span>
          </Link>
          <Link
            to="/admin/transit"
            className={`admin-nav-link ${pathname === '/admin/transit' ? 'is-active' : ''}`}
            aria-current={pathname === '/admin/transit' ? 'page' : undefined}
          >
            <Route size={16} aria-hidden="true" />
            <span>Transit routes</span>
          </Link>
          <Link to="/" className="admin-nav-link">
            <ExternalLink size={16} aria-hidden="true" />
            <span>Public schedules</span>
          </Link>
        </nav>

        <div className="admin-user-bar">
          <span className="admin-username" title="Signed-in administrator">{user?.username}</span>
          <button className="button button-secondary admin-logout-btn" type="button" onClick={logout}>
            <LogOut size={16} aria-hidden="true" />
            <span>Sign out</span>
          </button>
        </div>
      </header>

      <main id="admin-main-content" className="admin-main" tabIndex={-1}>
        {children}
      </main>

      <footer className="site-footer">
        <span><BusFront size={16} aria-hidden="true" /> Transit Administration · Waterloo Region</span>
        <span>Sessions expire 15 minutes after sign-in.</span>
      </footer>
    </div>
  )
}
