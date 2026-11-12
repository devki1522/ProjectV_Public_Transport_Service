import { CalendarDays, Clock3 } from 'lucide-react'
import { directionLabel } from '../lib/transit.js'
import { ScheduleView } from './schedule/ScheduleView.jsx'
import { DepartureView } from './departures/DepartureView.jsx'

export function TransitWorkspace({ route, group, onDirectionChange, today, date, onDateChange, view, onViewChange }) {
  function handleTabKey(event) {
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    const next = event.key === 'Home' ? 'schedule' : event.key === 'End' ? 'departures' : view === 'schedule' ? 'departures' : 'schedule'
    onViewChange(next)
    document.getElementById(`tab-${next}`)?.focus()
  }

  return (
    <>
      <div className="route-heading">
        <div className="selected-route-name"><span className="route-number route-number-large">{route.number}</span><div><h2>{route.name}</h2><p>Route {route.number} · {directionLabel(route.direction)}</p></div></div>
        <div className="direction-field"><label htmlFor="direction-select">Travel direction</label><select id="direction-select" value={route.id} onChange={(event) => onDirectionChange(Number(event.target.value))}>
          {group.directions.map((direction) => <option key={direction.id} value={direction.id}>{directionLabel(direction.direction)}</option>)}
        </select></div>
      </div>

      <div className="view-tabs" role="tablist" aria-label="Transit information" onKeyDown={handleTabKey}>
        <button id="tab-schedule" type="button" role="tab" aria-selected={view === 'schedule'} aria-controls="panel-schedule" tabIndex={view === 'schedule' ? 0 : -1} onClick={() => onViewChange('schedule')}><CalendarDays size={18} aria-hidden="true" />Timetable</button>
        <button id="tab-departures" type="button" role="tab" aria-selected={view === 'departures'} aria-controls="panel-departures" tabIndex={view === 'departures' ? 0 : -1} onClick={() => onViewChange('departures')}><Clock3 size={18} aria-hidden="true" />Upcoming departures</button>
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
