import { useEffect, useMemo, useState } from 'react'
import { AlertTriangle, ArrowRightLeft, BusFront, CalendarDays, Map as MapIcon, MapPin } from 'lucide-react'
import { getRoutes, getServiceStatus } from '../../api/transit.js'
import { useResource } from '../../hooks/useResource.js'
import { formatDate, groupRoutes, serviceDate } from '../../lib/transit.js'
import { Link } from '../../lib/router.jsx'
import { RouteSidebar } from '../../components/RouteSidebar.jsx'
import { EmptyState, ErrorNotice, LoadingRows } from '../../components/DataState.jsx'
import { TransitWorkspace } from '../TransitWorkspace.jsx'
import { JourneyView } from '../journey/JourneyView.jsx'
import { LiveMapView } from '../live/LiveMapView.jsx'
import { ServiceAlertsView } from '../alerts/ServiceAlertsView.jsx'

const RESIDENT_SECTIONS = ['schedules', 'journey', 'map', 'alerts']

export function ResidentPage() {
  const routes = useResource('routes', getRoutes)
  const groups = useMemo(() => groupRoutes(routes.data ?? []), [routes.data])
  const [section, setSection] = useState('schedules')
  const [routeId, setRouteId] = useState(null)
  const [mapRouteId, setMapRouteId] = useState('')
  const [alertsRouteId, setAlertsRouteId] = useState('')
  const [today, setToday] = useState(serviceDate)
  const [date, setDate] = useState(serviceDate)
  const [view, setView] = useState('schedule')
  const selectedRoute = routes.data?.find((route) => route.id === routeId) ?? groups[0]?.directions[0]
  const selectedGroup = groups.find((group) => group.number === selectedRoute?.number)

  const serviceStatusKey =
    section === 'alerts' || (Array.isArray(routes.data) && routes.data.length > 0)
      ? 'service-status'
      : null
  const serviceStatus = useResource(serviceStatusKey, getServiceStatus)
  const validServiceStatus =
    serviceStatus.data &&
    typeof serviceStatus.data === 'object' &&
    !Array.isArray(serviceStatus.data) &&
    Array.isArray(serviceStatus.data.active_disruptions)
      ? serviceStatus.data
      : null

  const activeDisruptionCount =
    validServiceStatus?.active_disruption_count ??
    validServiceStatus?.active_disruptions?.length ??
    0

  const selectedRouteAffected = useMemo(
    () =>
      selectedRoute && Array.isArray(validServiceStatus?.affected_routes)
        ? validServiceStatus.affected_routes.find((item) => item.route_id === selectedRoute.id) ?? null
        : null,
    [selectedRoute, validServiceStatus],
  )

  const selectedRouteDisruptions = useMemo(
    () =>
      selectedRoute && Array.isArray(validServiceStatus?.active_disruptions)
        ? validServiceStatus.active_disruptions.filter(
            (disruption) =>
              Array.isArray(disruption.affected_routes) &&
              disruption.affected_routes.some((route) => route.route_id === selectedRoute.id),
          )
        : [],
    [selectedRoute, validServiceStatus],
  )

  const selectedRouteOperationalStatus =
    selectedRouteAffected?.operational_status ??
    (selectedRouteDisruptions.length > 0 ? 'disrupted' : validServiceStatus ? 'normal' : null)

  useEffect(() => {
    if (section === 'journey') {
      document.title = 'Journey Search · Public Transport Waterloo Region'
    } else if (section === 'map') {
      document.title = 'Live Map · Public Transport Waterloo Region'
    } else if (section === 'alerts') {
      document.title = 'Service Alerts · Public Transport Waterloo Region'
    } else {
      document.title = 'Schedules & Departures · Public Transport Waterloo Region'
    }
  }, [section])

  useEffect(() => {
    const timer = setInterval(() => setToday(serviceDate()), 60_000)
    return () => clearInterval(timer)
  }, [])

  function handleSectionTabKey(event) {
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    const currentIndex = RESIDENT_SECTIONS.indexOf(section)
    let next = section
    if (event.key === 'Home') {
      next = 'schedules'
    } else if (event.key === 'End') {
      next = 'alerts'
    } else if (event.key === 'ArrowRight') {
      next = RESIDENT_SECTIONS[(currentIndex + 1) % RESIDENT_SECTIONS.length]
    } else if (event.key === 'ArrowLeft') {
      next = RESIDENT_SECTIONS[(currentIndex - 1 + RESIDENT_SECTIONS.length) % RESIDENT_SECTIONS.length]
    }
    setSection(next)
    document.getElementById(`section-tab-${next}`)?.focus()
  }

  function handleOpenLiveMap(targetRouteId) {
    setMapRouteId(targetRouteId ? String(targetRouteId) : '')
    setSection('map')
  }

  function handleOpenRouteSchedule(targetRouteId) {
    if (targetRouteId) {
      setRouteId(Number(targetRouteId))
    }
    setView('schedule')
    setSection('schedules')
  }

  function handleOpenAlerts(targetRouteId) {
    setAlertsRouteId(targetRouteId ? String(targetRouteId) : '')
    setSection('alerts')
  }

  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">Skip to transit information</a>
      <header className="site-header resident-header">
        <Link className="brand" to="/" onClick={() => setSection('schedules')}>
          <span className="brand-mark"><BusFront size={24} aria-hidden="true" /></span>
          <span>Public Transport{' '}<span className="brand-region">Waterloo Region</span></span>
        </Link>

        <div className="resident-nav" role="tablist" aria-label="Resident sections" onKeyDown={handleSectionTabKey}>
          <button
            id="section-tab-schedules"
            type="button"
            role="tab"
            className="resident-nav-tab"
            aria-selected={section === 'schedules'}
            aria-controls="section-panel-schedules"
            tabIndex={section === 'schedules' ? 0 : -1}
            onClick={() => setSection('schedules')}
          >
            <CalendarDays size={16} aria-hidden="true" />
            <span>Schedules &amp; Departures</span>
          </button>
          <button
            id="section-tab-journey"
            type="button"
            role="tab"
            className="resident-nav-tab"
            aria-selected={section === 'journey'}
            aria-controls="section-panel-journey"
            tabIndex={section === 'journey' ? 0 : -1}
            onClick={() => setSection('journey')}
          >
            <ArrowRightLeft size={16} aria-hidden="true" />
            <span>Journey Search</span>
          </button>
          <button
            id="section-tab-map"
            type="button"
            role="tab"
            className="resident-nav-tab"
            aria-selected={section === 'map'}
            aria-controls="section-panel-map"
            tabIndex={section === 'map' ? 0 : -1}
            onClick={() => setSection('map')}
          >
            <MapIcon size={16} aria-hidden="true" />
            <span>Live Map</span>
          </button>
          <button
            id="section-tab-alerts"
            type="button"
            role="tab"
            className="resident-nav-tab"
            aria-selected={section === 'alerts'}
            aria-controls="section-panel-alerts"
            tabIndex={section === 'alerts' ? 0 : -1}
            onClick={() => setSection('alerts')}
          >
            <AlertTriangle size={16} aria-hidden="true" />
            <span>Service Alerts</span>
            {activeDisruptionCount > 0 && (
              <span className="nav-alert-badge" aria-hidden="true">
                {activeDisruptionCount}
              </span>
            )}
          </button>
        </div>

        <div className="header-location"><MapPin size={16} aria-hidden="true" /> Waterloo Region, Ontario</div>
      </header>

      <div className="page-heading">
        {section === 'journey' ? (
          <div><h1>Journey Search</h1><p>Find transit routes and walking connections between two locations.</p></div>
        ) : section === 'map' ? (
          <div><h1>Live Map</h1><p>Track active buses across Waterloo Region and view upcoming stop arrivals.</p></div>
        ) : section === 'alerts' ? (
          <div><h1>Service Alerts</h1><p>View active transit disruptions, detours, and affected routes and stops across Waterloo Region.</p></div>
        ) : (
          <div><h1>Schedules &amp; Departures</h1><p>A little planning. A smoother journey.</p></div>
        )}
        <p className="today-label">{formatDate(today, { weekday: 'short', month: 'short' })}<span>All times Eastern · 24-hour clock</span></p>
      </div>

      {section === 'schedules' ? (
        <div
          id="section-panel-schedules"
          role="tabpanel"
          aria-labelledby="section-tab-schedules"
          className="transit-layout"
        >
          <RouteSidebar groups={groups} selected={selectedGroup?.number} onSelect={setRouteId} pending={routes.pending} />
          <main id="main-content" className="main-content" tabIndex={-1}>
            {routes.error ? <ErrorNotice error={routes.error} onRetry={routes.retry} />
              : routes.pending && !routes.data ? <LoadingRows label="Loading routes" />
                : !selectedRoute ? <EmptyState title="No routes available" description="There are no active routes to show right now. Try again in a moment." onRetry={routes.retry} />
                  : <TransitWorkspace route={selectedRoute} group={selectedGroup}
                    onDirectionChange={setRouteId} today={today} date={date} onDateChange={setDate} view={view} onViewChange={setView}
                    onViewLiveMap={() => handleOpenLiveMap(selectedRoute.id)}
                    operationalStatus={selectedRouteOperationalStatus}
                    activeDisruptions={selectedRouteDisruptions}
                    onViewServiceAlerts={() => handleOpenAlerts(selectedRoute.id)} />}
          </main>
        </div>
      ) : section === 'journey' ? (
        <div
          id="section-panel-journey"
          role="tabpanel"
          aria-labelledby="section-tab-journey"
        >
          <main id="main-content" className="main-content journey-main" tabIndex={-1}>
            <div className="view-panel">
              <JourneyView today={today} />
            </div>
          </main>
        </div>
      ) : section === 'map' ? (
        <div
          id="section-panel-map"
          role="tabpanel"
          aria-labelledby="section-tab-map"
        >
          <main id="main-content" className="main-content live-main" tabIndex={-1}>
            <div className="view-panel">
              <LiveMapView
                routes={routes.data ?? []}
                selectedRouteId={mapRouteId}
                onSelectedRouteChange={setMapRouteId}
              />
            </div>
          </main>
        </div>
      ) : (
        <div
          id="section-panel-alerts"
          role="tabpanel"
          aria-labelledby="section-tab-alerts"
        >
          <main id="main-content" className="main-content alerts-main" tabIndex={-1}>
            <div className="view-panel">
              <ServiceAlertsView
                routes={routes.data ?? []}
                serviceStatusResource={serviceStatus}
                selectedRouteId={alertsRouteId}
                onSelectedRouteChange={setAlertsRouteId}
                onSelectRouteSchedule={handleOpenRouteSchedule}
                onSelectRouteLiveMap={handleOpenLiveMap}
              />
            </div>
          </main>
        </div>
      )}

      <footer className="site-footer">
        <span><BusFront size={16} aria-hidden="true" /> Public Transport · Waterloo Region</span>
        <span className="footer-links">
          <span>Published schedules. Estimates shown when available.</span>
          <Link to="/admin">Administrator portal</Link>
        </span>
      </footer>
    </div>
  )
}
