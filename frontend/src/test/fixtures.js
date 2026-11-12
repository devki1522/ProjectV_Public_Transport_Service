export const routes = [
  { id: 1, number: '1', name: 'Queen-River', direction: 'direction_0', is_active: true },
  { id: 2, number: '1', name: 'Queen-River', direction: 'direction_1', is_active: true },
  { id: 7, number: '7', name: 'King', direction: 'direction_0', is_active: true },
  { id: 90, number: '90', name: 'Inactive', direction: 'direction_0', is_active: false },
]

export function timetable(routeId, date, count = 2) {
  return {
    route: routes.find((route) => route.id === Number(routeId)), service_date: date, timezone: 'America/Toronto',
    trips: Array.from({ length: count }, (_, index) => ({
      id: 100 + index, schedule_type: 'weekday', status: 'scheduled',
      stop_times: [
        { route_stop_id: 10, stop_id: 21, stop_name: 'Central Station', stop_sequence: 1, scheduled_arrival_time: `${date}T13:00:00Z`, scheduled_departure_time: `${date}T13:02:00Z` },
        { route_stop_id: 11, stop_id: 22, stop_name: 'Fairway Station', stop_sequence: 2, scheduled_arrival_time: `${date}T14:00:00Z`, scheduled_departure_time: `${date}T14:01:00Z` },
      ],
    })),
  }
}

export function departures(from = '2026-11-10T13:00:00Z') {
  const start = new Date(from).getTime() + 10 * 60_000
  return [
    { trip_id: 100, route: routes[0], route_stop_id: 10, stop_id: 21, stop_name: 'Central Station', stop_sequence: 1, service_date: from.slice(0, 10), scheduled_departure_time: new Date(start).toISOString(), estimated_departure_time: new Date(start + 5 * 60_000).toISOString() },
    { trip_id: 101, route: routes[0], route_stop_id: 11, stop_id: 22, stop_name: 'Fairway Station', stop_sequence: 2, service_date: from.slice(0, 10), scheduled_departure_time: new Date(start + 15 * 60_000).toISOString(), estimated_departure_time: null },
  ]
}

export function jsonResponse(data, status = 200) {
  return { ok: status >= 200 && status < 300, status, json: async () => data }
}
