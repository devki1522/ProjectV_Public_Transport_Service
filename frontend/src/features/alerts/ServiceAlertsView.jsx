import { useMemo, useState } from 'react'
import {
  AlertTriangle,
  CalendarDays,
  CheckCircle2,
  Clock3,
  Map as MapIcon,
  MapPin,
  Radio,
  RefreshCw,
} from 'lucide-react'
import { getServiceStatus } from '../../api/transit.js'
import { EmptyState, ErrorNotice, LoadingRows } from '../../components/DataState.jsx'
import { useResource } from '../../hooks/useResource.js'
import { directionLabel, formatTime } from '../../lib/transit.js'
import {
  DISRUPTION_TYPE_OPTIONS,
  disruptionStatusLabel,
  disruptionStatusPillClass,
  disruptionTypeClass,
  disruptionTypeLabel,
  formatAlertTimestamp,
  formatDisruptionWindow,
  operationalStatusLabel,
  operationalStatusPillClass,
  sourceServiceLabel,
} from './disruptionFormat.js'

function isServiceStatusObject(data) {
  return Boolean(data && typeof data === 'object' && !Array.isArray(data) && Array.isArray(data.active_disruptions))
}

export function ServiceAlertsView({
  routes = [],
  serviceStatusResource = null,
  selectedRouteId = '',
  onSelectedRouteChange = null,
  onSelectRouteSchedule = null,
  onSelectRouteLiveMap = null,
}) {
  const localResource = useResource(serviceStatusResource ? null : 'service-alerts-status', getServiceStatus)
  const resource = serviceStatusResource ?? localResource

  const [typeFilter, setTypeFilter] = useState('')
  const [localRouteFilter, setLocalRouteFilter] = useState('')
  const routeFilter = onSelectedRouteChange ? selectedRouteId : localRouteFilter
  const setRouteFilter = onSelectedRouteChange ?? setLocalRouteFilter

  const statusData = isServiceStatusObject(resource.data) ? resource.data : null
  const activeDisruptions = useMemo(() => statusData?.active_disruptions ?? [], [statusData])
  const affectedRoutes = useMemo(() => statusData?.affected_routes ?? [], [statusData])
  const affectedStops = useMemo(() => statusData?.affected_stops ?? [], [statusData])
  const affectedTrips = useMemo(() => statusData?.affected_trips ?? [], [statusData])
  const affectedVehicles = useMemo(() => statusData?.affected_vehicles ?? [], [statusData])
  const affectedSegments = useMemo(() => statusData?.affected_segments ?? [], [statusData])

  const routeOptions = useMemo(() => {
    const byId = new Map()
    for (const route of routes) {
      byId.set(route.id, {
        id: route.id,
        number: route.number,
        name: route.name,
        direction: route.direction,
      })
    }
    for (const affected of affectedRoutes) {
      if (!byId.has(affected.route_id)) {
        byId.set(affected.route_id, {
          id: affected.route_id,
          number: affected.route_number,
          name: affected.route_name,
          direction: affected.direction,
        })
      }
    }
    return [...byId.values()].sort((a, b) => {
      const numCmp = String(a.number).localeCompare(String(b.number), 'en', { numeric: true })
      return numCmp !== 0 ? numCmp : String(a.direction ?? '').localeCompare(String(b.direction ?? ''))
    })
  }, [routes, affectedRoutes])

  const selectedRouteNum = routeFilter ? Number(routeFilter) : null

  const filteredDisruptions = useMemo(() => {
    return activeDisruptions.filter((item) => {
      if (typeFilter && item.type !== typeFilter) return false
      if (selectedRouteNum) {
        const matchesRoute = Array.isArray(item.affected_routes) &&
          item.affected_routes.some((r) => r.route_id === selectedRouteNum)
        if (!matchesRoute) return false
      }
      return true
    })
  }, [activeDisruptions, typeFilter, selectedRouteNum])

  const filteredDisruptionIds = useMemo(
    () => new Set(filteredDisruptions.map((item) => item.id)),
    [filteredDisruptions],
  )

  const hasActiveFilter = Boolean(typeFilter || routeFilter)

  const visibleAffectedRoutes = useMemo(() => {
    return affectedRoutes.filter((route) => {
      if (selectedRouteNum && route.route_id !== selectedRouteNum) return false
      if (typeFilter && Array.isArray(route.disruption_ids)) {
        return route.disruption_ids.some((id) => filteredDisruptionIds.has(id))
      }
      return true
    })
  }, [affectedRoutes, selectedRouteNum, typeFilter, filteredDisruptionIds])

  const visibleAffectedStops = useMemo(() => {
    return affectedStops.filter((stop) => {
      if (hasActiveFilter && Array.isArray(stop.disruption_ids)) {
        return stop.disruption_ids.some((id) => filteredDisruptionIds.has(id))
      }
      return true
    })
  }, [affectedStops, hasActiveFilter, filteredDisruptionIds])

  const visibleAffectedTrips = useMemo(() => {
    return affectedTrips.filter((trip) => {
      if (selectedRouteNum && trip.route_id && trip.route_id !== selectedRouteNum) return false
      if (hasActiveFilter && Array.isArray(trip.disruption_ids)) {
        return trip.disruption_ids.some((id) => filteredDisruptionIds.has(id))
      }
      return true
    })
  }, [affectedTrips, selectedRouteNum, hasActiveFilter, filteredDisruptionIds])

  const visibleAffectedVehicles = useMemo(() => {
    return affectedVehicles.filter((vehicle) => {
      if (hasActiveFilter && Array.isArray(vehicle.disruption_ids)) {
        return vehicle.disruption_ids.some((id) => filteredDisruptionIds.has(id))
      }
      return true
    })
  }, [affectedVehicles, hasActiveFilter, filteredDisruptionIds])

  const visibleAffectedSegments = useMemo(() => {
    return affectedSegments.filter((segment) => {
      if (selectedRouteNum && segment.route_id && segment.route_id !== selectedRouteNum) return false
      if (hasActiveFilter && segment.disruption_id) {
        return filteredDisruptionIds.has(segment.disruption_id)
      }
      return true
    })
  }, [affectedSegments, selectedRouteNum, hasActiveFilter, filteredDisruptionIds])

  function clearFilters() {
    setTypeFilter('')
    setRouteFilter('')
  }

  const totalActiveCount = statusData?.active_disruption_count ?? activeDisruptions.length
  const isDisrupted = statusData?.overall_status === 'disrupted' || totalActiveCount > 0

  return (
    <div className="alerts-view">
      <div className="view-toolbar alerts-toolbar">
        <div className="field alerts-type-field">
          <label htmlFor="alerts-type-filter">Disruption type</label>
          <select
            id="alerts-type-filter"
            value={typeFilter}
            onChange={(event) => setTypeFilter(event.target.value)}
          >
            <option value="">All types</option>
            {DISRUPTION_TYPE_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </div>

        <div className="field alerts-route-field">
          <label htmlFor="alerts-route-filter">Route</label>
          <select
            id="alerts-route-filter"
            value={routeFilter}
            onChange={(event) => setRouteFilter(event.target.value)}
          >
            <option value="">All routes</option>
            {routeOptions.map((route) => (
              <option key={route.id} value={String(route.id)}>
                Route {route.number} · {route.name} ({directionLabel(route.direction)})
              </option>
            ))}
          </select>
        </div>

        <div className="alerts-toolbar-actions">
          <button
            type="button"
            className="button button-secondary"
            onClick={resource.retry}
            disabled={resource.pending}
          >
            <RefreshCw size={15} aria-hidden="true" />
            <span>{resource.pending ? 'Refreshing…' : 'Refresh'}</span>
          </button>
          {hasActiveFilter && (
            <button type="button" className="text-button" onClick={clearFilters}>
              Clear filters
            </button>
          )}
        </div>

        {statusData?.generated_at && (
          <span className="alerts-updated-label">
            Updated {formatTime(statusData.generated_at)}
          </span>
        )}
      </div>

      {resource.error && !statusData ? (
        <ErrorNotice error={resource.error} onRetry={resource.retry} />
      ) : resource.pending && !statusData ? (
        <LoadingRows label="Loading active transit disruptions" />
      ) : (
        <>
          {resource.error && statusData && (
            <p className="stale-notice" role="status">
              Could not refresh service alerts ({resource.error.message}). Showing the last loaded snapshot.
            </p>
          )}

          {statusData && (
            <div
              className={`alerts-status-banner ${isDisrupted ? 'is-disrupted' : 'is-normal'}`}
              role="status"
              aria-label="Overall transit service status"
            >
              <div className="alerts-status-banner-main">
                {isDisrupted ? (
                  <AlertTriangle size={20} aria-hidden="true" />
                ) : (
                  <CheckCircle2 size={20} aria-hidden="true" />
                )}
                <div>
                  <strong>
                    {isDisrupted
                      ? `${totalActiveCount} active ${totalActiveCount === 1 ? 'disruption' : 'disruptions'} reported`
                      : 'All services operating normally'}
                  </strong>
                  <p>
                    {isDisrupted
                      ? `${affectedRoutes.length} affected ${affectedRoutes.length === 1 ? 'route' : 'routes'} and ${affectedStops.length} affected ${affectedStops.length === 1 ? 'stop' : 'stops'} across Waterloo Region.`
                      : 'No active service disruptions, detours, or stop closures are currently in effect.'}
                  </p>
                </div>
              </div>
              <span className={`status-pill ${isDisrupted ? 'is-disrupted' : 'is-active'}`}>
                {isDisrupted ? 'Service disrupted' : 'Normal service'}
              </span>
            </div>
          )}

          {activeDisruptions.length === 0 ? (
            <div className="empty-state alerts-normal-state" role="status">
              <CheckCircle2 size={32} aria-hidden="true" className="alerts-normal-icon" />
              <h3>All services operating normally</h3>
              <p>
                There are no active disruptions, detours, road closures, or stop closures reported across Waterloo Region transit routes right now.
              </p>
            </div>
          ) : (
            <div className="alerts-grid">
              <section className="alerts-column" aria-label="Active disruptions">
                <div className="alerts-section-heading">
                  <div>
                    <h2>Active disruptions</h2>
                    <p>
                      {hasActiveFilter
                        ? `Showing ${filteredDisruptions.length} of ${activeDisruptions.length} active disruptions`
                        : `${activeDisruptions.length} active ${activeDisruptions.length === 1 ? 'disruption' : 'disruptions'} currently in effect`}
                    </p>
                  </div>
                </div>

                {filteredDisruptions.length === 0 ? (
                  <EmptyState
                    title="No disruptions match this filter"
                    description="Try selecting a different disruption type or route to view all active service alerts."
                    onRetry={clearFilters}
                    actionLabel="Show all disruptions"
                  />
                ) : (
                  <div className="alerts-list">
                    {filteredDisruptions.map((disruption) => (
                      <article
                        key={disruption.id}
                        className={`alert-card ${disruptionTypeClass(disruption.type)}`}
                      >
                        <div className="alert-card-header">
                          <div className="alert-card-pills">
                            <span className={`status-pill ${disruptionTypeClass(disruption.type)}`}>
                              {disruptionTypeLabel(disruption.type)}
                            </span>
                            <span className={`status-pill ${disruptionStatusPillClass(disruption.status)}`}>
                              {disruptionStatusLabel(disruption.status)}
                            </span>
                          </div>
                          <span className="alert-source-tag">
                            {sourceServiceLabel(disruption.source_service)}
                          </span>
                        </div>

                        <h3 className="alert-card-title">{disruption.title}</h3>
                        {disruption.description && (
                          <p className="alert-card-description">{disruption.description}</p>
                        )}

                        <dl className="alert-card-meta">
                          {disruption.affected_area && (
                            <div className="alert-meta-item">
                              <dt className="visually-hidden">Affected area</dt>
                              <dd>
                                <MapPin size={14} aria-hidden="true" />
                                <span>{disruption.affected_area}</span>
                              </dd>
                            </div>
                          )}
                          <div className="alert-meta-item">
                            <dt className="visually-hidden">Time window</dt>
                            <dd>
                              <Clock3 size={14} aria-hidden="true" />
                              <span>{formatDisruptionWindow(disruption.start_time, disruption.end_time)}</span>
                            </dd>
                          </div>
                          {disruption.received_at && (
                            <div className="alert-meta-item">
                              <dt className="visually-hidden">Received</dt>
                              <dd>
                                <span>Reported {formatAlertTimestamp(disruption.received_at)}</span>
                                {disruption.external_reference ? ` · Ref ${disruption.external_reference}` : ''}
                              </dd>
                            </div>
                          )}
                        </dl>

                        {((disruption.affected_routes?.length ?? 0) > 0 ||
                          (disruption.affected_stops?.length ?? 0) > 0 ||
                          (disruption.affected_trips?.length ?? 0) > 0 ||
                          (disruption.affected_vehicles?.length ?? 0) > 0 ||
                          (disruption.affected_segments?.length ?? 0) > 0) && (
                          <div className="alert-chips" aria-label="Affected services for this disruption">
                            {(disruption.affected_routes ?? []).map((route) => (
                              <span key={`route-${route.route_id}`} className="alert-chip alert-chip-route">
                                Route {route.route_number} · {route.route_name}
                              </span>
                            ))}
                            {(disruption.affected_stops ?? []).map((stop) => (
                              <span key={`stop-${stop.stop_id}-${stop.route_id ?? 'any'}`} className="alert-chip alert-chip-stop">
                                Stop: {stop.stop_name}
                              </span>
                            ))}
                            {(disruption.affected_trips ?? []).map((tripId) => (
                              <span key={`trip-${tripId}`} className="alert-chip">
                                Trip {tripId}
                              </span>
                            ))}
                            {(disruption.affected_vehicles ?? []).map((vehicleId) => (
                              <span key={`vehicle-${vehicleId}`} className="alert-chip">
                                Vehicle #{vehicleId}
                              </span>
                            ))}
                            {(disruption.affected_segments ?? []).map((segment, index) => (
                              <span key={`segment-${index}-${segment}`} className="alert-chip alert-chip-segment">
                                Segment: {segment}
                              </span>
                            ))}
                          </div>
                        )}
                      </article>
                    ))}
                  </div>
                )}
              </section>

              <section className="affected-summary-column" aria-label="Affected routes and stops">
                <div className="affected-panel">
                  <div className="alerts-section-heading">
                    <div>
                      <h2>Affected routes &amp; stops</h2>
                      <p>Operational status across impacted transit services</p>
                    </div>
                  </div>

                  <div className="affected-group">
                    <h3>Affected routes ({visibleAffectedRoutes.length})</h3>
                    {visibleAffectedRoutes.length === 0 ? (
                      <p className="affected-empty-note">No routes impacted for the current filter.</p>
                    ) : (
                      <ul className="affected-list">
                        {visibleAffectedRoutes.map((route) => (
                          <li key={route.route_id} className="affected-route-item">
                            <div className="affected-route-top">
                              <div className="affected-route-identity">
                                <span className="route-number">{route.route_number}</span>
                                <div>
                                  <strong>{route.route_name}</strong>
                                  <small>{directionLabel(route.direction)}</small>
                                </div>
                              </div>
                              <span className={`status-pill ${operationalStatusPillClass(route.operational_status)}`}>
                                {operationalStatusLabel(route.operational_status)}
                              </span>
                            </div>
                            <div className="affected-route-actions">
                              {onSelectRouteSchedule && (
                                <button
                                  type="button"
                                  className="button button-secondary button-compact"
                                  onClick={() => onSelectRouteSchedule(route.route_id)}
                                >
                                  <CalendarDays size={13} aria-hidden="true" />
                                  <span>View schedule</span>
                                </button>
                              )}
                              {onSelectRouteLiveMap && (
                                <button
                                  type="button"
                                  className="button button-secondary button-compact"
                                  onClick={() => onSelectRouteLiveMap(route.route_id)}
                                >
                                  <MapIcon size={13} aria-hidden="true" />
                                  <span>Live map</span>
                                </button>
                              )}
                            </div>
                          </li>
                        ))}
                      </ul>
                    )}
                  </div>

                  <div className="affected-group">
                    <h3>Affected stops ({visibleAffectedStops.length})</h3>
                    {visibleAffectedStops.length === 0 ? (
                      <p className="affected-empty-note">No stops impacted for the current filter.</p>
                    ) : (
                      <ul className="affected-list">
                        {visibleAffectedStops.map((stop) => (
                          <li key={stop.stop_id} className="affected-stop-item">
                            <div>
                              <strong>{stop.stop_name}</strong>
                              <small>Stop #{stop.stop_id}</small>
                            </div>
                            <span className={`status-pill ${operationalStatusPillClass(stop.operational_status)}`}>
                              {operationalStatusLabel(stop.operational_status)}
                            </span>
                          </li>
                        ))}
                      </ul>
                    )}
                  </div>

                  {visibleAffectedTrips.length > 0 && (
                    <div className="affected-group">
                      <h3>Affected trips ({visibleAffectedTrips.length})</h3>
                      <ul className="affected-compact-list">
                        {visibleAffectedTrips.map((trip) => (
                          <li key={trip.trip_id} className="affected-stop-item">
                            <div>
                              <strong>Trip {trip.trip_id}</strong>
                              {trip.route_id ? <small>Route ID {trip.route_id}</small> : null}
                            </div>
                            <span className={`status-pill ${operationalStatusPillClass(trip.operational_status)}`}>
                              {operationalStatusLabel(trip.operational_status)}
                            </span>
                          </li>
                        ))}
                      </ul>
                    </div>
                  )}

                  {visibleAffectedVehicles.length > 0 && (
                    <div className="affected-group">
                      <h3>Affected vehicles ({visibleAffectedVehicles.length})</h3>
                      <ul className="affected-compact-list">
                        {visibleAffectedVehicles.map((vehicle) => (
                          <li key={vehicle.vehicle_id} className="affected-stop-item">
                            <div>
                              <strong>
                                <Radio size={12} aria-hidden="true" /> Bus #{vehicle.vehicle_number || vehicle.vehicle_id}
                              </strong>
                            </div>
                            <span className={`status-pill ${operationalStatusPillClass(vehicle.operational_status)}`}>
                              {operationalStatusLabel(vehicle.operational_status)}
                            </span>
                          </li>
                        ))}
                      </ul>
                    </div>
                  )}

                  {visibleAffectedSegments.length > 0 && (
                    <div className="affected-group">
                      <h3>Affected route segments ({visibleAffectedSegments.length})</h3>
                      <ul className="affected-compact-list">
                        {visibleAffectedSegments.map((segment, index) => (
                          <li key={`${segment.disruption_id}-${index}`} className="affected-stop-item">
                            <div>
                              <strong>{segment.segment}</strong>
                            </div>
                            <span className={`status-pill ${operationalStatusPillClass(segment.operational_status)}`}>
                              {operationalStatusLabel(segment.operational_status)}
                            </span>
                          </li>
                        ))}
                      </ul>
                    </div>
                  )}
                </div>
              </section>
            </div>
          )}
        </>
      )}
    </div>
  )
}
