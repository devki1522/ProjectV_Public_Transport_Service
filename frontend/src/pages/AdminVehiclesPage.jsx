import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Accessibility,
  AlertCircle,
  CheckCircle2,
  MapPin,
  Pause,
  Play,
  RefreshCw,
  RotateCcw,
  Send,
  StepForward,
} from 'lucide-react'
import {
  getAllRoutes,
  resetVehicleSimulator,
  startVehicleSimulator,
  stepVehicleSimulator,
  stopVehicleSimulator,
  submitVehicleLocation,
} from '../api/admin.js'
import { getActiveVehicles, getSimulatorStatus } from '../api/transit.js'
import { useAuth } from '../auth/authState.js'
import { EmptyState, LoadingRows } from '../components/DataState.jsx'
import { AdminLayout } from '../features/admin/AdminLayout.jsx'
import {
  VEHICLE_STATUS_OPTIONS,
  vehicleStatusLabel,
  vehicleStatusPillClass,
} from '../features/live/vehicleFormat.js'
import { useResource } from '../hooks/useResource.js'
import { directionLabel, formatTime } from '../lib/transit.js'

const WATERLOO_PRESETS = [
  { label: 'Central Station', latitude: '43.4516', longitude: '-80.4981' },
  { label: 'University of Waterloo', latitude: '43.4723', longitude: '-80.5449' },
  { label: 'Fairway Station', latitude: '43.4209', longitude: '-80.4388' },
  { label: 'Conestoga College', latitude: '43.3895', longitude: '-80.4050' },
]

const SPEED_OPTIONS = [1, 2, 5]

const INITIAL_FORM = {
  routeId: '',
  vehicleNumber: '',
  tripId: '',
  latitude: '',
  longitude: '',
  heading: '',
  speedKmh: '',
  status: 'in_service',
  isAccessible: true,
}

function validateVehicleForm(form, effectiveRouteId) {
  const routeId = Number(effectiveRouteId)
  if (!Number.isInteger(routeId) || routeId <= 0) {
    return { error: 'Select a valid route before submitting a location update.' }
  }

  const vehicleNumber = form.vehicleNumber.trim()
  if (!vehicleNumber || [...vehicleNumber].length > 20 || vehicleNumber.includes('\0')) {
    return { error: 'Vehicle number is required and must be 20 characters or fewer.' }
  }

  let tripId = null
  if (form.tripId.trim() !== '') {
    const parsedTrip = Number(form.tripId.trim())
    if (!Number.isInteger(parsedTrip) || parsedTrip <= 0) {
      return { error: 'Trip ID must be a positive whole number when provided.' }
    }
    tripId = parsedTrip
  }

  if (form.latitude.trim() === '' || form.longitude.trim() === '') {
    return { error: 'Latitude and longitude are required. Choose a Waterloo preset or enter coordinates.' }
  }
  const latitude = Number(form.latitude)
  if (!Number.isFinite(latitude) || latitude < -90 || latitude > 90) {
    return { error: 'Latitude must be a number between -90 and 90.' }
  }
  const longitude = Number(form.longitude)
  if (!Number.isFinite(longitude) || longitude < -180 || longitude > 180) {
    return { error: 'Longitude must be a number between -180 and 180.' }
  }

  let heading = null
  if (form.heading.trim() !== '') {
    const parsedHeading = Number(form.heading)
    if (!Number.isFinite(parsedHeading) || parsedHeading < 0 || parsedHeading > 360) {
      return { error: 'Heading must be between 0 and 360 degrees.' }
    }
    heading = parsedHeading
  }

  let speedKmh = null
  if (form.speedKmh.trim() !== '') {
    const parsedSpeed = Number(form.speedKmh)
    if (!Number.isFinite(parsedSpeed) || parsedSpeed < 0 || parsedSpeed > 200) {
      return { error: 'Speed must be between 0 and 200 km/h.' }
    }
    speedKmh = parsedSpeed
  }

  const validStatuses = new Set(VEHICLE_STATUS_OPTIONS.map((option) => option.value))
  if (!validStatuses.has(form.status)) {
    return { error: 'Select a valid operating status.' }
  }

  const payload = {
    vehicle_number: vehicleNumber,
    route_id: routeId,
    latitude,
    longitude,
    status: form.status,
    is_accessible: Boolean(form.isAccessible),
  }
  if (tripId !== null) payload.trip_id = tripId
  if (heading !== null) payload.heading = heading
  if (speedKmh !== null) payload.speed_kmh = speedKmh

  return { payload }
}

