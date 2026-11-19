import { RouteBadge } from '../../components/RouteBadge.jsx'
import { routePalette } from '../../lib/routeColors.js'
import { useCallback, useMemo, useState } from 'react'
import {
  AlertCircle,
  ArrowLeftRight,
  ArrowRight,
  Check,
  ChevronDown,
  Clock3,
  Footprints,
  Info,
} from 'lucide-react'
import { getJourneys, getStopsCatalog } from '../../api/transit.js'
import { RouteMap } from '../live/RouteMap.jsx'
import { useResource } from '../../hooks/useResource.js'
import { directionLabel, formatDate, formatTime, serviceDate } from '../../lib/transit.js'
import { EmptyState, ErrorNotice, LoadingRows } from '../../components/DataState.jsx'

function hasValidTime(value) {
  return Boolean(value) && !Number.isNaN(new Date(value).getTime())
}

function formatDisruptionType(type) {
  return type ? type.replaceAll('_', ' ') : 'disruption'
}

function resolveTimes(entry) {
  const hasEstimatedDeparture = hasValidTime(entry.estimated_departure_time)
  const hasEstimatedArrival = hasValidTime(entry.estimated_arrival_time)
  return {
    hasEstimatedDeparture,
    hasEstimatedArrival,
    isEstimated: hasEstimatedDeparture || hasEstimatedArrival,
    departure: hasEstimatedDeparture
      ? entry.estimated_departure_time
      : entry.scheduled_departure_time,
    arrival: hasEstimatedArrival
      ? entry.estimated_arrival_time
      : entry.scheduled_arrival_time,
  }
}

