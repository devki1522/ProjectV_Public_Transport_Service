import { apiPath } from './base.js'

export class TransitApiError extends Error {
  constructor(message, status) {
    super(message)
    this.name = 'TransitApiError'
    this.status = status
  }
}

async function getJSON(path, signal) {
  let response
  try {
    response = await fetch(apiPath(path), { signal, headers: { Accept: 'application/json' } })
  } catch (error) {
    if (error.name === 'AbortError') throw error
    throw new TransitApiError('Transit information is unavailable. Check your connection and try again.')
  }
  if (!response.ok) {
    const message = response.status === 404
      ? 'This route, stop or trip is no longer available. Please choose another.'
      : 'We couldn’t load transit information. Please try again.'
    throw new TransitApiError(message, response.status)
  }
  try {
    return await response.json()
  } catch {
    throw new TransitApiError('The transit service returned an unreadable response. Please try again.')
  }
}

function requireArray(data) {
  if (!Array.isArray(data)) throw new TransitApiError('The transit service returned unexpected data. Please try again.')
  return data
}

export async function getRoutes(signal) {
  return requireArray(await getJSON('/routes', signal)).filter((route) => route.is_active)
}

export async function getTimetable(routeId, date, signal) {
  const data = await getJSON(`/routes/${encodeURIComponent(routeId)}/schedule?${new URLSearchParams({ date })}`, signal)
  if (!data || !Array.isArray(data.trips)) throw new TransitApiError('The timetable is unavailable. Please try again.')
  return data
}

export async function getDepartures({ routeId, stopId, tripId, hours = 2 }, signal) {
  const from = new Date()
  const until = new Date(from.getTime() + hours * 60 * 60 * 1000)
  const params = new URLSearchParams({
    route_id: routeId, from: from.toISOString(), until: until.toISOString(), limit: '100',
  })
  if (stopId) params.set('stop_id', stopId)
  if (tripId) params.set('trip_id', tripId)
  return requireArray(await getJSON(`/departures?${params}`, signal))
}

export async function getStopsCatalog(query = '', signal) {
  const params = new URLSearchParams({ limit: '100' })
  if (typeof query === 'string' && query.trim()) params.set('q', query.trim())
  return requireArray(await getJSON(`/stops?${params}`, signal))
}

export async function getJourneys({
  origin,
  destination,
  originStopId,
  destinationStopId,
  hours = 6,
  maxTransfers = 3,
  maxWalkMeters = 600,
}, signal) {
  const from = new Date()
  const until = new Date(from.getTime() + hours * 60 * 60 * 1000)
  const params = new URLSearchParams({
    from: from.toISOString(),
    until: until.toISOString(),
    limit: '5',
    max_transfers: String(maxTransfers),
    max_walk_meters: String(maxWalkMeters),
  })
  if (originStopId) params.set('origin_stop_id', originStopId)
  else if (origin?.trim()) params.set('origin', origin.trim())
  if (destinationStopId) params.set('destination_stop_id', destinationStopId)
  else if (destination?.trim()) params.set('destination', destination.trim())
  const data = await getJSON(`/journeys?${params}`, signal)
  if (!data || !Array.isArray(data.itineraries)) {
    throw new TransitApiError('Journey results are unavailable. Please try again.')
  }
  return data
}

export async function getActiveVehicles({ routeId, routeNumber, tripId, maxAgeSeconds } = {}, signal) {
  const params = new URLSearchParams()
  if (routeId) params.set('route_id', String(routeId))
  if (typeof routeNumber === 'string' && routeNumber.trim()) params.set('route_number', routeNumber.trim())
  if (tripId) params.set('trip_id', String(tripId))
  if (maxAgeSeconds) params.set('max_age_seconds', String(maxAgeSeconds))
  const query = params.toString()
  return requireArray(await getJSON(query ? `/vehicles?${query}` : '/vehicles', signal))
}

export async function getRouteShape(routeId, signal) {
  const data = await getJSON(`/routes/${encodeURIComponent(routeId)}/shape`, signal)
  if (!data || !Array.isArray(data.coordinates)) {
    throw new TransitApiError('Route map path is unavailable. Please try again.')
  }
  return data
}

export async function getSimulatorStatus(signal) {
  const data = await getJSON('/vehicles/simulator', signal)
  if (!data || typeof data.running !== 'boolean') {
    throw new TransitApiError('Simulator status is unavailable. Please try again.')
  }
  return data
}

export async function getTripArrivals(tripId, signal) {
  return requireArray(await getJSON(`/trips/${encodeURIComponent(tripId)}/arrivals`, signal))
}

export async function getStopArrivals(stopId, routeId, signal) {
  const params = new URLSearchParams()
  if (routeId) params.set('route_id', String(routeId))
  const query = params.toString()
  const path = `/stops/${encodeURIComponent(stopId)}/arrivals${query ? `?${query}` : ''}`
  return requireArray(await getJSON(path, signal))
}

export async function getActiveDisruptions({ type, routeId, stopId } = {}, signal) {
  const params = new URLSearchParams()
  if (typeof type === 'string' && type.trim()) params.set('type', type.trim())
  if (routeId) params.set('route_id', String(routeId))
  if (stopId) params.set('stop_id', String(stopId))
  const query = params.toString()
  const path = query ? `/disruptions/active?${query}` : '/disruptions/active'
  return requireArray(await getJSON(path, signal))
}

export async function getServiceStatus(signal) {
  const data = await getJSON('/service-status', signal)
  if (!data || typeof data !== 'object' || Array.isArray(data) || !Array.isArray(data.active_disruptions)) {
    throw new TransitApiError('Service status is unavailable. Please try again.')
  }
  return data
}

export async function getAffectedServices(signal) {
  const data = await getJSON('/disruptions/affected', signal)
  if (
    !data ||
    typeof data !== 'object' ||
    Array.isArray(data) ||
    !Array.isArray(data.affected_routes) ||
    !Array.isArray(data.affected_stops)
  ) {
    throw new TransitApiError('Affected service information is unavailable. Please try again.')
  }
  return data
}

export async function getRouteStatus(routeId, signal) {
  const data = await getJSON(`/routes/${encodeURIComponent(routeId)}/status`, signal)
  if (
    !data ||
    typeof data !== 'object' ||
    Array.isArray(data) ||
    typeof data.operational_status !== 'string' ||
    !Array.isArray(data.active_disruptions)
  ) {
    throw new TransitApiError('Route status is unavailable. Please try again.')
  }
  return data
}
