import { useEffect, useState } from 'react'
import { AlertCircle, ArrowLeft, BusFront, Lock } from 'lucide-react'
import { hasAdminRole, useAuth } from '../auth/authState.js'
import { Link } from '../lib/router.jsx'
import { sanitizeRedirectPath, useRouter } from '../lib/routerState.js'

export function LoginPage() {
  const { isInitializing, isAuthenticated, isAdmin, login } = useAuth()
  const { searchParams, navigate } = useRouter()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState(null)
  const [submitting, setSubmitting] = useState(false)

  const targetPath = sanitizeRedirectPath(searchParams.get('next'), '/admin')

  useEffect(() => {
    document.title = 'Administrator sign in · Transit Administration'
  }, [])

  useEffect(() => {
    if (isInitializing || !isAuthenticated) return
    if (isAdmin) {
      navigate(targetPath, { replace: true })
    } else {
      navigate('/unauthorized', { replace: true })
    }
  }, [isInitializing, isAuthenticated, isAdmin, targetPath, navigate])

  async function handleSubmit(event) {
    event.preventDefault()
    if (submitting) return
    const trimmedUser = username.trim()
    if (!trimmedUser || !password) {
      setError('Enter both your username and password.')
      return
    }

    setSubmitting(true)
    setError(null)
    try {
      const session = await login({ username: trimmedUser, password })
      setPassword('')
      if (hasAdminRole(session.user)) {
        navigate(targetPath, { replace: true })
      } else {
        navigate('/unauthorized', { replace: true })
      }
    } catch (err) {
      setPassword('')
      setError(err.message || 'Unable to sign in. Please try again.')
    } finally {
      setSubmitting(false)
    }
  }

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
        <section className="auth-card" aria-labelledby="login-heading">
          <div className="auth-card-header">
            <span className="auth-badge"><Lock size={18} aria-hidden="true" /></span>
            <div>
              <p className="auth-eyebrow">Transit Administration</p>
              <h1 id="login-heading">Administrator sign in</h1>
            </div>
          </div>

          <p className="auth-description">
            Sign in with your transit administrator account to view route inventory and manage published routes. Sessions expire after 15 minutes.
          </p>

          {error && (
            <div className="error-notice auth-error" role="alert">
              <AlertCircle size={20} aria-hidden="true" />
              <div><strong>Sign in failed</strong><p>{error}</p></div>
            </div>
          )}

          <form className="auth-form" onSubmit={handleSubmit} noValidate>
            <div className="field">
              <label htmlFor="login-username">Username</label>
              <input
                id="login-username"
                name="username"
                type="text"
                autoComplete="username"
                value={username}
                onChange={(event) => setUsername(event.target.value)}
                disabled={submitting}
                required
              />
            </div>

            <div className="field">
              <label htmlFor="login-password">Password</label>
              <input
                id="login-password"
                name="password"
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                disabled={submitting}
                required
              />
            </div>

            <div className="auth-actions">
              <button className="button button-primary" type="submit" disabled={submitting}>
                {submitting ? 'Signing in…' : 'Sign in'}
              </button>
              <Link className="button button-secondary" to="/">Return to public schedules</Link>
            </div>
          </form>
        </section>
      </main>
    </div>
  )
}
