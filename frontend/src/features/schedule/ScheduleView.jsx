import { useCallback, useState } from 'react'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { getTimetable } from '../../api/transit.js'
import { useResource } from '../../hooks/useResource.js'
import { formatDate, inPeriod, isServiceDate } from '../../lib/transit.js'
import { EmptyState, ErrorNotice, LoadingRows } from '../../components/DataState.jsx'
import { TripRow } from './TripRow.jsx'

const PAGE_SIZE = 8

export function ScheduleView({ route, date, today, onDateChange }) {
  const load = useCallback((signal) => getTimetable(route.id, date, signal), [route.id, date])
  const validDate = isServiceDate(date)
  const timetable = useResource(validDate ? `schedule-${route.id}-${date}` : null, load)
  const [period, setPeriod] = useState('all')
  const [page, setPage] = useState(0)
  const trips = (timetable.data?.trips ?? []).filter((trip) => inPeriod(trip, period))
  const pages = Math.ceil(trips.length / PAGE_SIZE)
  const currentPage = Math.min(page, Math.max(0, pages - 1))
  const start = currentPage * PAGE_SIZE

  function changeDate(value) { onDateChange(value); setPage(0) }

  return (
    <>
      <div className="view-toolbar schedule-toolbar">
        <div className="field"><label htmlFor="service-date">Travel date</label><div className="date-controls">
          <input id="service-date" type="date" min="0001-01-01" max="9999-12-30" value={date} onChange={(event) => changeDate(event.target.value)} required />
          <button type="button" className="button button-secondary" onClick={() => changeDate(today)} disabled={date === today}>Today</button>
        </div></div>
        <div className="field period-filter"><label htmlFor="time-period">Time of day</label><select id="time-period" value={period} onChange={(event) => { setPeriod(event.target.value); setPage(0) }}>
          <option value="all">All day</option><option value="morning">Morning · 06–12</option><option value="afternoon">Afternoon · 12–18</option><option value="evening">Evening · 18–24</option><option value="night">Overnight · 00–06</option>
        </select></div>
      </div>

      <div className="results-heading"><div><h3>{validDate ? formatDate(date) : 'Choose a valid travel date'}</h3><p>Select a trip to see arrival and departure times at every stop.</p></div>
        {timetable.data && <span className="result-count">{trips.length} {trips.length === 1 ? 'trip' : 'trips'}</span>}
      </div>

      {!validDate ? <EmptyState title="When are you travelling?" description="Choose a valid travel date to see this route’s scheduled trips." />
        : timetable.error ? <ErrorNotice error={timetable.error} onRetry={timetable.retry} />
          : !timetable.data ? <LoadingRows label="Loading timetable" />
            : !trips.length ? <EmptyState title="No trips for this selection" description="Try another date or choose All day to see more scheduled service." />
              : <>
                <div className="trip-list" key={`${date}-${period}-${currentPage}`}>
                  <div className="trip-list-labels" aria-hidden="true"><span>Departs</span><span>Arrives</span><span>Destination</span><span>Stops</span></div>
                  {trips.slice(start, start + PAGE_SIZE).map((trip) => <TripRow key={trip.id} trip={trip} date={date} />)}
                </div>
                <nav className="pagination" aria-label="Timetable pages"><span>Showing {start + 1}–{Math.min(start + PAGE_SIZE, trips.length)} of {trips.length} trips</span>
                  <div><button type="button" className="icon-button" aria-label="Previous trips" disabled={currentPage === 0} onClick={() => setPage(currentPage - 1)}><ChevronLeft size={18} aria-hidden="true" /></button><span>{currentPage + 1} / {pages}</span><button type="button" className="icon-button" aria-label="Next trips" disabled={currentPage + 1 >= pages} onClick={() => setPage(currentPage + 1)}><ChevronRight size={18} aria-hidden="true" /></button></div>
                </nav>
              </>}
      <div className="schedule-note"><p>Schedules repeat by weekday and weekend. Holiday changes are not included.</p><p>Times after midnight belong to the travel date shown. A +1 label means the next calendar day.</p></div>
    </>
  )
}
