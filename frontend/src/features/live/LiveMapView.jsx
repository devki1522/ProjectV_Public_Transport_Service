import { DepartureBoard } from './DepartureBoard.jsx'
import { VehicleArrivals } from './VehicleArrivals.jsx'
import { RouteBadge } from '../../components/RouteBadge.jsx'
import { FeedFreshness } from '../../components/FeedFreshness.jsx'
import { useCallback, useMemo, useState } from 'react'
import {
  AlertTriangle,
  List,
  Map as MapIcon,
  Pause,
  Play,
  Radio,
  RefreshCw,
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
  vehicleStatusLabel,
  vehicleStatusPillClass,
} from './vehicleFormat.js'

const LIVE_POLL_MS = 3_000
const SIMULATOR_POLL_MS = 15_000
const ARRIVALS_POLL_MS = 15_000

export function LiveMapView({ routes = [], selectedRouteId = '', onSelectedRouteChange }) {
  const [focusedRouteId, setFocusedRouteId] = useState(null)
  const [routeSearch, setRouteSearch] = useState('')
  const [layout, setLayout] = useState('map')
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

  const displayedRouteId = focusedRouteId ?? selectedVehicle?.route_id ?? parsedRouteId
  const loadRouteShape = useCallback(
    (signal) => getRouteShape(displayedRouteId, signal),
    [displayedRouteId],
  )
  const routeShapeRes = useResource(
    displayedRouteId > 0 ? `route-shape-${displayedRouteId}` : null,
    loadRouteShape,
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
    setFocusedRouteId(null)
    onSelectedRouteChange?.(event.target.value)
  }

  function handleStatusChange(event) {
    setSelectedVehicleId(null)
    setStatusFilter(event.target.value)
  }

  function handleShowAllRoutes() {
    setSelectedVehicleId(null)
    setLastVehiclesError(null)
    setFocusedRouteId(null)
    setStatusFilter('all')
    onSelectedRouteChange?.('')
  }

  function handleRefresh() {
    routeShapeRes.retry()
    vehiclesRes.retry()
    simulatorRes.retry()
    if (selectedTripId) {
      arrivalsRes.retry()
    }
  }

  const handleSelectVehicle = useCallback((vehicleId) => {
    setFocusedRouteId(null)
    setSelectedVehicleId(vehicleId)
  }, [])

  const routeShape = displayedRouteId > 0 ? routeShapeRes.data : null
  const mapVehicles = displayedRouteId > 0 ? filteredVehicles.filter((vehicle) => vehicle.route_id === displayedRouteId) : filteredVehicles

  return (
    <section className="primary-transit" aria-label="Transit map and departure board">
      <div className="primary-view-heading">
        <div><h1>Live Map</h1><span>Waterloo Region</span></div>
        <div className="primary-live-actions">
          <button type="button" className="primary-live-toggle" aria-pressed={autoRefresh} onClick={() => setAutoRefresh(!autoRefresh)}>
            {autoRefresh ? <Pause size={14} aria-hidden="true" /> : <Play size={14} aria-hidden="true" />}<span>{autoRefresh ? 'Live · 3s' : 'Paused'}</span>
          </button>
          <button type="button" className="primary-refresh" aria-label="Refresh" onClick={handleRefresh} disabled={vehiclesRes.pending}><RefreshCw size={17} aria-hidden="true" /></button>
        </div>
      </div>
      {vehiclesError && <ErrorNotice error={vehiclesError} onRetry={handleRefresh} />}
      <FeedFreshness updatedAt={vehiclesRes.updatedAt} paused={!autoRefresh} maxAge={30_000} />
      {vehiclesError && vehiclesRes.data && <p className="primary-feed-note" role="status">Showing the last successful vehicle positions. Bus locations may have moved.</p>}
      {routeShapeRes.error && <ErrorNotice error={routeShapeRes.error} onRetry={routeShapeRes.retry} />}
      {!autoRefresh && <p className="primary-feed-note"><Pause size={14} aria-hidden="true" />Updates paused. Resume to check current arrivals.</p>}
      <div className="primary-mobile-switch" role="group" aria-label="Map or vehicle list">
        <button type="button" aria-pressed={layout === 'map'} onClick={() => setLayout('map')}><MapIcon size={15} aria-hidden="true" />Map</button>
        <button type="button" aria-pressed={layout === 'list'} onClick={() => setLayout('list')}><List size={15} aria-hidden="true" />Departures</button>
      </div>
      <div className={`primary-layout primary-layout-${layout}`}>
        <div className="primary-map">
          <div className="primary-map-heading"><span>{routeShape ? `Route ${routeShape.route_number} / ${routeShape.route_name}` : 'Regional network'}</span><span><Radio size={12} aria-hidden="true" />{mapVehicles.length} {mapVehicles.length === 1 ? 'bus' : 'buses'}</span></div>
          <LeafletMap vehicles={mapVehicles} routeShape={routeShape} selectedVehicleId={selectedVehicle?.vehicle_id ?? null} onSelectVehicle={handleSelectVehicle} />
          <div className="primary-map-footer"><span>{routeShape ? 'Full route line shown' : 'Select a bus or departure to see its route'}</span><button type="button" onClick={handleShowAllRoutes}>Show all routes</button></div>
          {showSimulationNotice && <p className="primary-simulation" role="status"><Radio size={13} aria-hidden="true" /><span>{simulatorStatus?.running ? `Simulated fleet movement is active. ${simulatorStatus.vehicle_count} demonstration buses; arrivals include simulated service.` : 'Active vehicles include simulated positions and estimates.'}</span></p>}
        </div>
        <div className="primary-panel">
          <DepartureBoard live={autoRefresh} previewRouteId={displayedRouteId} onPreviewRoute={(id) => { setSelectedVehicleId(null); setFocusedRouteId(Number(id)) }} />
          {selectedVehicle && <VehicleArrivals vehicle={selectedVehicle} resource={arrivalsRes} onClear={() => setSelectedVehicleId(null)} />}
          <details className="primary-route-filters">
            <summary>Routes &amp; filters</summary>
            <div className="primary-filter-fields">
              <label htmlFor="live-route-search">Find a route<input id="live-route-search" type="search" placeholder="Route number or name" value={routeSearch} onChange={(event) => setRouteSearch(event.target.value)} /></label>
              <label htmlFor="live-route-filter">Route<select id="live-route-filter" value={selectedRouteId} onChange={handleRouteChange}><option value="">All active routes</option>{sortedRoutes.filter((route) => String(route.id) === selectedRouteId || `${route.number} ${route.name}`.toLowerCase().includes(routeSearch.toLowerCase())).map((route) => <option key={route.id} value={String(route.id)}>Route {route.number} · {route.name} ({directionLabel(route.direction)})</option>)}</select></label>
              <label htmlFor="live-status-filter">Status<select id="live-status-filter" value={statusFilter} onChange={handleStatusChange}><option value="all">All statuses</option><option value="in_service">In service</option><option value="delayed">Delayed</option><option value="layover">Layover</option></select></label>
            </div>
          </details>
          <section className="primary-fleet" aria-labelledby="live-vehicles-heading">
            <div className="primary-fleet-heading"><h2 id="live-vehicles-heading">Active vehicles</h2><span>{filteredVehicles.length} reporting</span></div>
            {!vehiclesRes.data && !vehiclesRes.error ? <LoadingRows label="Loading active vehicles" variant="vehicles" /> : filteredVehicles.length === 0 ? <EmptyState title={activeFilterApplied ? 'No active vehicles match this filter' : 'No active vehicles right now'} description="Show all routes or refresh to check for active service." onRetry={activeFilterApplied ? handleShowAllRoutes : handleRefresh} actionLabel={activeFilterApplied ? 'Show all routes' : 'Refresh'} /> : <ul className="primary-fleet-list" aria-label="Active vehicles list" aria-busy={vehiclesRes.pending}>
              {filteredVehicles.map((vehicle) => <li key={vehicle.vehicle_id}><button type="button" aria-pressed={selectedVehicleId === vehicle.vehicle_id} onClick={() => handleSelectVehicle(vehicle.vehicle_id)}><RouteBadge number={vehicle.route_number} /><span><strong>{vehicle.route_name}</strong><small>Bus #{vehicle.vehicle_number} · {directionLabel(vehicle.direction)}</small>{vehicle.is_simulated && <small>Simulated</small>}</span><span className={`status-pill ${vehicleStatusPillClass(vehicle.status)}`}>{vehicle.status === 'delayed' && <AlertTriangle size={12} aria-hidden="true" />}{vehicleStatusLabel(vehicle.status)}</span></button></li>)}
            </ul>}
          </section>
          <p className="primary-feed-timestamp">{vehiclesRes.updatedAt ? `Fleet updated ${formatTime(vehiclesRes.updatedAt)}` : 'Checking the network'}. All times Eastern.</p>
        </div>
      </div>
    </section>
  )
}
