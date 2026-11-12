import { useEffect, useMemo, useState } from 'react'
import { BusFront, MapPin } from 'lucide-react'
import { getRoutes } from '../../api/transit.js'
import { useResource } from '../../hooks/useResource.js'
import { formatDate, groupRoutes, serviceDate } from '../../lib/transit.js'
import { Link } from '../../lib/router.jsx'
import { RouteSidebar } from '../../components/RouteSidebar.jsx'
import { EmptyState, ErrorNotice, LoadingRows } from '../../components/DataState.jsx'
import { TransitWorkspace } from '../TransitWorkspace.jsx'

export function ResidentPage() {
  const routes = useResource('routes', getRoutes)
  const groups = useMemo(() => groupRoutes(routes.data ?? []), [routes.data])
  const [routeId, setRouteId] = useState(null)
  const [today, setToday] = useState(serviceDate)
  const [date, setDate] = useState(serviceDate)
  const [view, setView] = useState('schedule')
  const selectedRoute = routes.data?.find((route) => route.id === routeId) ?? groups[0]?.directions[0]
  const selectedGroup = groups.find((group) => group.number === selectedRoute?.number)

  useEffect(() => {
    document.title = 'Schedules & departures · Public Transport Waterloo Region'
  }, [])

  useEffect(() => {
    const timer = setInterval(() => setToday(serviceDate()), 60_000)
    return () => clearInterval(timer)
  }, [])

  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">Skip to transit information</a>
      <header className="site-header">
        <Link className="brand" to="/">
          <span className="brand-mark"><BusFront size={24} aria-hidden="true" /></span>
          <span>Public Transport{' '}<span className="brand-region">Waterloo Region</span></span>
        </Link>
        <div className="header-location"><MapPin size={16} aria-hidden="true" /> Waterloo Region, Ontario</div>
      </header>

      <div className="page-heading">
        <div><h1>Schedules &amp; departures</h1><p>A little planning. A smoother journey.</p></div>
        <p className="today-label">{formatDate(today, { weekday: 'short', month: 'short' })}<span>All times Eastern · 24-hour clock</span></p>
      </div>

      <div className="transit-layout">
        <RouteSidebar groups={groups} selected={selectedGroup?.number} onSelect={setRouteId} pending={routes.pending} />
        <main id="main-content" className="main-content" tabIndex={-1}>
          {routes.error ? <ErrorNotice error={routes.error} onRetry={routes.retry} />
            : routes.pending && !routes.data ? <LoadingRows label="Loading routes" />
              : !selectedRoute ? <EmptyState title="No routes available" description="There are no active routes to show right now. Try again in a moment." onRetry={routes.retry} />
                : <TransitWorkspace route={selectedRoute} group={selectedGroup}
                  onDirectionChange={setRouteId} today={today} date={date} onDateChange={setDate} view={view} onViewChange={setView} />}
        </main>
      </div>

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
