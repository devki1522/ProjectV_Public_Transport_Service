import { useCallback, useEffect, useState } from 'react'
import { Bookmark, BookmarkCheck, MapPin, RefreshCw, Search, X } from 'lucide-react'
import { getStopArrivals, getStopsCatalog } from '../../api/transit.js'
import { useResource } from '../../hooks/useResource.js'
import { useStoredPreference } from '../../hooks/usePreferences.js'
import { formatTime } from '../../lib/transit.js'
import { RouteBadge } from '../../components/RouteBadge.jsx'
import { FeedFreshness } from '../../components/FeedFreshness.jsx'
import { EmptyState, ErrorNotice, LoadingRows } from '../../components/DataState.jsx'

export function SavedStops() {
  const [open, setOpen] = useState(false)
  const [stored, setSaved] = useStoredPreference('transit-saved-stops', [])
  const saved = Array.isArray(stored) ? stored.filter((stop) => Number.isInteger(stop?.id) && typeof stop.name === 'string').slice(0, 20) : []
  return <div className="saved-stops">
    <button className="button button-secondary" type="button" aria-expanded={open} aria-controls="saved-stops-panel" onClick={() => setOpen(!open)}>
      <Bookmark size={16} aria-hidden="true" />Saved stops{saved.length > 0 && <span className="saved-count">{saved.length}</span>}
    </button>
    {open && <section id="saved-stops-panel" className="saved-stops-panel" aria-label="Saved stops">
      <div className="saved-stops-heading"><div><h2>Stops you use every day</h2><p>Save up to 20 stops on this device. No account needed.</p></div>
        <button className="icon-button" aria-label="Close saved stops" onClick={() => setOpen(false)}><X size={18} aria-hidden="true" /></button>
      </div>
      <StopPicker saved={saved} onSave={(stop) => setSaved([...saved, { id: stop.id, name: stop.name }])} onRemove={(id) => setSaved(saved.filter((stop) => stop.id !== id))} />
    </section>}
  </div>
}

function StopPicker({ saved, onSave, onRemove }) {
  const [search, setSearch] = useState('')
  const [query, setQuery] = useState('')
  const [selectedId, setSelectedId] = useState(null)
  useEffect(() => { const timer = setTimeout(() => setQuery(search.trim()), 250); return () => clearTimeout(timer) }, [search])
  const load = useCallback((signal) => getStopsCatalog(query, signal), [query])
  const stops = useResource(query ? `saved-stop-search-${query}` : null, load)
  const selected = saved.find((stop) => stop.id === selectedId)
  return <>
    <label className="saved-search"><Search size={18} aria-hidden="true" /><span className="visually-hidden">Search stops to save</span>
      <input type="search" placeholder="Stop or station name" value={search} onChange={(event) => setSearch(event.target.value)} />
    </label>
    <div className="visually-hidden" role="status">{stops.data ? `${stops.data.length} stops found` : ''}</div>
    {stops.error && <ErrorNotice error={stops.error} onRetry={stops.retry} />}
    {query && stops.pending && <LoadingRows label="Searching stops" variant="vehicles" />}
    {query && stops.data && <ul className="stop-search-results" aria-label="Stops to save">
      {stops.data.map((stop) => {
        const pinned = saved.some((entry) => entry.id === stop.id)
        return <li key={stop.id}><MapPin size={16} aria-hidden="true" /><span><strong>{stop.name}</strong><small>Stop {stop.id}</small></span>
          <button type="button" className="button button-secondary" disabled={!pinned && saved.length >= 20} aria-label={`${pinned ? 'Unsave' : 'Save'} ${stop.name}`} aria-pressed={pinned} onClick={() => pinned ? onRemove(stop.id) : onSave(stop)}>
            {pinned ? <BookmarkCheck size={16} aria-hidden="true" /> : <Bookmark size={16} aria-hidden="true" />}{pinned ? 'Saved' : 'Save'}
          </button></li>
      })}
      {stops.data.length === 0 && <li>No stops found. Try a station name or a shorter search.</li>}
    </ul>}
    {saved.length ? <ul className="saved-stop-list" aria-label="Pinned stops">{saved.map((stop) => <li key={stop.id}>
      <button type="button" className="saved-stop-choice" aria-pressed={selectedId === stop.id} onClick={() => setSelectedId(stop.id)}><BookmarkCheck size={16} aria-hidden="true" />{stop.name}<small>View arrivals</small></button>
      <button className="icon-button" type="button" aria-label={`Remove ${stop.name} from saved stops`} onClick={() => onRemove(stop.id)}><X size={16} aria-hidden="true" /></button>
    </li>)}</ul> : <p className="saved-stops-empty">Search for a stop above, then select Save for a quicker departure check next time.</p>}
    {selected && <SavedArrivals key={selected.id} stop={selected} />}
  </>
}

function SavedArrivals({ stop }) {
  const [live, setLive] = useState(true)
  const load = useCallback((signal) => getStopArrivals(stop.id, null, signal), [stop.id])
  const arrivals = useResource(`saved-arrivals-${stop.id}`, load, live ? 30_000 : 0)
  return <section className="saved-arrivals" aria-label={`Arrivals at ${stop.name}`}>
    <div className="saved-stops-heading"><h3>{stop.name}</h3><div className="saved-arrival-actions">
      <button className="button button-secondary" aria-pressed={live} onClick={() => setLive(!live)}>{live ? 'Pause updates' : 'Resume updates'}</button>
      <button className="icon-button" aria-label="Refresh saved stop arrivals" onClick={arrivals.retry}><RefreshCw size={16} aria-hidden="true" /></button>
    </div></div>
    <p className="feed-timestamp" role="status">{arrivals.updatedAt ? `Updated ${formatTime(arrivals.updatedAt)}${arrivals.error ? '. Last known arrivals; refresh failed.' : ''}` : 'Checking arrivals…'}</p>
    {arrivals.error && <ErrorNotice error={arrivals.error} onRetry={arrivals.retry} />}
    <FeedFreshness updatedAt={arrivals.updatedAt} paused={!live} />
    {!arrivals.data && !arrivals.error && <LoadingRows label="Loading saved stop arrivals" variant="departures" />}
    {arrivals.data?.length === 0 && <EmptyState title="No upcoming arrivals" description="Check the route timetable for later service, or refresh in a moment." onRetry={arrivals.retry} />}
    <ul className="saved-arrival-list" aria-label="Saved stop arrival estimates">{arrivals.data?.map((arrival) => <li key={`${arrival.vehicle_id}-${arrival.trip_id}-${arrival.stop_sequence}`}>
      <RouteBadge number={arrival.route_number} /><span><strong>{arrival.route_name}</strong><small>Trip {arrival.trip_id}{arrival.is_simulated ? ' · Simulated' : ''}</small><small>Scheduled {formatTime(arrival.scheduled_arrival_time)}</small></span>
      <span><strong><time dateTime={arrival.estimated_arrival_time ?? arrival.scheduled_arrival_time}>{formatTime(arrival.estimated_arrival_time ?? arrival.scheduled_arrival_time)}</time></strong><small>{arrival.estimated_arrival_time ? 'Estimated' : 'Scheduled'}</small></span>
    </li>)}</ul>
  </section>
}
