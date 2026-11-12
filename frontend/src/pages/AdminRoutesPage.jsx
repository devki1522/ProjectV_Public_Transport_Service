import { useEffect, useMemo, useState } from 'react'
import { AlertCircle, CheckCircle2, Edit3, Plus, Power, RefreshCw, Search } from 'lucide-react'
import {
  createAdminRoute,
  deactivateAdminRoute,
  getAllRoutes,
  updateAdminRoute,
} from '../api/admin.js'
import { useAuth } from '../auth/authState.js'
import { EmptyState, LoadingRows } from '../components/DataState.jsx'
import { AdminLayout } from '../features/admin/AdminLayout.jsx'
import { useResource } from '../hooks/useResource.js'
import { directionLabel } from '../lib/transit.js'

const DIRECTIONS = [
  { value: 'direction_0', label: 'Direction 1 (direction_0)' },
  { value: 'direction_1', label: 'Direction 2 (direction_1)' },
]

function validateRouteFields({ number, name, direction }) {
  const trimmedNumber = number.trim()
  const trimmedName = name.trim()
  if (!trimmedNumber || [...trimmedNumber].length > 20 || trimmedNumber.includes('\0')) {
    return 'Route number is required and must be 20 characters or fewer.'
  }
  if (!trimmedName || [...trimmedName].length > 100 || trimmedName.includes('\0')) {
    return 'Route name is required and must be 100 characters or fewer.'
  }
  if (direction !== 'direction_0' && direction !== 'direction_1') {
    return 'Direction must be direction_0 or direction_1.'
  }
  return null
}