export function JourneyView({ today }) {
  const [origin, setOrigin] = useState('')
  const [destination, setDestination] = useState('')
  const [hours, setHours] = useState(6)
  const [maxTransfers, setMaxTransfers] = useState(3)
  const [validationError, setValidationError] = useState('')
  const [submitted, setSubmitted] = useState(null)
  const [selectedIndex, setSelectedIndex] = useState(0)

  const loadStops = useCallback((signal) => getStopsCatalog('', signal), [])
  const stopsCatalog = useResource('stops-catalog', loadStops)
  const stopNames = useMemo(
    () => [...new Set((stopsCatalog.data ?? []).map((stop) => stop.name))],
    [stopsCatalog.data],
  )

  const loadJourneys = useCallback(
    (signal) => getJourneys(submitted, signal),
    [submitted],
  )
  const journeyKey = submitted
    ? `journeys-${submitted.origin}-${submitted.destination}-${submitted.hours}-${submitted.maxTransfers}`
    : null
  const journeys = useResource(journeyKey, loadJourneys)

  function handleSwap() {
    setOrigin(destination)
    setDestination(origin)
    setValidationError('')
  }

  function handleSubmit(event) {
    event.preventDefault()
    const originTrimmed = origin.trim()
    const destinationTrimmed = destination.trim()

    if (!originTrimmed || !destinationTrimmed) {
      setValidationError('Enter both an origin and a destination to search.')
      document.getElementById(!originTrimmed ? 'journey-origin' : 'journey-destination')?.focus()
      return
    }
    if (originTrimmed.toLowerCase() === destinationTrimmed.toLowerCase()) {
      setValidationError('Origin and destination must be different locations.')
      return
    }

    setValidationError('')
    setSelectedIndex(0)
    if (
      submitted &&
      submitted.origin === originTrimmed &&
      submitted.destination === destinationTrimmed &&
      submitted.hours === hours &&
      submitted.maxTransfers === maxTransfers
    ) {
      journeys.retry()
      return
    }
    setSubmitted({
      origin: originTrimmed,
      destination: destinationTrimmed,
      hours,
      maxTransfers,
    })
  }

  const itineraries = journeys.data?.itineraries ?? []
  const excludedDisruptions = journeys.data?.excluded_disruptions ?? []
  const hasEstimates = itineraries.some(
    (itinerary) =>
      hasValidTime(itinerary.estimated_departure_time) ||
      hasValidTime(itinerary.estimated_arrival_time) ||
      itinerary.legs.some(
        (leg) =>
          hasValidTime(leg.estimated_departure_time) ||
          hasValidTime(leg.estimated_arrival_time),
      ),
  )
  const selectedItinerary =
    selectedIndex !== null ? itineraries[selectedIndex] ?? null : null
  const selectedTimes = selectedItinerary ? resolveTimes(selectedItinerary) : null

  return (
    <>
      <form className="view-toolbar journey-toolbar" onSubmit={handleSubmit} noValidate>
        <div className="field journey-location-field">
          <label htmlFor="journey-origin">Origin</label>
          <input
            id="journey-origin"
            aria-invalid={Boolean(validationError)}
            aria-describedby={validationError ? 'journey-validation-error' : undefined}
            type="search"
            list="transit-stops-list"
            placeholder="e.g. Central Station"
            value={origin}
            onChange={(event) => setOrigin(event.target.value)}
          />
        </div>

        <button
          type="button"
          className="button button-secondary journey-swap-button"
          aria-label="Swap origin and destination"
          onClick={handleSwap}
        >
          <ArrowLeftRight size={16} aria-hidden="true" />
        </button>

        <div className="field journey-location-field">
          <label htmlFor="journey-destination">Destination</label>
          <input
            id="journey-destination"
            aria-invalid={Boolean(validationError)}
            aria-describedby={validationError ? 'journey-validation-error' : undefined}
            type="search"
            list="transit-stops-list"
            placeholder="e.g. Fairway Station"
            value={destination}
            onChange={(event) => setDestination(event.target.value)}
          />
        </div>

        <div className="field journey-window-field">
          <label htmlFor="journey-window">Search window</label>
          <select
            id="journey-window"
            value={hours}
            onChange={(event) => setHours(Number(event.target.value))}
          >
            <option value={2}>Next 2 hours</option>
            <option value={6}>Next 6 hours</option>
            <option value={24}>Next 24 hours</option>
          </select>
        </div>

        <div className="field journey-transfers-field">
          <label htmlFor="journey-transfers">Max transfers</label>
          <select
            id="journey-transfers"
            value={maxTransfers}
            onChange={(event) => setMaxTransfers(Number(event.target.value))}
          >
            <option value={3}>Up to 3 transfers</option>
            <option value={2}>Up to 2 transfers</option>
            <option value={1}>Up to 1 transfer</option>
            <option value={0}>Direct only</option>
          </select>
        </div>

        <button type="submit" className="button button-primary journey-submit-button">
          Find journeys
        </button>

        <datalist id="transit-stops-list">
          {stopNames.map((name) => <option key={name} value={name} />)}
        </datalist>
      </form>

      <RouteMap itinerary={selectedItinerary} />

      {validationError && (
        <div id="journey-validation-error" className="filter-warning journey-validation" role="alert">
          <AlertCircle size={17} aria-hidden="true" />
          <p>{validationError}</p>
        </div>
      )}

      {!submitted ? (
        <EmptyState
          title="Plan a transit journey"
          description="Enter an origin and destination stop or station to search for direct, multi-transfer, and walking-connected itineraries."
        />
      ) : journeys.error ? (
        <ErrorNotice error={journeys.error} onRetry={journeys.retry} />
      ) : !journeys.data ? (
        <LoadingRows label="Searching transit journeys" variant="journeys" />
      ) : (
        <>
          <div className="results-heading">
            <div>
              <h3>{journeys.data.origin} to {journeys.data.destination}</h3>
              <p>{journeys.data.message} · Next {submitted.hours} hours</p>
            </div>
            <span className="result-count">
              {itineraries.length}{' '}
              {itineraries.length === 1 ? 'option' : 'options'}
            </span>
          </div>

          {excludedDisruptions.length > 0 && (
            <div className="journey-disruption-notice" role="status" aria-label="Excluded disruptions">
              <AlertCircle size={17} aria-hidden="true" />
              <div>
                <strong>
                  {excludedDisruptions.length === 1
                    ? '1 disruption avoided during trip planning:'
                    : `${excludedDisruptions.length} disruptions avoided during trip planning:`}
                </strong>
                <ul className="journey-disruption-list">
                  {excludedDisruptions.map((disruption) => (
                    <li key={disruption.id}>
                      <span>{disruption.title}</span>
                      <span className="journey-disruption-meta">
                        {formatDisruptionType(disruption.type)} · {disruption.status}
                      </span>
                    </li>
                  ))}
                </ul>
              </div>
            </div>
          )}

          {itineraries.length > 0 && (
            <div className={`estimate-note ${hasEstimates ? 'has-estimates' : ''}`}>
              <Info size={16} aria-hidden="true" />
              <span>
                {hasEstimates
                  ? 'Live estimates are shown where available alongside scheduled times.'
                  : 'Showing scheduled times. Estimates will appear here when available.'}
              </span>
            </div>
          )}

          {selectedItinerary && selectedTimes && (
            <div className="journey-selected-banner" role="status" aria-label="Selected itinerary">
              <div className="journey-selected-details">
                <strong>Selected itinerary (Option {selectedIndex + 1})</strong>
                <p>
                  {formatTime(selectedTimes.departure)} → {formatTime(selectedTimes.arrival)}{' '}
                  ({selectedTimes.isEstimated ? 'Estimated' : 'Scheduled'}) ·{' '}
                  {selectedItinerary.duration_minutes} min ·{' '}
                  {selectedItinerary.legs
                    .map((leg) => (leg.mode === 'walk' ? 'Walk' : `Route ${leg.route?.number}`))
                    .join(' → ')}
                </p>
              </div>
              <button
                type="button"
                className="button button-secondary button-compact"
                onClick={() => setSelectedIndex(null)}
              >
                Clear selection
              </button>
            </div>
          )}

          {itineraries.length === 0 ? (
            <EmptyState
              title="No journeys found"
              description={
                journeys.data.message ||
                'No transit itineraries found between these locations in the selected time window.'
              }
            />
          ) : (
            <div className="journey-list" aria-label="Journey options">
              {itineraries.map((itinerary, index) => (
                <ItineraryCard
                  key={`${itinerary.origin_stop.id}-${itinerary.destination_stop.id}-${itinerary.scheduled_departure_time}-${index}`}
                  itinerary={itinerary}
                  selected={selectedIndex === index}
                  onSelect={() => setSelectedIndex((current) => (current === index ? null : index))}
                  today={today}
                />
              ))}
            </div>
          )}
        </>
      )}

      <div className="schedule-note">
        <p>Searches scheduled weekday and weekend service in Eastern time, excluding active disruptions and including walking links and up to 3 transfers.</p>
        <p>Walking estimates use straight-line stop distances (up to 600 m) with a street-routing factor. Holiday changes are not included.</p>
      </div>
    </>
  )
}

