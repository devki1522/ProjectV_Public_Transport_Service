import { useCallback, useMemo, useState } from 'react'
import {
  Accessibility,
  Clock3,
  Info,
  Pause,
  Play,
  Radio,
  RefreshCw,
  X,
} from 'lucide-react'
import {
  getActiveVehicles,
  getRouteShape,
  getSimulatorStatus,
  getTripArrivals,
} from '../../api/transit.js'
import { useResource } from '../../hooks/useResource.js'
import { directionLabel, formatTime } from '../../lib/transit.js'
import { EmptyState, ErrorNotice, LoadingRows } from '../../components/DataState.jsx'
import { LeafletMap } from './LeafletMap.jsx'
import {
  formatDelayMinutes,
  formatDistance,
  vehicleStatusLabel,
  vehicleStatusPillClass,
} from './vehicleFormat.js'

const LIVE_POLL_MS = 3_000
const SIMULATOR_POLL_MS = 15_000
const ARRIVALS_POLL_MS = 15_000

export function LiveMapView({ routes = [], selectedRouteId = '', onSelectedRouteChange }) {
  const [statusFilter, setStatusFilter] = useState('all')
  const [autoRefresh, setAutoRefresh] = useState(true)
  const [selectedVehicleId, setSelectedVehicleId] = useState(null)
  const [lastVehiclesError, setLastVehiclesError] = useState(null)

  const sortedRoutes = useMemo(
    () =>
      [...routes].sort((a, b) => {
        const numberOrder = a.number.localeCompare(b.number, 'en', { numeric: true })
        return numberOrder !== 0 ? numberOrder : a.direction.localeCompare(b.direction)
      }),
    [routes],
  )

  const parsedRouteId = selectedRouteId ? Number(selectedRouteId) : 0
  const loadVehicles = useCallback(
    async (signal) => {
      try {
        const data = await getActiveVehicles(parsedRouteId > 0 ? { routeId: parsedRouteId } : {}, signal)
        setLastVehiclesError(null)
        return data
      } catch (error) {
        if (error?.name !== 'AbortError') {
          setLastVehiclesError(error)
        }
        throw error
      }
    },
    [parsedRouteId],
  )
  const vehiclesRes = useResource(
    `live-vehicles-${parsedRouteId || 'all'}`,
    loadVehicles,
    autoRefresh ? LIVE_POLL_MS : 0,
  )

  const vehiclesError = vehiclesRes.error ?? lastVehiclesError

  const loadRouteShape = useCallback(
    (signal) => getRouteShape(parsedRouteId, signal),
    [parsedRouteId],
  )
  const routeShapeRes = useResource(
    parsedRouteId > 0 ? `route-shape-${parsedRouteId}` : null,
    loadRouteShape,
  )

  const loadSimulator = useCallback((signal) => getSimulatorStatus(signal), [])
  const simulatorRes = useResource(
    'live-simulator-status',
    loadSimulator,
    autoRefresh ? SIMULATOR_POLL_MS : 0,
  )

  const allVehicles = useMemo(() => vehiclesRes.data ?? [], [vehiclesRes.data])
  const filteredVehicles = useMemo(() => {
    if (statusFilter === 'all') return allVehicles
    return allVehicles.filter((vehicle) => vehicle.status === statusFilter)
  }, [allVehicles, statusFilter])

  const selectedVehicle = useMemo(
    () => filteredVehicles.find((vehicle) => vehicle.vehicle_id === selectedVehicleId) ?? null,
    [filteredVehicles, selectedVehicleId],
  )

  const selectedTripId = selectedVehicle?.trip_id ?? null
  const loadArrivals = useCallback(
    (signal) => getTripArrivals(selectedTripId, signal),
    [selectedTripId],
  )
  const arrivalsRes = useResource(
    selectedTripId ? `trip-arrivals-${selectedTripId}` : null,
    loadArrivals,
    autoRefresh ? ARRIVALS_POLL_MS : 0,
  )

  const simulatorStatus = simulatorRes.data
  const hasSimulatedVehicles = allVehicles.some((vehicle) => vehicle.is_simulated)
  const showSimulationNotice = Boolean(simulatorStatus?.running || hasSimulatedVehicles)
  const activeFilterApplied = Boolean(selectedRouteId) || statusFilter !== 'all'

  function handleRouteChange(event) {
    setSelectedVehicleId(null)
    setLastVehiclesError(null)
    onSelectedRouteChange?.(event.target.value)
  }

  function handleStatusChange(event) {
    setSelectedVehicleId(null)
    setStatusFilter(event.target.value)
  }

  function handleShowAllRoutes() {
    setSelectedVehicleId(null)
    setLastVehiclesError(null)
    setStatusFilter('all')
    onSelectedRouteChange?.('')
  }

  function handleRefresh() {
    vehiclesRes.retry()
    simulatorRes.retry()
    if (selectedTripId) {
      arrivalsRes.retry()
    }
  }

  const handleSelectVehicle = useCallback((vehicleId) => {
    setSelectedVehicleId(vehicleId)
  }, [])

  const routeShape = parsedRouteId > 0 ? routeShapeRes.data : null

  return (
    <>
      <div className="view-toolbar live-toolbar">
        <div className="field live-route-field">
          <label htmlFor="live-route-filter">Route</label>
          <select
            id="live-route-filter"
            value={selectedRouteId}
            onChange={handleRouteChange}
          >
            <option value="">All active routes</option>
            {sortedRoutes.map((route) => (
              <option key={route.id} value={String(route.id)}>
                Route {route.number} · {route.name} ({directionLabel(route.direction)})
              </option>
            ))}
          </select>
        </div>

        <div className="field live-status-field">
          <label htmlFor="live-status-filter">Status</label>
          <select
            id="live-status-filter"
            value={statusFilter}
            onChange={handleStatusChange}
          >
            <option value="all">All statuses</option>
            <option value="in_service">In service</option>
            <option value="delayed">Delayed</option>
            <option value="layover">Layover</option>
          </select>
        </div>

        <div className="live-toolbar-actions">
          <button
            type="button"
            className={`button button-secondary live-toggle-button ${autoRefresh ? 'is-live' : ''}`}
            aria-pressed={autoRefresh}
            onClick={() => setAutoRefresh((enabled) => !enabled)}
          >
            {autoRefresh ? <Pause size={15} aria-hidden="true" /> : <Play size={15} aria-hidden="true" />}
            <span>{autoRefresh ? 'Live · 3s' : 'Paused'}</span>
          </button>

          <button
            type="button"
            className="button button-secondary refresh-button"
            onClick={handleRefresh}
            disabled={vehiclesRes.pending}
          >
            <RefreshCw size={15} aria-hidden="true" />
            <span>{vehiclesRes.pending && vehiclesRes.data ? 'Updating…' : 'Refresh'}</span>
          </button>
        </div>

        <p className="live-updated-label">
          {vehiclesRes.updatedAt
            ? `Updated ${formatTime(vehiclesRes.updatedAt)}`
            : autoRefresh
              ? 'Auto-refreshes every 3s'
              : 'Auto-refresh paused'}
        </p>
      </div>

      {showSimulationNotice && (
        <div className="estimate-note has-estimates live-sim-banner" role="status">
          <Info size={17} aria-hidden="true" />
          <p>
            {simulatorStatus?.running
              ? `Simulated fleet movement is active (${simulatorStatus.vehicle_count} simulated ${simulatorStatus.vehicle_count === 1 ? 'bus' : 'buses'} across ${simulatorStatus.route_count} ${simulatorStatus.route_count === 1 ? 'route' : 'routes'} at ${simulatorStatus.speed_multiplier}x speed). Live map positions and arrival estimates include simulated buses alongside any reported GPS updates.`
              : 'Active vehicles include simulated bus positions for demonstration alongside any reported GPS updates.'}
          </p>
        </div>
      )}

      {vehiclesError && <ErrorNotice error={vehiclesError} onRetry={handleRefresh} />}
      {vehiclesError && vehiclesRes.data && (
        <p className="stale-notice" role="status">
          Showing the last successful vehicle positions. Bus locations may have moved.
        </p>
      )}

      <div className="live-grid">
        <div className="live-map-column">
          <LeafletMap
            vehicles={filteredVehicles}
            routeShape={routeShape}
            selectedVehicleId={selectedVehicle?.vehicle_id ?? null}
            onSelectVehicle={handleSelectVehicle}
          />

          <div className="live-map-legend" aria-label="Map legend">
            <span className="legend-item">
              <span className="legend-dot is-in-service" aria-hidden="true" />
              In service
            </span>
            <span className="legend-item">
              <span className="legend-dot is-delayed" aria-hidden="true" />
              Delayed
            </span>
            <span className="legend-item">
              <span className="legend-dot is-layover" aria-hidden="true" />
              Layover
            </span>
            {routeShape?.coordinates?.length >= 2 && (
              <span className="legend-item">
                <span className="legend-line" aria-hidden="true" />
                Route {routeShape.route_number} path
              </span>
            )}
            <span className="legend-count">
              {filteredVehicles.length} active {filteredVehicles.length === 1 ? 'bus' : 'buses'}
            </span>
          </div>
        </div>

        <div className="live-sidebar-column">
          <section className="live-panel" aria-labelledby="live-vehicles-heading">
            <div className="live-panel-header">
              <div>
                <h2 id="live-vehicles-heading">Active vehicles</h2>
                <p>
                  {filteredVehicles.length}{' '}
                  {filteredVehicles.length === 1 ? 'bus' : 'buses'} reporting
                </p>
              </div>
              {activeFilterApplied && (
                <button type="button" className="text-button" onClick={handleShowAllRoutes}>
                  Show all routes
                </button>
              )}
            </div>

            {!vehiclesRes.data && !vehiclesRes.error ? (
              <LoadingRows label="Loading active vehicles" />
            ) : filteredVehicles.length === 0 ? (
              <EmptyState
                title={activeFilterApplied ? 'No active vehicles match this filter' : 'No active vehicles right now'}
                description={
                  activeFilterApplied
                    ? 'No buses are currently reporting for the selected route or status. Show all routes to view the full active fleet.'
                    : 'No transit vehicles have reported recent positions. Refresh to check again in a moment.'
                }
                onRetry={activeFilterApplied ? handleShowAllRoutes : handleRefresh}
                actionLabel={activeFilterApplied ? 'Show all routes' : 'Refresh'}
              />
            ) : (
              <ul className="live-vehicle-list" aria-label="Active vehicles list" aria-busy={vehiclesRes.pending}>
                {filteredVehicles.map((vehicle) => {
                  const isSelected = selectedVehicle?.vehicle_id === vehicle.vehicle_id
                  return (
                    <li key={vehicle.vehicle_id}>
                      <button
                        type="button"
                        className={`live-vehicle-card ${isSelected ? 'is-selected' : ''}`}
                        aria-pressed={isSelected}
                        onClick={() => handleSelectVehicle(vehicle.vehicle_id)}
                      >
                        <span className="route-number">{vehicle.route_number}</span>
                        <div className="live-vehicle-info">
                          <div className="live-vehicle-top">
                            <strong>{vehicle.route_name}</strong>
                            <span className={`status-pill ${vehicleStatusPillClass(vehicle.status)}`}>
                              {vehicleStatusLabel(vehicle.status)}
                            </span>
                          </div>
                          <p className="live-vehicle-meta">
                            Bus #{vehicle.vehicle_number} · {directionLabel(vehicle.direction)}
                            {vehicle.trip_id ? ` · Trip ${vehicle.trip_id}` : ''}
                          </p>
                          <div className="live-vehicle-tags">
                            {typeof vehicle.speed_kmh === 'number' && Number.isFinite(vehicle.speed_kmh) && (
                              <span>{Math.round(vehicle.speed_kmh)} km/h</span>
                            )}
                            <span>Reported {formatTime(vehicle.recorded_at)}</span>
                            {vehicle.is_accessible && (
                              <span className="live-tag-accessible">
                                <Accessibility size={12} aria-hidden="true" />
                                Accessible
                              </span>
                            )}
                            {vehicle.is_simulated && (
                              <span className="live-tag-simulated">Simulated</span>
                            )}
                          </div>
                        </div>
                      </button>
                    </li>
                  )
                })}
              </ul>
            )}
          </section>

          <section className="live-panel live-arrivals-panel" aria-labelledby="live-arrivals-heading">
            <div className="live-panel-header">
              <div>
                <h2 id="live-arrivals-heading">Upcoming estimated arrivals</h2>
                <p>
                  {selectedVehicle
                    ? `Bus #${selectedVehicle.vehicle_number} · Route ${selectedVehicle.route_number}${selectedVehicle.trip_id ? ` · Trip ${selectedVehicle.trip_id}` : ''}`
                    : 'Select an active bus to view upcoming stop arrival estimates.'}
                </p>
              </div>
              {selectedVehicle && (
                <button
                  type="button"
                  className="button button-secondary button-compact"
                  onClick={() => setSelectedVehicleId(null)}
                >
                  <X size={14} aria-hidden="true" />
                  <span>Clear</span>
                </button>
              )}
            </div>

            {!selectedVehicle ? (
              <div className="live-arrivals-placeholder">
                <Radio size={20} aria-hidden="true" />
                <p>Choose a bus from the map or list above to see estimated arrival times for its upcoming stops.</p>
              </div>
            ) : !selectedVehicle.trip_id ? (
              <div className="live-arrivals-placeholder" role="status">
                <Info size={18} aria-hidden="true" />
                <p>This vehicle is not assigned to an active scheduled trip, so stop arrival estimates are unavailable.</p>
              </div>
            ) : arrivalsRes.error ? (
              <ErrorNotice error={arrivalsRes.error} onRetry={arrivalsRes.retry} />
            ) : !arrivalsRes.data ? (
              <LoadingRows label="Loading estimated arrivals" />
            ) : arrivalsRes.data.length === 0 ? (
              <div className="live-arrivals-placeholder" role="status">
                <Clock3 size={18} aria-hidden="true" />
                <p>No upcoming stop arrival estimates are available for Trip {selectedVehicle.trip_id} right now.</p>
              </div>
            ) : (
              <ol className="live-arrival-list" aria-label={`Estimated arrivals for Trip ${selectedVehicle.trip_id}`}>
                {arrivalsRes.data.map((arrival) => {
                  const delayText = formatDelayMinutes(arrival.delay_minutes)
                  const distanceText = formatDistance(arrival.distance_meters)
                  return (
                    <li
                      key={`${arrival.trip_id}-${arrival.stop_id}-${arrival.stop_sequence}`}
                      className="live-arrival-row"
                    >
                      <div className="live-arrival-stop">
                        <span className="stop-sequence">{arrival.stop_sequence}</span>
                        <div>
                          <strong>{arrival.stop_name}</strong>
                          <small>
                            Scheduled {formatTime(arrival.scheduled_arrival_time)}
                            {distanceText ? ` · ${distanceText} away` : ''}
                          </small>
                        </div>
                      </div>
                      <div className="live-arrival-time is-estimated">
                        <time dateTime={arrival.estimated_arrival_time}>
                          {formatTime(arrival.estimated_arrival_time)}
                        </time>
                        <span className="time-source">Estimated</span>
                        {delayText && (
                          <small className={arrival.delay_minutes > 0 ? 'delay-label' : ''}>
                            {delayText}
                          </small>
                        )}
                      </div>
                    </li>
                  )
                })}
              </ol>
            )}
          </section>
        </div>
      </div>

      <div className="schedule-note">
        <p>Live map displays active vehicles reported within the last 15 minutes using OpenStreetMap map tiles. All times are Eastern (24-hour clock).</p>
        <p>Estimated arrivals combine live vehicle position and speed with upcoming scheduled stops and are clearly marked as Estimated.</p>
      </div>
    </>
  )
}
