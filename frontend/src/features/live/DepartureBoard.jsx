import { arrivalCountdown } from './arrivalCountdown.js'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Bookmark, BookmarkCheck, ChevronRight, Radio, Search } from 'lucide-react'
import { getRouteShape, getStopArrivals, getStopsCatalog } from '../../api/transit.js'
import { useResource } from '../../hooks/useResource.js'
import { useStoredPreference } from '../../hooks/usePreferences.js'
import { directionLabel, formatTime } from '../../lib/transit.js'
import { RouteBadge } from '../../components/RouteBadge.jsx'
import { EmptyState, ErrorNotice, LoadingRows } from '../../components/DataState.jsx'
import { FeedFreshness } from '../../components/FeedFreshness.jsx'

export function DepartureBoard({ live, onPreviewRoute, previewRouteId }) {
  const [search, setSearch] = useState('')
  const [query, setQuery] = useState('')
  const [chosenStop, setChosenStop] = useState(null)
  const [now, setNow] = useState(() => Date.now())
  const [stored, setPinned] = useStoredPreference('transit-saved-stops', [])
  const pinned = Array.isArray(stored) ? stored.filter((stop) => Number.isInteger(stop?.id) && typeof stop.name === 'string').slice(0, 20) : []
  useEffect(() => { const timer = setTimeout(() => setQuery(search.trim()), 250); return () => clearTimeout(timer) }, [search])
  useEffect(() => {
    if (!live) return
    const timer = setInterval(() => setNow(Date.now()), 15_000)
    return () => clearInterval(timer)
  }, [live])
  const loadStops = useCallback((signal) => getStopsCatalog(query || 'Central Station', signal), [query])
  const stops = useResource(`primary-stop-search-${query || 'central'}`, loadStops)
  const stop = chosenStop ?? pinned[0] ?? (!query ? stops.data?.[0] : null)
  const stopId = Number(stop?.id) || 0
  const loadArrivals = useCallback((signal) => getStopArrivals(stopId, null, signal), [stopId])
  const arrivals = useResource(stopId ? `primary-arrivals-${stopId}` : null, loadArrivals, live ? 15_000 : 0)
  const routeKey = [...new Set((arrivals.data ?? []).map((arrival) => arrival.route_id))].sort((a, b) => a - b).join(',')
  const loadHeadsigns = useCallback(async (signal) => {
    const results = await Promise.allSettled(routeKey.split(',').map((id) => getRouteShape(id, signal)))
    return results.filter((result) => result.status === 'fulfilled').map((result) => result.value)
  }, [routeKey])
  const headsigns = useResource(routeKey ? `primary-headsigns-${routeKey}` : null, loadHeadsigns)
  const destinations = useMemo(() => new Map((headsigns.data ?? []).map((shape) => [shape.route_id, shape.headsign])), [headsigns.data])
  const isPinned = pinned.some((entry) => entry.id === stopId)
  const sortedArrivals = useMemo(() => [...(arrivals.data ?? [])].sort((a, b) => Date.parse(a.estimated_arrival_time ?? a.scheduled_arrival_time) - Date.parse(b.estimated_arrival_time ?? b.scheduled_arrival_time)), [arrivals.data])

  function selectStop(next) { setChosenStop(next); setSearch(''); setQuery('') }
  function togglePin() {
    if (!stop) return
    setPinned(isPinned ? pinned.filter((entry) => entry.id !== stopId) : [...pinned, { id: stop.id, name: stop.name }])
  }

  return <section className="commuter-board" aria-label="Live departures">
    <div className="primary-search">
      <label htmlFor="primary-stop-search"><Search size={19} aria-hidden="true" /><span className="visually-hidden">Search stops and stations</span>
        <input id="primary-stop-search" type="search" placeholder="Find a stop or station" autoComplete="off" value={search} onChange={(event) => setSearch(event.target.value)} />
      </label>
      {stops.error && <ErrorNotice error={stops.error} onRetry={stops.retry} />}
      {query && <div className="primary-search-results">
        {stops.pending ? <LoadingRows label="Searching stops" variant="routes" /> : <ul aria-label="Matching stops">
          {stops.data?.slice(0, 6).map((entry) => <li key={entry.id}><button type="button" onClick={() => selectStop(entry)}><span>{entry.name}<small className="stop-id">Stop {entry.id}</small></span><ChevronRight size={16} aria-hidden="true" /></button></li>)}
          {stops.data?.length === 0 && <li className="primary-no-stops">No matching stops. Try a station name.</li>}
        </ul>}
      </div>}
    </div>
    <div className="primary-pins"><span><Bookmark size={13} aria-hidden="true" />Pinned stops</span>
      <div className="primary-pin-list" role="group" aria-label="Pinned stops">
        {pinned.map((entry) => <button key={entry.id} type="button" aria-pressed={entry.id === stopId} onClick={() => selectStop(entry)}>{entry.name}</button>)}
        {!pinned.length && <p>Pin a stop to keep it here.</p>}
      </div>
    </div>
    <div className="primary-station-heading">
      <div><span className="station-source">Departures from</span><h2>{stop?.name ?? 'Choose a stop'}</h2>{stop && <span className="stop-id">Stop {stopId}</span>}</div>
      {stop && <button className="pin-stop-button" type="button" aria-label={`${isPinned ? 'Unpin' : 'Pin'} ${stop.name}`} aria-pressed={isPinned} disabled={!isPinned && pinned.length >= 20} onClick={togglePin}>{isPinned ? <BookmarkCheck size={19} aria-hidden="true" /> : <Bookmark size={19} aria-hidden="true" />}</button>}
    </div>
    <div className="primary-board-caption"><span>Destination</span><span>Arrival</span></div>
    <div className="visually-hidden" role="status">{arrivals.data ? `${arrivals.data.length} arrivals at ${stop?.name}.` : ''}</div>
    {!stop && !stops.error && <LoadingRows label="Loading station" variant="departures" />}
    {stop && !arrivals.data && !arrivals.error && <LoadingRows label="Loading stop departures" variant="departures" />}
    {arrivals.error && <ErrorNotice error={arrivals.error} onRetry={arrivals.retry} />}
    <FeedFreshness updatedAt={arrivals.updatedAt} maxAge={45_000} paused={!live} />
    {arrivals.error && arrivals.data && <p className="primary-feed-note">Last known arrivals. Refresh failed; times may have changed.</p>}
    {arrivals.data?.length === 0 && <EmptyState title="No approaching vehicles" description="Try another stop or check the timetable for later service." onRetry={arrivals.retry} actionLabel="Refresh arrivals" />}
    <ul className="primary-departures" aria-label={stop ? `Departures from ${stop.name}` : 'Stop departures'}>{sortedArrivals.map((arrival) => {
      const estimated = Boolean(arrival.estimated_arrival_time) && Number.isFinite(Date.parse(arrival.estimated_arrival_time))
      const time = estimated ? arrival.estimated_arrival_time : arrival.scheduled_arrival_time
      const eta = arrivalCountdown(time, now)
      const destination = destinations.get(arrival.route_id) || arrival.route_name
      return <li key={`${arrival.vehicle_id}-${arrival.trip_id}-${arrival.stop_sequence}`}><button className="primary-departure" type="button" aria-pressed={Number(previewRouteId) === arrival.route_id} onClick={() => onPreviewRoute(String(arrival.route_id))} aria-label={`Route ${arrival.route_number} to ${destination}, ${estimated ? 'estimated' : 'scheduled'} ${formatTime(time)}. Show route on map.`}>
        <RouteBadge number={arrival.route_number} />
        <span className="primary-destination"><strong>{destination}</strong><span>{arrival.route_name} <span aria-hidden="true">·</span> {directionLabel(arrival.direction)}</span><small>{arrival.delay_minutes > 0 && <span className="departure-delay" title={`${arrival.delay_minutes} minutes later than scheduled`}>Delayed / </span>}Scheduled {formatTime(arrival.scheduled_arrival_time)}{arrival.is_simulated ? ' · Simulated' : ''}</small></span>
        <span className="primary-eta"><span className={`eta-countdown${eta.value.includes(':') ? ' eta-clock' : ''}`}>{eta.value}<small>{eta.unit}</small></span><span className={estimated ? 'eta-source is-live' : 'eta-source'}>{estimated && <Radio size={11} aria-hidden="true" />}{estimated ? 'Estimated' : 'Scheduled'}</span></span>
      </button></li>
    })}</ul>
    <div className="primary-board-footer"><span><Radio size={13} aria-hidden="true" />{live ? 'Updates every 15 seconds' : 'Updates paused'}</span><span>Times Eastern</span></div>
  </section>
}
