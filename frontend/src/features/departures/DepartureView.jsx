import { useCallback, useMemo, useState } from 'react'
import { Clock3, Info, RefreshCw } from 'lucide-react'
import { getDepartures, getTimetable } from '../../api/transit.js'
import { useResource } from '../../hooks/useResource.js'
import { departureEstimate, directionLabel, formatDate, formatTime, getStops, serviceDate, tripEndpoints } from '../../lib/transit.js'
import { EmptyState, ErrorNotice, LoadingRows } from '../../components/DataState.jsx'

export function DepartureView({ route, today }) {
  const [selectedStop, setSelectedStop] = useState(null)
  const stopId = selectedStop ? String(selectedStop.id) : ''
  const [tripId, setTripId] = useState('')
  const [hours, setHours] = useState(2)
  const loadTimetable = useCallback((signal) => getTimetable(route.id, today, signal), [route.id, today])
  const timetable = useResource(`filter-options-${route.id}-${today}`, loadTimetable)
  const loadDepartures = useCallback((signal) => getDepartures({ routeId: route.id, stopId, tripId, hours }, signal), [route.id, stopId, tripId, hours])
  const departures = useResource(`departures-${route.id}-${stopId}-${tripId}-${hours}`, loadDepartures, 60_000)
  const hasEstimates = departures.data?.some((departure) => departureEstimate(departure))
  const stops = useMemo(() => {
    const choices = new Map(getStops(timetable.data?.trips ?? []).map((stop) => [stop.id, stop]))
    // A departure window can include another service day with different stops.
    for (const departure of departures.data ?? []) {
      if (!choices.has(departure.stop_id)) choices.set(departure.stop_id, { id: departure.stop_id, name: departure.stop_name })
    }
    // Keep the selected label while its refreshed request is pending or empty.
    if (selectedStop && !choices.has(selectedStop.id)) choices.set(selectedStop.id, selectedStop)
    return [...choices.values()].sort((a, b) => a.name.localeCompare(b.name))
  }, [timetable.data, departures.data, selectedStop])
  const tripChoices = useMemo(() => {
    const choices = new Map((timetable.data?.trips ?? []).map((trip) => [trip.id, {
      id: trip.id, label: `${formatTime(tripEndpoints(trip).first?.scheduled_departure_time)} · Trip ${trip.id}`,
    }]))
    // Include overnight trips from earlier service days returned by the board.
    for (const departure of departures.data ?? []) {
      if (!choices.has(departure.trip_id)) choices.set(departure.trip_id, { id: departure.trip_id, label: `Trip ${departure.trip_id} · upcoming` })
    }
    if (tripId && !choices.has(Number(tripId))) choices.set(Number(tripId), { id: Number(tripId), label: `Trip ${tripId}` })
    return [...choices.values()]
  }, [timetable.data, departures.data, tripId])

  return (
    <>
      <div className="view-toolbar departure-toolbar">
        <div className="field stop-filter"><label htmlFor="departure-stop">Stop</label><select id="departure-stop" value={stopId}
          onChange={(event) => setSelectedStop(stops.find((stop) => stop.id === Number(event.target.value)) ?? null)} disabled={!stops.length}>
          <option value="">{timetable.pending && !timetable.data ? 'Loading stops…' : 'All stops on this route'}</option>
          {stops.map((stop) => <option value={stop.id} key={stop.id}>{stop.name}</option>)}
        </select></div>
        <div className="field"><label htmlFor="departure-trip">Trip</label><select id="departure-trip" value={tripId} onChange={(event) => setTripId(event.target.value)} disabled={!tripChoices.length}>
          <option value="">All trips</option>
          {tripChoices.map((trip) => <option value={trip.id} key={trip.id}>{trip.label}</option>)}
        </select></div>
        <div className="field"><label htmlFor="departure-window">Show departures</label><select id="departure-window" value={hours} onChange={(event) => setHours(Number(event.target.value))}>
          <option value={2}>Next 2 hours</option><option value={6}>Next 6 hours</option><option value={24}>Next 24 hours</option>
        </select></div>
      </div>

      {timetable.error && <div className="filter-warning" role="status"><Info size={17} aria-hidden="true" /><p>Full stop and trip lists couldn’t be loaded. You can still filter the departures shown.</p><button className="text-button" type="button" onClick={timetable.retry}>Retry filters</button></div>}

      <div className="results-heading departure-results-heading"><div><h3>{stopId ? stops.find((stop) => stop.id === Number(stopId))?.name ?? 'Stop departures' : 'Next departures'}</h3><p>{formatDate(today, { weekday: 'short', month: 'short' })} · Next {hours} hours{tripId ? ` · Trip ${tripId}` : ''}</p></div>
        <button className="button button-secondary refresh-button" type="button" onClick={departures.retry} disabled={departures.pending}><RefreshCw size={15} aria-hidden="true" />{departures.pending && departures.data ? 'Updating…' : 'Refresh'}</button>
      </div>

      <div className={`estimate-note${hasEstimates ? ' has-estimates' : ''}`}><Clock3 size={17} aria-hidden="true" /><p>{hasEstimates ? 'Estimated times are shown where available. Published times remain listed for reference.' : 'Showing scheduled times. Estimates will appear here when available.'}</p></div>

      {departures.error && <ErrorNotice error={departures.error} onRetry={departures.retry} />}
      {departures.error && departures.data && <p className="stale-notice" role="status">Showing the last successful update. These departures may have changed.</p>}
      {!departures.data && !departures.error ? <LoadingRows label="Loading upcoming departures" />
        : departures.data?.length === 0 ? <EmptyState title="No departures in this window" description="Try a longer time window, another stop, or All trips. You can also check the timetable for later service." />
          : departures.data && <div className="departure-list" aria-label="Upcoming departures" aria-busy={departures.pending}>
            <div className="departure-list-labels" aria-hidden="true"><span>Route &amp; stop</span><span>Scheduled</span><span>Departure</span></div>
            {departures.data.map((departure) => <DepartureRow key={`${departure.trip_id}-${departure.route_stop_id}-${departure.service_date}`} departure={departure} today={today} />)}
          </div>}

      <div className="departure-footer"><p>{departures.updatedAt ? `Updated ${formatTime(departures.updatedAt)} · Refreshes every minute while this page is visible` : 'Departures refresh every minute while this page is visible.'}</p>
        {departures.data?.length === 100 && <p>Showing the first 100 departures. Choose a stop or trip to narrow the results.</p>}
        <p>Filters use today’s timetable and the departures shown. Holiday changes are not included.</p>
      </div>
    </>
  )
}

