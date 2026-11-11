import { ChevronDown, ArrowRight } from 'lucide-react'
import { dayOffset, formatTime, tripEndpoints } from '../../lib/transit.js'

function ScheduledTime({ value, date }) {
  const offset = dayOffset(value, date)
  return <span className="time-value"><time dateTime={value}>{formatTime(value)}</time>{offset > 0 && <sup aria-label={`${offset} calendar day${offset === 1 ? '' : 's'} later`}>+{offset}</sup>}</span>
}

export function TripRow({ trip, date }) {
  const { first, last } = tripEndpoints(trip)
  return (
    <details className="trip-row">
      <summary>
        <span className="trip-departs"><span className="visually-hidden">Departs </span><ScheduledTime value={first?.scheduled_departure_time} date={date} /></span>
        <span className="trip-arrives"><span className="visually-hidden">Arrives </span><ScheduledTime value={last?.scheduled_arrival_time} date={date} /></span>
        <span className="trip-destination"><span>{last?.stop_name ?? 'Stop times unavailable'}</span>{' '}<small>Trip {trip.id}</small></span>
        <span className="trip-stop-count">{trip.stop_times.length}<span className="visually-hidden"> stops</span><ChevronDown size={18} aria-hidden="true" /></span>
      </summary>
      <div className="trip-details">
        {first && <p className="trip-journey"><span>{first.stop_name}</span><ArrowRight size={16} aria-hidden="true" /><span>{last.stop_name}</span></p>}
        {trip.stop_times.length ? <div className="stop-table-wrap"><table className="stop-table"><caption className="visually-hidden">Stop times for trip {trip.id}, {date}. All times Eastern.</caption>
          <thead><tr><th scope="col">Stop</th><th scope="col">Arrival</th><th scope="col">Departure</th></tr></thead>
          <tbody>{trip.stop_times.map((stop) => <tr key={stop.route_stop_id}><th scope="row"><span className="stop-sequence">{stop.stop_sequence}</span>{stop.stop_name}</th><td><ScheduledTime value={stop.scheduled_arrival_time} date={date} /></td><td><ScheduledTime value={stop.scheduled_departure_time} date={date} /></td></tr>)}</tbody>
        </table></div> : <p>No stop times have been published for this trip.</p>}
      </div>
    </details>
  )
}
