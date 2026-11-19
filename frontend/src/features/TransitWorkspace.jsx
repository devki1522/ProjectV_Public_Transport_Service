import { AlertTriangle, CalendarDays, Clock3, Radio } from 'lucide-react'
import { directionLabel } from '../lib/transit.js'
import {
  disruptionTypeLabel,
  operationalStatusLabel,
  operationalStatusPillClass,
} from './alerts/disruptionFormat.js'
import { ScheduleView } from './schedule/ScheduleView.jsx'
import { DepartureView } from './departures/DepartureView.jsx'

export function TransitWorkspace({
  route,
  group,
  onDirectionChange,
  today,
  date,
  onDateChange,
  view,
  onViewChange,
  onViewLiveMap,
  operationalStatus = null,
  activeDisruptions = [],
  onViewServiceAlerts = null,
}) {
  function handleTabKey(event) {
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    const next = event.key === 'Home' ? 'schedule' : event.key === 'End' ? 'departures' : view === 'schedule' ? 'departures' : 'schedule'
    onViewChange(next)
    document.getElementById(`tab-${next}`)?.focus()
  }

  const hasRouteDisruption =
    activeDisruptions.length > 0 || (Boolean(operationalStatus) && operationalStatus !== 'normal')

  return (
    <>
      <div className="route-heading">
        <div className="selected-route-name">
          <span className="route-number route-number-large">{route.number}</span>
          <div>
            <div className="route-title-row">
              <h2>{route.name}</h2>
              {operationalStatus && (
                <span className={`status-pill ${operationalStatusPillClass(operationalStatus)}`}>
                  {operationalStatusLabel(operationalStatus)}
                </span>
              )}
            </div>
            <p>Route {route.number} · {directionLabel(route.direction)}</p>
          </div>
        </div>
        <div className="route-heading-controls">
          <div className="direction-field"><label htmlFor="direction-select">Travel direction</label><select id="direction-select" value={route.id} onChange={(event) => onDirectionChange(Number(event.target.value))}>
            {group.directions.map((direction) => <option key={direction.id} value={direction.id}>{directionLabel(direction.direction)}</option>)}
          </select></div>
          {onViewLiveMap && (
            <button type="button" className="button button-secondary live-buses-button" onClick={onViewLiveMap}>
              <Radio size={15} aria-hidden="true" />
              <span>Live buses</span>
            </button>
          )}
        </div>
      </div>

      {hasRouteDisruption && (
        <div className="route-disruption-banner" role="status" aria-label="Route service disruption notice">
          <AlertTriangle size={18} aria-hidden="true" />
          <div className="route-disruption-copy">
            <strong>
              Route {route.number} status: {operationalStatusLabel(operationalStatus || 'disrupted')}
            </strong>
            {activeDisruptions.length > 0 && (
              <ul className="route-disruption-list">
                {activeDisruptions.map((item) => (
                  <li key={item.id}>
                    <span className="route-disruption-type">{disruptionTypeLabel(item.type)}:</span>{' '}
                    <span>{item.title}</span>
                    {item.affected_area ? ` (${item.affected_area})` : ''}
                  </li>
                ))}
              </ul>
            )}
          </div>
          {onViewServiceAlerts && (
            <button
              type="button"
              className="button button-secondary button-compact"
              onClick={onViewServiceAlerts}
            >
              View service alerts
            </button>
          )}
        </div>
      )}

      <div className="view-tabs" role="tablist" aria-label="Transit information" onKeyDown={handleTabKey}>
        <button id="tab-schedule" type="button" role="tab" aria-selected={view === 'schedule'} aria-controls="panel-schedule" tabIndex={view === 'schedule' ? 0 : -1} onClick={() => onViewChange('schedule')}><CalendarDays size={18} aria-hidden="true" />Timetable</button>
        <button id="tab-departures" type="button" role="tab" aria-selected={view === 'departures'} aria-controls="panel-departures" tabIndex={view === 'departures' ? 0 : -1} onClick={() => onViewChange('departures')}><Clock3 size={18} aria-hidden="true" />Upcoming Departures</button>
      </div>

      <div id="panel-schedule" role="tabpanel" aria-labelledby="tab-schedule" tabIndex={0} className="view-panel" hidden={view !== 'schedule'}>
        {view === 'schedule' && <ScheduleView key={route.id} route={route} date={date} today={today} onDateChange={onDateChange} />}
      </div>
      <div id="panel-departures" role="tabpanel" aria-labelledby="tab-departures" tabIndex={0} className="view-panel" hidden={view !== 'departures'}>
        {view === 'departures' && <DepartureView key={`${route.id}-${today}`} route={route} today={today} />}
      </div>
    </>
  )
}