function DepartureRow({ departure, today }) {
  const estimate = departureEstimate(departure)
  const actual = estimate?.time ?? departure.scheduled_departure_time
  const departureDay = serviceDate(new Date(actual))
  const dayLabel = departureDay !== today ? formatDate(departureDay, { weekday: 'short', month: 'short' }) : null
  let estimateLabel = 'On time'
  if (estimate?.delay > 0) estimateLabel = `${estimate.delay} min later`
  if (estimate?.delay < 0) estimateLabel = `${Math.abs(estimate.delay)} min earlier`

  return (
    <article className="departure-row" aria-label={`Route ${departure.route.number}, ${departure.stop_name}, ${formatTime(actual)}`}>
      <div className="departure-location"><span className="route-number">{departure.route.number}</span><div><h4>{departure.stop_name}</h4><p>{directionLabel(departure.route.direction)} · Trip {departure.trip_id} · Stop {departure.stop_sequence}</p></div></div>
      <div className="scheduled-column"><span className="mobile-column-label">Scheduled</span><time dateTime={departure.scheduled_departure_time}>{formatTime(departure.scheduled_departure_time)}</time></div>
      <div className={`departure-time${estimate ? ' is-estimated' : ''}`}><div><time dateTime={actual}>{formatTime(actual)}</time><span className="time-source">{estimate ? 'Estimated' : 'Scheduled'}</span></div>
        {estimate && <small className={estimate.delay > 0 ? 'delay-label' : ''}>{estimateLabel}</small>}
        {dayLabel && <small>{dayLabel}</small>}
      </div>
    </article>
  )
}