export function AdminVehiclesPage() {
  const { csrfToken, handleApiError } = useAuth()
  const routesRes = useResource('admin-fleet-routes', getAllRoutes)
  const loadActiveVehicles = useCallback((signal) => getActiveVehicles({}, signal), [])
  const vehiclesRes = useResource('admin-active-vehicles', loadActiveVehicles)
  const simulatorRes = useResource('admin-simulator-status', getSimulatorStatus)

  const [form, setForm] = useState(INITIAL_FORM)
  const [submitPending, setSubmitPending] = useState(false)
  const [submitError, setSubmitError] = useState(null)

  const [speedSelection, setSpeedSelection] = useState(null)
  const [simPending, setSimPending] = useState(false)
  const [simOverride, setSimOverride] = useState(null)
  const [actionNotice, setActionNotice] = useState(null)
  const [actionError, setActionError] = useState(null)

  useEffect(() => {
    if (routesRes.error?.status === 401 || routesRes.error?.status === 403) {
      handleApiError(routesRes.error)
    }
  }, [routesRes.error, handleApiError])

  const routes = useMemo(
    () =>
      [...(routesRes.data ?? [])].sort((a, b) => {
        if (a.is_active !== b.is_active) return a.is_active ? -1 : 1
        const numberOrder = a.number.localeCompare(b.number, 'en', { numeric: true })
        return numberOrder !== 0 ? numberOrder : a.direction.localeCompare(b.direction)
      }),
    [routesRes.data],
  )

  const effectiveRouteId = form.routeId || (routes[0] ? String(routes[0].id) : '')
  const vehicles = useMemo(() => vehiclesRes.data ?? [], [vehiclesRes.data])
  const simulatorStatus = simOverride ?? simulatorRes.data

  const effectiveSpeed = useMemo(() => {
    if (speedSelection !== null) return speedSelection
    const current = simulatorStatus?.speed_multiplier
    return SPEED_OPTIONS.includes(current) ? String(current) : '1'
  }, [speedSelection, simulatorStatus])

  const mutationPending = submitPending || simPending

  function refreshAll() {
    setSimOverride(null)
    routesRes.retry()
    vehiclesRes.retry()
    simulatorRes.retry()
  }

  function applyPreset(preset) {
    setForm((current) => ({
      ...current,
      latitude: preset.latitude,
      longitude: preset.longitude,
    }))
    setSubmitError(null)
  }

  async function handleLocationSubmit(event) {
    event.preventDefault()
    if (mutationPending) return

    const { payload, error: validationError } = validateVehicleForm(form, effectiveRouteId)
    if (validationError) {
      setSubmitError(validationError)
      return
    }

    setSubmitPending(true)
    setSubmitError(null)
    setActionNotice(null)
    setActionError(null)

    try {
      const updated = await submitVehicleLocation(payload, csrfToken)
      setActionNotice(
        `Submitted location update for bus #${updated.vehicle_number} on Route ${updated.route_number}.`,
      )
      vehiclesRes.retry()
    } catch (error) {
      if (error.status === 401 || error.status === 403) {
        handleApiError(error)
        return
      }
      setSubmitError(error.message)
    } finally {
      setSubmitPending(false)
    }
  }

  async function handleSimulatorAction(action) {
    if (mutationPending) return
    setSimPending(true)
    setActionNotice(null)
    setActionError(null)

    try {
      let nextStatus
      let notice
      if (action === 'start') {
        const multiplier = Number(effectiveSpeed)
        nextStatus = await startVehicleSimulator({ speed_multiplier: multiplier }, csrfToken)
        notice = `Live bus simulator started at ${nextStatus.speed_multiplier}x speed.`
      } else if (action === 'stop') {
        nextStatus = await stopVehicleSimulator(csrfToken)
        notice = 'Live bus simulator paused.'
      } else if (action === 'step') {
        nextStatus = await stepVehicleSimulator(csrfToken)
        notice = `Advanced simulated vehicles by one tick (${nextStatus.total_updates_sent} total updates).`
      } else {
        nextStatus = await resetVehicleSimulator(csrfToken)
        notice = 'Reset simulated bus positions.'
      }
      setSimOverride(nextStatus)
      setActionNotice(notice)
      vehiclesRes.retry()
    } catch (error) {
      if (error.status === 401 || error.status === 403) {
        handleApiError(error)
        return
      }
      setActionError(error.message)
    } finally {
      setSimPending(false)
    }
  }

  const visibleVehiclesError =
    vehiclesRes.error && vehiclesRes.error.status !== 401 && vehiclesRes.error.status !== 403
      ? vehiclesRes.error
      : null

  return (
    <AdminLayout title="Live fleet">
      <div className="page-heading admin-page-heading">
        <div>
          <h1>Live fleet &amp; vehicle tracking</h1>
          <p>Submit live GPS vehicle location updates and control the built-in Waterloo Region bus simulator.</p>
        </div>
        <button
          className="button button-secondary"
          type="button"
          onClick={refreshAll}
          disabled={vehiclesRes.pending || simulatorRes.pending}
        >
          <RefreshCw size={16} aria-hidden="true" />
          <span>Refresh</span>
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
          <div>
            <strong>Fleet action failed</strong>
            <p>{actionError}</p>
          </div>
        </div>
      )}

      <section className="admin-panel simulator-card" aria-labelledby="simulator-controls-heading">
        <div className="simulator-card-header">
          <div>
            <div className="simulator-title-row">
              <h2 id="simulator-controls-heading">Live bus simulator</h2>
              <span
                className={`status-pill ${simulatorStatus?.running ? 'is-active' : 'is-inactive'}`}
              >
                {simulatorStatus?.running ? 'Running' : 'Paused'}
              </span>
            </div>
            <p>
              Simulates buses moving along active Waterloo Region route paths for live map and arrival demonstrations.
            </p>
          </div>

          <div className="simulator-controls">
            <div className="field simulator-speed-field">
              <label htmlFor="simulator-speed">Speed</label>
              <select
                id="simulator-speed"
                value={effectiveSpeed}
                onChange={(event) => setSpeedSelection(event.target.value)}
                disabled={mutationPending}
              >
                {SPEED_OPTIONS.map((speed) => (
                  <option key={speed} value={String(speed)}>
                    {speed}x
                  </option>
                ))}
              </select>
            </div>

            {simulatorStatus?.running ? (
              <button
                type="button"
                className="button button-secondary"
                onClick={() => handleSimulatorAction('stop')}
                disabled={mutationPending}
              >
                <Pause size={15} aria-hidden="true" />
                <span>Pause simulator</span>
              </button>
            ) : (
              <button
                type="button"
                className="button button-primary"
                onClick={() => handleSimulatorAction('start')}
                disabled={mutationPending}
              >
                <Play size={15} aria-hidden="true" />
                <span>Start simulator</span>
              </button>
            )}

            <button
              type="button"
              className="button button-secondary"
              onClick={() => handleSimulatorAction('step')}
              disabled={mutationPending}
            >
              <StepForward size={15} aria-hidden="true" />
              <span>Step once</span>
            </button>

            <button
              type="button"
              className="button button-secondary"
              onClick={() => handleSimulatorAction('reset')}
              disabled={mutationPending}
            >
              <RotateCcw size={15} aria-hidden="true" />
              <span>Reset simulated buses</span>
            </button>
          </div>
        </div>

        <div className="simulator-metrics">
          <div className="metric-card">
            <span className="metric-label">Simulated buses</span>
            <strong className="metric-value metric-value-accent">
              {simulatorStatus?.vehicle_count ?? 0}
            </strong>
            <span className="metric-hint">Active simulator fleet</span>
          </div>
          <div className="metric-card">
            <span className="metric-label">Simulated routes</span>
            <strong className="metric-value">{simulatorStatus?.route_count ?? 0}</strong>
            <span className="metric-hint">Routes with geometry</span>
          </div>
          <div className="metric-card">
            <span className="metric-label">Total updates sent</span>
            <strong className="metric-value">{simulatorStatus?.total_updates_sent ?? 0}</strong>
            <span className="metric-hint">
              {simulatorStatus?.last_tick_at
                ? `Last tick ${formatTime(simulatorStatus.last_tick_at)}`
                : 'No ticks sent yet'}
            </span>
          </div>
        </div>
      </section>

      <div className="admin-fleet-grid">
        <section className="admin-panel" aria-labelledby="submit-location-heading">
          <div className="admin-panel-header">
            <h2 id="submit-location-heading">Submit vehicle location update</h2>
            <p>Report a bus position directly to the live tracking service.</p>
          </div>

          {submitError && (
            <div className="error-notice auth-error" role="alert">
              <AlertCircle size={20} aria-hidden="true" />
              <div>
                <strong>Could not submit vehicle location</strong>
                <p>{submitError}</p>
              </div>
            </div>
          )}

          <form className="admin-form" onSubmit={handleLocationSubmit} noValidate>
            <div className="field">
              <label htmlFor="vehicle-route-id">Route</label>
              <select
                id="vehicle-route-id"
                value={effectiveRouteId}
                onChange={(event) => setForm((f) => ({ ...f, routeId: event.target.value }))}
                disabled={mutationPending || routes.length === 0}
                required
              >
                {routes.length === 0 ? (
                  <option value="">Loading routes…</option>
                ) : (
                  routes.map((route) => (
                    <option key={route.id} value={String(route.id)}>
                      Route {route.number} · {route.name} ({directionLabel(route.direction)})
                      {!route.is_active ? ' [Inactive]' : ''}
                    </option>
                  ))
                )}
              </select>
            </div>

            <div className="admin-vehicle-form-row">
              <div className="field">
                <label htmlFor="vehicle-number">Vehicle number</label>
                <input
                  id="vehicle-number"
                  type="text"
                  maxLength={20}
                  placeholder="e.g. BUS-701"
                  value={form.vehicleNumber}
                  onChange={(event) => setForm((f) => ({ ...f, vehicleNumber: event.target.value }))}
                  disabled={mutationPending}
                  required
                />
              </div>

              <div className="field">
                <label htmlFor="vehicle-trip-id">Trip ID (optional)</label>
                <input
                  id="vehicle-trip-id"
                  type="number"
                  min="1"
                  step="1"
                  placeholder="e.g. 100"
                  value={form.tripId}
                  onChange={(event) => setForm((f) => ({ ...f, tripId: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>
            </div>

            <div className="field">
              <span className="preset-label">Waterloo Region location presets</span>
              <div className="preset-buttons" role="group" aria-label="Waterloo Region location presets">
                {WATERLOO_PRESETS.map((preset) => (
                  <button
                    key={preset.label}
                    type="button"
                    className="button button-secondary button-compact preset-button"
                    onClick={() => applyPreset(preset)}
                    disabled={mutationPending}
                  >
                    <MapPin size={13} aria-hidden="true" />
                    <span>{preset.label}</span>
                  </button>
                ))}
              </div>
            </div>

            <div className="admin-vehicle-form-row">
              <div className="field">
                <label htmlFor="vehicle-latitude">Latitude</label>
                <input
                  id="vehicle-latitude"
                  type="number"
                  step="any"
                  min="-90"
                  max="90"
                  placeholder="43.4516"
                  value={form.latitude}
                  onChange={(event) => setForm((f) => ({ ...f, latitude: event.target.value }))}
                  disabled={mutationPending}
                  required
                />
              </div>

              <div className="field">
                <label htmlFor="vehicle-longitude">Longitude</label>
                <input
                  id="vehicle-longitude"
                  type="number"
                  step="any"
                  min="-180"
                  max="180"
                  placeholder="-80.4981"
                  value={form.longitude}
                  onChange={(event) => setForm((f) => ({ ...f, longitude: event.target.value }))}
                  disabled={mutationPending}
                  required
                />
              </div>
            </div>

            <div className="admin-vehicle-form-row">
              <div className="field">
                <label htmlFor="vehicle-heading">Heading (°)</label>
                <input
                  id="vehicle-heading"
                  type="number"
                  step="any"
                  min="0"
                  max="360"
                  placeholder="0–360"
                  value={form.heading}
                  onChange={(event) => setForm((f) => ({ ...f, heading: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>

              <div className="field">
                <label htmlFor="vehicle-speed">Speed (km/h)</label>
                <input
                  id="vehicle-speed"
                  type="number"
                  step="any"
                  min="0"
                  max="200"
                  placeholder="0–200"
                  value={form.speedKmh}
                  onChange={(event) => setForm((f) => ({ ...f, speedKmh: event.target.value }))}
                  disabled={mutationPending}
                />
              </div>
            </div>

            <div className="field">
              <label htmlFor="vehicle-status">Status</label>
              <select
                id="vehicle-status"
                value={form.status}
                onChange={(event) => setForm((f) => ({ ...f, status: event.target.value }))}
                disabled={mutationPending}
              >
                {VEHICLE_STATUS_OPTIONS.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </div>

            <label className="checkbox-field" htmlFor="vehicle-accessible">
              <input
                id="vehicle-accessible"
                type="checkbox"
                checked={form.isAccessible}
                onChange={(event) => setForm((f) => ({ ...f, isAccessible: event.target.checked }))}
                disabled={mutationPending}
              />
              <span>Wheelchair accessible vehicle</span>
            </label>

            <button className="button button-primary" type="submit" disabled={mutationPending}>
              <Send size={15} aria-hidden="true" />
              <span>{submitPending ? 'Submitting update…' : 'Submit location update'}</span>
            </button>
          </form>
        </section>

        <section className="admin-panel" aria-labelledby="active-fleet-heading">
          <div className="admin-catalog-toolbar">
            <div>
              <h2 id="active-fleet-heading">Active vehicles</h2>
              <p>
                {vehicles.length} active {vehicles.length === 1 ? 'vehicle' : 'vehicles'} reported in the last 15 minutes
              </p>
            </div>
          </div>

          {visibleVehiclesError ? (
            <div className="error-notice" role="alert">
              <AlertCircle size={20} aria-hidden="true" />
              <div>
                <strong>Active vehicles couldn’t be loaded</strong>
                <p>{visibleVehiclesError.message}</p>
              </div>
              <button className="button button-secondary" type="button" onClick={vehiclesRes.retry}>
                <RefreshCw size={16} aria-hidden="true" />
                <span>Try again</span>
              </button>
            </div>
          ) : vehiclesRes.pending && vehicles.length === 0 ? (
            <LoadingRows label="Loading active vehicles" />
          ) : vehicles.length === 0 ? (
            <EmptyState
              title="No active vehicles reported"
              description="Submit a vehicle location update or start the live bus simulator to populate the active fleet."
            />
          ) : (
            <div className="admin-table-wrap">
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>Vehicle</th>
                    <th>Route</th>
                    <th>Position</th>
                    <th>Telemetry</th>
                    <th>Status</th>
                    <th>Reported</th>
                  </tr>
                </thead>
                <tbody>
                  {vehicles.map((vehicle) => (
                    <tr key={vehicle.vehicle_id}>
                      <td>
                        <div className="admin-vehicle-cell">
                          <strong>Bus #{vehicle.vehicle_number}</strong>
                          <small>
                            ID {vehicle.vehicle_id}
                            {vehicle.is_accessible ? (
                              <span className="inline-accessible">
                                {' '}· <Accessibility size={11} aria-hidden="true" /> Accessible
                              </span>
                            ) : null}
                            {vehicle.is_simulated ? ' · Simulated' : ''}
                          </small>
                        </div>
                      </td>
                      <td>
                        <div className="admin-route-cell">
                          <span className="route-number">{vehicle.route_number}</span>
                          <div>
                            <strong>{vehicle.route_name}</strong>
                            <small>
                              {directionLabel(vehicle.direction)}
                              {vehicle.trip_id ? ` · Trip ${vehicle.trip_id}` : ''}
                            </small>
                          </div>
                        </div>
                      </td>
                      <td className="tabular-cell">
                        {vehicle.latitude.toFixed(4)}, {vehicle.longitude.toFixed(4)}
                      </td>
                      <td className="tabular-cell">
                        {typeof vehicle.speed_kmh === 'number' ? `${Math.round(vehicle.speed_kmh)} km/h` : '—'}
                        {' · '}
                        {typeof vehicle.heading === 'number' ? `${Math.round(vehicle.heading)}°` : '—'}
                      </td>
                      <td>
                        <span className={`status-pill ${vehicleStatusPillClass(vehicle.status)}`}>
                          {vehicleStatusLabel(vehicle.status)}
                        </span>
                      </td>
                      <td>
                        <div>
                          <time dateTime={vehicle.recorded_at}>{formatTime(vehicle.recorded_at)}</time>
                          <small className="muted-block">{vehicle.source_service || 'transit'}</small>
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