export function AdminRoutesPage() {
  const { csrfToken, handleApiError } = useAuth()
  const catalog = useResource('admin-routes', getAllRoutes)
  const [query, setQuery] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')

  const [createForm, setCreateForm] = useState({ number: '', name: '', direction: 'direction_0' })
  const [createPending, setCreatePending] = useState(false)
  const [createError, setCreateError] = useState(null)

  const [editingId, setEditingId] = useState(null)
  const [editForm, setEditForm] = useState({ number: '', name: '', direction: 'direction_0', is_active: true })
  const [editPending, setEditPending] = useState(false)
  const [editError, setEditError] = useState(null)

  const [confirmDeactivate, setConfirmDeactivate] = useState(null)
  const [deactivatePending, setDeactivatePending] = useState(false)
  const [actionNotice, setActionNotice] = useState(null)
  const [actionError, setActionError] = useState(null)
  const mutationPending = createPending || editPending || deactivatePending

  useEffect(() => {
    if (catalog.error?.status === 401 || catalog.error?.status === 403) {
      handleApiError(catalog.error)
    }
  }, [catalog.error, handleApiError])

  const routes = useMemo(() => catalog.data ?? [], [catalog.data])

  const filteredRoutes = useMemo(() => {
    const normalized = query.trim().toLowerCase()
    return routes.filter((route) => {
      if (statusFilter === 'active' && !route.is_active) return false
      if (statusFilter === 'inactive' && route.is_active) return false
      if (!normalized) return true
      return route.number.toLowerCase().includes(normalized) || route.name.toLowerCase().includes(normalized)
    })
  }, [routes, query, statusFilter])

  async function handleCreateSubmit(event) {
    event.preventDefault()
    if (mutationPending) return

    const validationError = validateRouteFields(createForm)
    if (validationError) {
      setCreateError(validationError)
      return
    }

    setCreatePending(true)
    setCreateError(null)
    setActionNotice(null)
    setActionError(null)
    try {
      const created = await createAdminRoute(
        {
          number: createForm.number.trim(),
          name: createForm.name.trim(),
          direction: createForm.direction,
        },
        csrfToken,
      )
      setCreateForm({ number: '', name: '', direction: 'direction_0' })
      setActionNotice(`Created route ${created.number} (${created.name}).`)
      catalog.retry()
    } catch (error) {
      if (error.status === 401 || error.status === 403) {
        handleApiError(error)
        return
      }
      setCreateError(error.message)
    } finally {
      setCreatePending(false)
    }
  }

  function startEdit(route) {
    if (mutationPending) return
    setEditingId(route.id)
    setEditForm({
      number: route.number,
      name: route.name,
      direction: route.direction,
      is_active: route.is_active,
    })
    setEditError(null)
    setConfirmDeactivate(null)
    setActionNotice(null)
    setActionError(null)
  }

  async function handleEditSubmit(event) {
    event.preventDefault()
    if (mutationPending || editingId === null) return

    const validationError = validateRouteFields(editForm)
    if (validationError) {
      setEditError(validationError)
      return
    }

    const current = routes.find((route) => route.id === editingId)
    if (current?.is_active && !editForm.is_active && !window.confirm('Deactivate this route? It will be hidden from public schedules. Related schedules and stops are preserved.')) return

    setEditPending(true)
    setEditError(null)
    setActionNotice(null)
    setActionError(null)
    try {
      const updated = await updateAdminRoute(
        editingId,
        {
          number: editForm.number.trim(),
          name: editForm.name.trim(),
          direction: editForm.direction,
          is_active: editForm.is_active,
        },
        csrfToken,
      )
      setEditingId(null)
      setActionNotice(`Updated route ${updated.number} (${updated.name}).`)
      catalog.retry()
    } catch (error) {
      if (error.status === 401 || error.status === 403) {
        handleApiError(error)
        return
      }
      setEditError(error.message)
    } finally {
      setEditPending(false)
    }
  }

  async function handleConfirmDeactivate() {
    if (!confirmDeactivate || mutationPending) return
    setDeactivatePending(true)
    setActionError(null)
    setActionNotice(null)
    try {
      await deactivateAdminRoute(confirmDeactivate.id, csrfToken)
      setActionNotice(`Deactivated route ${confirmDeactivate.number} (${confirmDeactivate.name}). Related schedules remain preserved.`)
      setConfirmDeactivate(null)
      if (editingId === confirmDeactivate.id) {
        setEditingId(null)
      }
      catalog.retry()
    } catch (error) {
      if (error.status === 401 || error.status === 403) {
        handleApiError(error)
        return
      }
      setActionError(error.message)
      setConfirmDeactivate(null)
    } finally {
      setDeactivatePending(false)
    }
  }

  const visibleListError = catalog.error && catalog.error.status !== 401 && catalog.error.status !== 403
    ? catalog.error
    : null

  return (
    <AdminLayout title="Transit routes">
      <div className="page-heading admin-page-heading">
        <div>
          <h1>Transit routes</h1>
          <p>Create, update, activate, or deactivate directional routes. Deactivating hides a route without deleting schedules.</p>
        </div>
        <button className="button button-secondary" type="button" onClick={catalog.retry} disabled={catalog.pending}>
          <RefreshCw size={16} aria-hidden="true" />Refresh
        </button>
      </div>

      {actionNotice && (
        <div className="success-notice" role="status">
          <CheckCircle2 size={18} aria-hidden="true" />
          <span>{actionNotice}</span>
        </div>
      )}

      {actionError && (
        <div className="error-notice" role="alert">
          <AlertCircle size={20} aria-hidden="true" />
          <div><strong>Route update failed</strong><p>{actionError}</p></div>
        </div>
      )}

      <div className="admin-routes-grid">
        <section className="admin-panel" aria-labelledby="create-route-heading">
          <div className="admin-panel-header">
            <h2 id="create-route-heading">Create route</h2>
            <p>New routes are active immediately upon creation.</p>
          </div>

          {createError && (
            <div className="error-notice auth-error" role="alert">
              <AlertCircle size={20} aria-hidden="true" />
              <div><strong>Could not create route</strong><p>{createError}</p></div>
            </div>
          )}

          <form className="admin-form" onSubmit={handleCreateSubmit} noValidate>
            <div className="field">
              <label htmlFor="create-route-number">Route number</label>
              <input
                id="create-route-number"
                type="text"
                maxLength={20}
                value={createForm.number}
                onChange={(event) => setCreateForm((f) => ({ ...f, number: event.target.value }))}
                placeholder="e.g. 7"
                disabled={mutationPending}
                required
              />
            </div>

            <div className="field">
              <label htmlFor="create-route-name">Route name</label>
              <input
                id="create-route-name"
                type="text"
                maxLength={100}
                value={createForm.name}
                onChange={(event) => setCreateForm((f) => ({ ...f, name: event.target.value }))}
                placeholder="e.g. King"
                disabled={mutationPending}
                required
              />
            </div>

            <div className="field">
              <label htmlFor="create-route-direction">Direction</label>
              <select
                id="create-route-direction"
                value={createForm.direction}
                onChange={(event) => setCreateForm((f) => ({ ...f, direction: event.target.value }))}
                disabled={mutationPending}
              >
                {DIRECTIONS.map((dir) => (
                  <option key={dir.value} value={dir.value}>{dir.label}</option>
                ))}
              </select>
            </div>

            <button className="button button-primary" type="submit" disabled={mutationPending}>
              <Plus size={16} aria-hidden="true" />
              {createPending ? 'Creating route…' : 'Create route'}
            </button>
          </form>
        </section>

        <section className="admin-panel" aria-labelledby="route-catalog-heading">
          <div className="admin-catalog-toolbar">
            <div>
              <h2 id="route-catalog-heading">Route catalog</h2>
              <p>{routes.length} total routes ({routes.filter((r) => r.is_active).length} active)</p>
            </div>

            <div className="admin-filter-controls">
              <div className="search-field admin-search-field">
                <Search size={16} aria-hidden="true" />
                <input
                  type="search"
                  aria-label="Filter routes by number or name"
                  placeholder="Filter by number or name"
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                />
              </div>
              <select
                aria-label="Filter by route status"
                value={statusFilter}
                onChange={(event) => setStatusFilter(event.target.value)}
              >
                <option value="all">All statuses</option>
                <option value="active">Active only</option>
                <option value="inactive">Inactive only</option>
              </select>
            </div>
          </div>

          {confirmDeactivate && (
            <div className="confirm-panel" role="alertdialog" aria-labelledby="confirm-deactivate-title">
              <div>
                <h3 id="confirm-deactivate-title">
                  Deactivate Route {confirmDeactivate.number} ({confirmDeactivate.name})?
                </h3>
                <p>
                  Deactivating this route hides it from public resident route lists while preserving all related schedules, stops, and historical trip data. You can reactivate it later by editing the route.
                </p>
              </div>
              <div className="confirm-actions">
                <button
                  className="button button-danger"
                  type="button"
                  onClick={handleConfirmDeactivate}
                  disabled={mutationPending}
                >
                  {deactivatePending ? 'Deactivating…' : 'Confirm deactivate'}
                </button>
                <button
                  className="button button-secondary"
                  type="button"
                  onClick={() => setConfirmDeactivate(null)}
                  disabled={mutationPending}
                >
                  Cancel
                </button>
              </div>
            </div>
          )}

          {editingId !== null && (
            <form className="admin-edit-panel" onSubmit={handleEditSubmit} noValidate aria-label="Edit selected route">
              <div className="admin-edit-header">
                <h3>Edit route #{editingId}</h3>
                <button className="text-button" type="button" onClick={() => setEditingId(null)} disabled={mutationPending}>
                  Cancel editing
                </button>
              </div>

              {editError && (
                <div className="error-notice auth-error" role="alert">
                  <AlertCircle size={20} aria-hidden="true" />
                  <div><strong>Could not save route</strong><p>{editError}</p></div>
                </div>
              )}

              <div className="admin-edit-fields">
                <div className="field">
                  <label htmlFor="edit-route-number">Route number</label>
                  <input
                    id="edit-route-number"
                    type="text"
                    maxLength={20}
                    value={editForm.number}
                    onChange={(event) => setEditForm((f) => ({ ...f, number: event.target.value }))}
                    disabled={mutationPending}
                    required
                  />
                </div>
                <div className="field">
                  <label htmlFor="edit-route-name">Route name</label>
                  <input
                    id="edit-route-name"
                    type="text"
                    maxLength={100}
                    value={editForm.name}
                    onChange={(event) => setEditForm((f) => ({ ...f, name: event.target.value }))}
                    disabled={mutationPending}
                    required
                  />
                </div>
                <div className="field">
                  <label htmlFor="edit-route-direction">Direction</label>
                  <select
                    id="edit-route-direction"
                    value={editForm.direction}
                    onChange={(event) => setEditForm((f) => ({ ...f, direction: event.target.value }))}
                    disabled={mutationPending}
                  >
                    {DIRECTIONS.map((dir) => (
                      <option key={dir.value} value={dir.value}>{dir.label}</option>
                    ))}
                  </select>
                </div>
                <div className="field">
                  <label htmlFor="edit-route-status">Status</label>
                  <select
                    id="edit-route-status"
                    value={editForm.is_active ? 'active' : 'inactive'}
                    onChange={(event) => setEditForm((f) => ({ ...f, is_active: event.target.value === 'active' }))}
                    disabled={mutationPending}
                  >
                    <option value="active">Active</option>
                    <option value="inactive">Inactive</option>
                  </select>
                </div>
              </div>

              <div className="admin-edit-actions">
                <button className="button button-primary" type="submit" disabled={mutationPending}>
                  {editPending ? 'Saving changes…' : 'Save changes'}
                </button>
              </div>
            </form>
          )}

          {visibleListError ? (
            <div className="error-notice" role="alert">
              <AlertCircle size={20} aria-hidden="true" />
              <div><strong>Routes couldn’t be loaded</strong><p>{visibleListError.message}</p></div>
              <button className="button button-secondary" type="button" onClick={catalog.retry}>
                <RefreshCw size={16} aria-hidden="true" />Try again
              </button>
            </div>
          ) : catalog.pending && routes.length === 0 ? (
            <LoadingRows label="Loading transit routes" />
          ) : filteredRoutes.length === 0 ? (
            <EmptyState
              title="No matching routes"
              description="Adjust your search filter or create a new route."
            />
          ) : (
            <div className="admin-table-wrap">
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>Route</th>
                    <th>Direction</th>
                    <th>Status</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredRoutes.map((route) => (
                    <tr key={route.id}>
                      <td>
                        <div className="admin-route-cell">
                          <span className="route-number">{route.number}</span>
                          <div>
                            <strong>{route.name}</strong>
                            <small>ID {route.id}</small>
                          </div>
                        </div>
                      </td>
                      <td>{directionLabel(route.direction)} <small className="muted-code">({route.direction})</small></td>
                      <td>
                        <span className={`status-pill ${route.is_active ? 'is-active' : 'is-inactive'}`}>
                          {route.is_active ? 'Active' : 'Inactive'}
                        </span>
                      </td>
                      <td>
                        <div className="table-actions">
                          <button
                            className="button button-secondary button-compact"
                            type="button"
                            onClick={() => startEdit(route)}
                            disabled={mutationPending}
                          >
                            <Edit3 size={14} aria-hidden="true" />Edit
                          </button>
                          {route.is_active && (
                            <button
                              className="button button-secondary button-compact"
                              type="button"
                              disabled={mutationPending}
                              onClick={() => {
                                if (mutationPending) return
                                setConfirmDeactivate(route)
                                setActionError(null)
                                setActionNotice(null)
                              }}
                            >
                              <Power size={14} aria-hidden="true" />Deactivate
                            </button>
                          )}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
      </div>
    </AdminLayout>
  )
}