function LegStopTimes({ leg }) {
  const times = resolveTimes(leg)
  return (
    <div className="journey-leg-stops">
      <div className={times.hasEstimatedDeparture ? 'is-estimated' : ''}>
        <time dateTime={times.departure}>{formatTime(times.departure)}</time>
        <span>{leg.from_stop_name}</span>
        {times.hasEstimatedDeparture && (
          <small className="journey-leg-sched">
            Sched. {formatTime(leg.scheduled_departure_time)}
          </small>
        )}
      </div>
      <div className={times.hasEstimatedArrival ? 'is-estimated' : ''}>
        <time dateTime={times.arrival}>{formatTime(times.arrival)}</time>
        <span>{leg.to_stop_name}</span>
        {times.hasEstimatedArrival && (
          <small className="journey-leg-sched">
            Sched. {formatTime(leg.scheduled_arrival_time)}
          </small>
        )}
      </div>
    </div>
  )
}

function ItineraryCard({ itinerary, selected, onSelect, today }) {
  const colors = routePalette(itinerary.legs.filter((leg) => leg.route).map((leg) => leg.route))
  const times = resolveTimes(itinerary)
  const departureDay = serviceDate(new Date(times.departure))
  const dayLabel = departureDay !== today
    ? formatDate(departureDay, { weekday: 'short', month: 'short' })
    : null
  const hasTransit = itinerary.legs.some((leg) => leg.mode !== 'walk')
  const transferLabel = !hasTransit
    ? 'Walk only'
    : itinerary.transfers === 0
      ? 'Direct'
      : `${itinerary.transfers} ${itinerary.transfers === 1 ? 'transfer' : 'transfers'}`

  return (
    <article
      className={`journey-card ${selected ? 'is-selected' : ''}`}
      aria-label={`${itinerary.origin_stop.name} to ${itinerary.destination_stop.name}, departs ${formatTime(times.departure)}, arrives ${formatTime(times.arrival)}`}
    >
      <div className="journey-card-header">
        <div className="journey-time-block">
          <div className={`journey-summary-times ${times.isEstimated ? 'is-estimated' : ''}`}>
            <time dateTime={times.departure}>{formatTime(times.departure)}</time>
            <ArrowRight size={16} aria-hidden="true" />
            <time dateTime={times.arrival}>{formatTime(times.arrival)}</time>
            <span className="journey-time-source">
              {times.isEstimated && <Clock3 size={12} aria-hidden="true" />}
              {times.isEstimated ? 'Estimated' : 'Scheduled'}
            </span>
            {dayLabel && <span className="journey-day-label">{dayLabel}</span>}
          </div>
          {times.isEstimated && (
            <p className="journey-estimated-ref">
              Scheduled {formatTime(itinerary.scheduled_departure_time)} →{' '}
              {formatTime(itinerary.scheduled_arrival_time)}
            </p>
          )}
        </div>
        <div className="journey-badges">
          {itinerary.walking_minutes > 0 && (
            <span className="journey-walk-badge">
              <Footprints size={14} aria-hidden="true" />
              {itinerary.walking_minutes} min walk
            </span>
          )}
          <span className="journey-duration">{itinerary.duration_minutes} min</span>
          <span className={`status-pill ${!hasTransit || itinerary.transfers > 0 ? 'is-transfer' : 'is-active'}`}>
            {transferLabel}
          </span>
          <button
            type="button"
            className={`button button-compact journey-select-button ${selected ? 'button-primary' : 'button-secondary'}`}
            aria-pressed={selected}
            onClick={onSelect}
          >
            {selected && <Check size={14} aria-hidden="true" />}
            {selected ? 'Selected' : 'Select itinerary'}
          </button>
        </div>
      </div>

      <p className="journey-endpoints">
        {itinerary.origin_stop.name} to {itinerary.destination_stop.name}
      </p>

      <ol className="journey-legs">
        {itinerary.legs.map((leg, index) => {
          if (leg.mode === 'walk') {
            return (
              <li
                className="journey-leg journey-leg-walk"
                key={`walk-${leg.from_stop_id}-${leg.to_stop_id}-${index}`}
              >
                <div className="journey-leg-main">
                  <div className="journey-leg-route">
                    <span className="journey-walk-icon" aria-hidden="true">
                      <Footprints size={16} />
                    </span>
                    <div>
                      <strong>Walk {leg.duration_minutes} min (~{leg.distance_meters} m)</strong>
                      <small>{leg.from_stop_name} to {leg.to_stop_name}</small>
                    </div>
                  </div>
                  <LegStopTimes leg={leg} />
                </div>
              </li>
            )
          }

          const prevLeg = index > 0 ? itinerary.legs[index - 1] : null
          const waitMinutes = prevLeg && prevLeg.mode !== 'walk'
            ? Math.max(0, Math.round((Date.parse(leg.scheduled_departure_time) - Date.parse(prevLeg.scheduled_arrival_time)) / 60000))
            : 0
          const legStops = Array.isArray(leg.stops) ? leg.stops : []

          return (
            <li
              className="journey-leg"
              key={`${leg.trip_id}-${leg.from_stop_sequence}-${leg.to_stop_sequence}-${index}`}
            >
              {prevLeg && prevLeg.mode !== 'walk' && (
                <p className="journey-transfer-note">
                  Transfer at {leg.from_stop_name}{waitMinutes > 0 ? ` · ${waitMinutes} min wait` : ''}
                </p>
              )}
              <div className="journey-leg-main">
                <div className="journey-leg-route">
                  <RouteBadge number={leg.route?.number} color={colors.get(String(leg.route?.number))} />
                  <div>
                    <strong>{leg.route?.name}</strong>
                    <small>
                      {directionLabel(leg.route?.direction)} · Trip {leg.trip_id} · {leg.stop_count}{' '}
                      {leg.stop_count === 1 ? 'stop' : 'stops'} · {leg.duration_minutes} min
                    </small>
                  </div>
                </div>
                <LegStopTimes leg={leg} />
              </div>
              {legStops.length > 0 && (
                <details className="journey-leg-stops-disclosure">
                  <summary>
                    <ChevronDown size={15} className="disclosure-chevron" aria-hidden="true" />
                    <span>
                      View stops ({legStops.length} {legStops.length === 1 ? 'stop' : 'stops'})
                    </span>
                  </summary>
                  <div className="stop-table-wrap">
                    <table className="stop-table journey-leg-stop-table">
                      <thead>
                        <tr>
                          <th scope="col">Stop</th>
                          <th scope="col">Arrival</th>
                          <th scope="col">Departure</th>
                        </tr>
                      </thead>
                      <tbody>
                        {legStops.map((stopTime) => (
                          <tr key={`${stopTime.route_stop_id}-${stopTime.stop_sequence}`}>
                            <th scope="row">
                              <span className="stop-sequence">{stopTime.stop_sequence}</span>
                              {stopTime.stop_name}
                            </th>
                            <td>
                              <time dateTime={stopTime.scheduled_arrival_time}>
                                {formatTime(stopTime.scheduled_arrival_time)}
                              </time>
                            </td>
                            <td>
                              <time dateTime={stopTime.scheduled_departure_time}>
                                {formatTime(stopTime.scheduled_departure_time)}
                              </time>
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </details>
              )}
            </li>
          )
        })}
      </ol>
    </article>
  )
}
