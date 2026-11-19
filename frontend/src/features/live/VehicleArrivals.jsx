import { Clock3, X } from 'lucide-react'
import { formatTime } from '../../lib/transit.js'
import { ErrorNotice, LoadingRows } from '../../components/DataState.jsx'
import { formatDelayMinutes, formatDistance } from './vehicleFormat.js'

export function VehicleArrivals({ vehicle, resource, onClear }) {
  return <section className="primary-vehicle-arrivals" aria-label="Upcoming estimated arrivals">
    <div className="primary-fleet-heading"><h2>Upcoming estimated arrivals</h2><button className="pin-stop-button" type="button" aria-label="Clear selected vehicle" onClick={onClear}><X size={16} aria-hidden="true" /></button></div>
    <p>Bus #{vehicle.vehicle_number} · Route {vehicle.route_number}</p>
    {!vehicle.trip_id ? <p>No active trip is assigned to this vehicle.</p> : resource.error ? <ErrorNotice error={resource.error} onRetry={resource.retry} /> : !resource.data ? <LoadingRows label="Loading estimated arrivals" variant="departures" /> : resource.data.length === 0 ? <p><Clock3 size={15} aria-hidden="true" />No upcoming stop arrival estimates are available for Trip {vehicle.trip_id} right now.</p> : <ol className="primary-vehicle-stops" aria-label={`Estimated arrivals for Trip ${vehicle.trip_id}`}>
      {resource.data.map((arrival) => <li key={`${arrival.trip_id}-${arrival.stop_id}-${arrival.stop_sequence}`}><span><strong>{arrival.stop_name}</strong><small>Scheduled {formatTime(arrival.scheduled_arrival_time)}{formatDistance(arrival.distance_meters) ? ` · ${formatDistance(arrival.distance_meters)} away` : ''}</small></span><span><time dateTime={arrival.estimated_arrival_time}>{formatTime(arrival.estimated_arrival_time)}</time><small>Estimated</small><small>{formatDelayMinutes(arrival.delay_minutes)}</small></span></li>)}
    </ol>}
  </section>
}
