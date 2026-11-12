import { useEffect } from 'react'
import { AlertCircle, RefreshCw, Route } from 'lucide-react'
import { getAdminDashboard } from '../api/admin.js'
import { useAuth } from '../auth/authState.js'
import { EmptyState, LoadingRows } from '../components/DataState.jsx'
import { AdminLayout } from '../features/admin/AdminLayout.jsx'
import { useResource } from '../hooks/useResource.js'
import { Link } from '../lib/router.jsx'

export function AdminDashboardPage() {
  const { handleApiError } = useAuth()
  const dashboard = useResource('admin-dashboard', getAdminDashboard)

  useEffect(() => {
    if (dashboard.error?.status === 401 || dashboard.error?.status === 403) {
      handleApiError(dashboard.error)
    }
  }, [dashboard.error, handleApiError])

  const visibleError = dashboard.error && dashboard.error.status !== 401 && dashboard.error.status !== 403
    ? dashboard.error
    : null

  return (
    <AdminLayout title="Dashboard">
      <div className="page-heading admin-page-heading">
        <div>
          <h1>Transit dashboard</h1>
          <p>Current route catalog inventory from PostgreSQL.</p>
        </div>
        <Link className="button button-primary" to="/admin/transit">Manage transit routes</Link>
      </div>

      <section className="admin-panel" aria-label="Route inventory summary">
        {visibleError ? (
          <div className="error-notice" role="alert">
            <AlertCircle size={20} aria-hidden="true" />
            <div>
              <strong>Dashboard summary couldn’t be loaded</strong>
              <p>{visibleError.message}</p>
            </div>
            <button className="button button-secondary" type="button" onClick={dashboard.retry}>
              <RefreshCw size={16} aria-hidden="true" />Try again
            </button>
          </div>
        ) : dashboard.pending && !dashboard.data ? (
          <LoadingRows label="Loading dashboard summary" />
        ) : dashboard.data && dashboard.data.total_routes === 0 ? (
          <EmptyState
            title="No routes in catalog"
            description="No transit routes have been created or seeded yet."
            onRetry={dashboard.retry}
          />
        ) : dashboard.data ? (
          <div className="dashboard-grid">
            <article className="metric-card">
              <span className="metric-label">Total routes</span>
              <strong className="metric-value">{dashboard.data.total_routes}</strong>
              <span className="metric-hint">All directional route entries</span>
            </article>
            <article className="metric-card">
              <span className="metric-label">Active routes</span>
              <strong className="metric-value metric-value-accent">{dashboard.data.active_routes}</strong>
              <span className="metric-hint">Published on resident schedules</span>
            </article>
            <article className="metric-card">
              <span className="metric-label">Inactive routes</span>
              <strong className="metric-value">{dashboard.data.inactive_routes}</strong>
              <span className="metric-hint">Hidden with schedules preserved</span>
            </article>
          </div>
        ) : null}

        <div className="admin-panel-footer">
          <Route size={16} aria-hidden="true" />
          <span>Counts reflect live database records. Add, edit, or deactivate routes under Transit routes.</span>
        </div>
      </section>
    </AdminLayout>
  )
}
