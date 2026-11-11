export const TRANSIT_TIMEZONE = 'America/Toronto'

const dateParts = new Intl.DateTimeFormat('en-CA', {
  timeZone: TRANSIT_TIMEZONE, year: 'numeric', month: '2-digit', day: '2-digit',
})
const clock = new Intl.DateTimeFormat('en-CA', {
  timeZone: TRANSIT_TIMEZONE, hour: '2-digit', minute: '2-digit', hourCycle: 'h23',
})

export function serviceDate(now = new Date()) {
  const parts = Object.fromEntries(dateParts.formatToParts(now).map(({ type, value }) => [type, value]))
  return `${parts.year}-${parts.month}-${parts.day}`
}

export function formatTime(value) {
  const date = new Date(value)
  return value && !Number.isNaN(date.getTime()) ? clock.format(date) : '—'
}

export function formatDate(date, options = {}) {
  // Noon UTC keeps this date-only input on the same date in the agency zone.
  return new Intl.DateTimeFormat('en-CA', {
    timeZone: TRANSIT_TIMEZONE, weekday: 'long', month: 'long', day: 'numeric', ...options,
  }).format(new Date(`${date}T12:00:00Z`))
}

export function isServiceDate(value) {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value) || value.startsWith('0000')) return false
  const parsed = new Date(`${value}T12:00:00Z`)
  return !Number.isNaN(parsed.getTime()) && parsed.toISOString().slice(0, 10) === value
}

export function directionLabel(direction) {
  if (direction === 'direction_0') return 'Direction 1'
  if (direction === 'direction_1') return 'Direction 2'
  return direction || 'Direction unavailable'
}

export function groupRoutes(routes) {
  const groups = new Map()
  for (const route of routes) {
    if (!groups.has(route.number)) groups.set(route.number, { number: route.number, name: route.name, directions: [] })
    groups.get(route.number).directions.push(route)
  }
  return [...groups.values()]
    .sort((a, b) => a.number.localeCompare(b.number, 'en', { numeric: true }))
    .map((group) => ({ ...group, directions: group.directions.sort((a, b) => a.direction.localeCompare(b.direction)) }))
}

export function getStops(trips) {
  const stops = new Map()
  for (const trip of trips) {
    for (const stop of trip.stop_times) {
      if (!stops.has(stop.stop_id)) stops.set(stop.stop_id, { id: stop.stop_id, name: stop.stop_name })
    }
  }
  return [...stops.values()].sort((a, b) => a.name.localeCompare(b.name))
}

export function tripEndpoints(trip) {
  const stops = trip.stop_times
  return { first: stops[0], last: stops.at(-1) }
}

export function inPeriod(trip, period) {
  if (period === 'all') return true
  const departure = trip.stop_times[0]?.scheduled_departure_time
  if (!departure) return false
  const hour = Number(formatTime(departure).split(':')[0])
  return ({ night: hour < 6, morning: hour >= 6 && hour < 12, afternoon: hour >= 12 && hour < 18, evening: hour >= 18 })[period]
}

export function departureEstimate(departure) {
  const scheduled = new Date(departure.scheduled_departure_time)
  const estimated = new Date(departure.estimated_departure_time)
  if (!departure.estimated_departure_time || Number.isNaN(estimated.getTime())) return null
  return {
    time: estimated.toISOString(),
    delay: Math.round((estimated.getTime() - scheduled.getTime()) / 60000),
  }
}

export function dayOffset(value, date) {
  if (!value) return 0
  const actualDate = serviceDate(new Date(value))
  return Math.round((Date.parse(`${actualDate}T12:00:00Z`) - Date.parse(`${date}T12:00:00Z`)) / 86400000)
}
