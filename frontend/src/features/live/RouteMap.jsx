import { routeColor, routePalette } from '../../lib/routeColors.js'
import { useCallback, useMemo } from 'react'
import { Clock3, Map as MapIcon } from 'lucide-react'
import { getRouteShape } from '../../api/transit.js'
import { useResource } from '../../hooks/useResource.js'
import { formatTime } from '../../lib/transit.js'
import { ErrorNotice } from '../../components/DataState.jsx'
import { LeafletMap } from './LeafletMap.jsx'

const EMPTY_LEGS = []

const validTime = (value) => Boolean(value) && Number.isFinite(Date.parse(value))

function PreviewTime({ label, estimated, scheduled }) {
  const isEstimated = validTime(estimated)
  const value = isEstimated ? estimated : scheduled
  return <div className="preview-time">
    <span>{label} · {isEstimated ? 'Estimated' : 'Scheduled'}</span>
    <strong className={isEstimated ? 'is-estimated' : ''}>
      {validTime(value) ? <time dateTime={value}>{formatTime(value)}</time> : 'Unavailable'}
    </strong>
    {isEstimated && validTime(scheduled) && <small>Scheduled {formatTime(scheduled)}</small>}
  </div>
}

export function RouteMap({ routeIds = [], itinerary = null }) {
  const legs = itinerary?.legs ?? EMPTY_LEGS
  const idsKey = [...new Set((itinerary ? legs.map((leg) => leg.route?.id) : routeIds).filter(Boolean))].join(',')
  const load = useCallback((signal) => Promise.all(idsKey.split(',').map((id) => getRouteShape(id, signal))), [idsKey])
  const shapes = useResource(idsKey ? `map-routes-${idsKey}` : null, load)
  const routeColors = useMemo(() => routePalette(legs.filter((leg) => leg.route).map((leg) => leg.route)), [legs])
  const mapShape = useMemo(() => shapes.data ? {
    route_id: idsKey,
    paths: shapes.data.map((shape) => ({
      coordinates: shape.coordinates,
      color: routeColors.get(String(shape.route_number)) ?? routeColor(shape.route_number),
      label: `Route ${shape.route_number}`,
    })),
  } : null, [shapes.data, idsKey, routeColors])

  return (
    <section className="route-preview" aria-label={itinerary ? 'Journey map preview' : 'Route map'}>
      <div className="route-preview-heading">
        <div><MapIcon size={18} aria-hidden="true" /><h2>{itinerary ? 'Your journey at a glance' : 'Route preview'}</h2></div>
        {itinerary && <span><Clock3 size={14} aria-hidden="true" />{itinerary.duration_minutes} min · {itinerary.transfers} {itinerary.transfers === 1 ? 'transfer' : 'transfers'}</span>}
      </div>
      {itinerary && <div className="preview-summary">
        <PreviewTime label="Depart" estimated={itinerary.estimated_departure_time} scheduled={itinerary.scheduled_departure_time} />
        <span className="preview-summary-arrow" aria-hidden="true">→</span>
        <PreviewTime label="Arrive" estimated={itinerary.estimated_arrival_time} scheduled={itinerary.scheduled_arrival_time} />
      </div>}
      <div className="route-preview-map">
        <LeafletMap routeShape={mapShape} />
        {shapes.pending && <p className="preview-map-status" role="status">Loading route lines…</p>}
      </div>
      {shapes.error && <ErrorNotice error={shapes.error} onRetry={shapes.retry} />}
      {shapes.data?.some((shape) => shape.coordinates.length < 2) && <p className="preview-map-note" role="status">A route line is unavailable. Use the stop-by-stop itinerary below or refresh the preview.</p>}
      {itinerary && <ol className="preview-leg-list" aria-label="Journey map legend and times">
        {legs.map((leg, index) => <li key={`${leg.trip_id ?? 'walk'}-${index}`} style={{ '--route-color': routeColors.get(String(leg.route?.number)) ?? 'var(--muted)' }}>
          <span className={`preview-route-badge${leg.mode === 'walk' ? ' is-walk' : ''}`}>{leg.mode === 'walk' ? 'Walk' : `Route ${leg.route?.number}`}</span>
          <div className="preview-leg-location"><strong>{leg.from_stop_name} → {leg.to_stop_name}</strong><small>{leg.duration_minutes} min{leg.mode === 'walk' ? ' walk' : ` · ${leg.route?.name ?? ''}`}</small></div>
          <div className="preview-leg-times">
            <PreviewTime label="Depart" estimated={leg.estimated_departure_time} scheduled={leg.scheduled_departure_time} />
            <PreviewTime label="Arrive" estimated={leg.estimated_arrival_time} scheduled={leg.scheduled_arrival_time} />
          </div>
        </li>)}
      </ol>}
      <p className="preview-map-note">{itinerary ? 'Colours match each transit route. Full route lines are shown; board and leave at the stops listed above. Walking connections are listed, not street-routed on the map. Times are Eastern.' : 'Full route line for the selected travel direction.'}</p>
    </section>
  )
}
