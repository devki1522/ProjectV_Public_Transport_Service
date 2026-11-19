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

export const stopsCatalog = [
  { id: 21, name: 'Central Station', latitude: 43.45, longitude: -80.49, wheelchair_boarding: 1, is_active: true },
  { id: 22, name: 'Fairway Station', latitude: 43.42, longitude: -80.43, wheelchair_boarding: 1, is_active: true },
  { id: 23, name: 'University Terminal', latitude: 43.47, longitude: -80.54, wheelchair_boarding: 1, is_active: true },
]

export function journeyResult({
  origin = 'Central Station',
  destination = 'Fairway Station',
  empty = false,
  estimate = false,
  excludedDisruptions = [],
} = {}) {
  const disruptionFields = excludedDisruptions.length > 0
    ? { excluded_disruptions: excludedDisruptions }
    : {}
  if (empty) {
    return {
      origin,
      destination,
      origin_stops: [stopsCatalog[0]],
      destination_stops: [stopsCatalog[1]],
      from: '2026-11-10T13:00:00Z',
      until: '2026-11-10T19:00:00Z',
      timezone: 'America/Toronto',
      itineraries: [],
      ...disruptionFields,
      message: 'No transit itineraries found between the selected origin and destination in this time window.',
    }
  }
  const firstEstimatedDeparture = estimate ? '2026-11-10T13:08:00Z' : null
  const firstEstimatedArrival = estimate ? '2026-11-10T13:28:00Z' : null
  return {
    origin,
    destination,
    origin_stops: [stopsCatalog[0]],
    destination_stops: [stopsCatalog[1]],
    from: '2026-11-10T13:00:00Z',
    until: '2026-11-10T19:00:00Z',
    timezone: 'America/Toronto',
    ...disruptionFields,
    message: 'Found 3 itinerary options.',
    itineraries: [
      {
        origin_stop: stopsCatalog[0],
        destination_stop: stopsCatalog[1],
        scheduled_departure_time: '2026-11-10T13:05:00Z',
        scheduled_arrival_time: '2026-11-10T13:25:00Z',
        estimated_departure_time: firstEstimatedDeparture,
        estimated_arrival_time: firstEstimatedArrival,
        duration_minutes: 20,
        walking_minutes: 0,
        transfers: 0,
        legs: [
          {
            mode: 'transit',
            trip_id: 100,
            route: routes[0],
            service_date: '2026-11-10',
            from_stop_id: 21,
            from_stop_name: 'Central Station',
            from_stop_sequence: 1,
            to_stop_id: 22,
            to_stop_name: 'Fairway Station',
            to_stop_sequence: 5,
            stop_count: 4,
            scheduled_departure_time: '2026-11-10T13:05:00Z',
            scheduled_arrival_time: '2026-11-10T13:25:00Z',
            estimated_departure_time: firstEstimatedDeparture,
            estimated_arrival_time: firstEstimatedArrival,
            duration_minutes: 20,
            stops: [
              {
                route_stop_id: 1001,
                stop_id: 21,
                stop_name: 'Central Station',
                stop_sequence: 1,
                scheduled_arrival_time: '2026-11-10T13:05:00Z',
                scheduled_departure_time: '2026-11-10T13:05:00Z',
              },
              {
                route_stop_id: 1002,
                stop_id: 29,
                stop_name: 'Midway Stop',
                stop_sequence: 3,
                scheduled_arrival_time: '2026-11-10T13:15:00Z',
                scheduled_departure_time: '2026-11-10T13:15:00Z',
              },
              {
                route_stop_id: 1003,
                stop_id: 22,
                stop_name: 'Fairway Station',
                stop_sequence: 5,
                scheduled_arrival_time: '2026-11-10T13:25:00Z',
                scheduled_departure_time: '2026-11-10T13:25:00Z',
              },
            ],
          },
        ],
      },
      {
        origin_stop: stopsCatalog[0],
        destination_stop: stopsCatalog[1],
        scheduled_departure_time: '2026-11-10T13:10:00Z',
        scheduled_arrival_time: '2026-11-10T13:45:00Z',
        estimated_departure_time: null,
        estimated_arrival_time: null,
        duration_minutes: 35,
        walking_minutes: 0,
        transfers: 1,
        legs: [
          {
            mode: 'transit',
            trip_id: 101,
            route: routes[0],
            service_date: '2026-11-10',
            from_stop_id: 21,
            from_stop_name: 'Central Station',
            from_stop_sequence: 1,
            to_stop_id: 23,
            to_stop_name: 'University Terminal',
            to_stop_sequence: 3,
            stop_count: 2,
            scheduled_departure_time: '2026-11-10T13:10:00Z',
            scheduled_arrival_time: '2026-11-10T13:22:00Z',
            estimated_departure_time: null,
            estimated_arrival_time: null,
            duration_minutes: 12,
            stops: [
              {
                route_stop_id: 1101,
                stop_id: 21,
                stop_name: 'Central Station',
                stop_sequence: 1,
                scheduled_arrival_time: '2026-11-10T13:10:00Z',
                scheduled_departure_time: '2026-11-10T13:10:00Z',
              },
              {
                route_stop_id: 1102,
                stop_id: 23,
                stop_name: 'University Terminal',
                stop_sequence: 3,
                scheduled_arrival_time: '2026-11-10T13:22:00Z',
                scheduled_departure_time: '2026-11-10T13:22:00Z',
              },
            ],
          },
          {
            mode: 'transit',
            trip_id: 202,
            route: routes[2],
            service_date: '2026-11-10',
            from_stop_id: 23,
            from_stop_name: 'University Terminal',
            from_stop_sequence: 1,
            to_stop_id: 22,
            to_stop_name: 'Fairway Station',
            to_stop_sequence: 4,
            stop_count: 3,
            scheduled_departure_time: '2026-11-10T13:28:00Z',
            scheduled_arrival_time: '2026-11-10T13:45:00Z',
            estimated_departure_time: null,
            estimated_arrival_time: null,
            duration_minutes: 17,
            stops: [
              {
                route_stop_id: 2101,
                stop_id: 23,
                stop_name: 'University Terminal',
                stop_sequence: 1,
                scheduled_arrival_time: '2026-11-10T13:28:00Z',
                scheduled_departure_time: '2026-11-10T13:28:00Z',
              },
              {
                route_stop_id: 2102,
                stop_id: 22,
                stop_name: 'Fairway Station',
                stop_sequence: 4,
                scheduled_arrival_time: '2026-11-10T13:45:00Z',
                scheduled_departure_time: '2026-11-10T13:45:00Z',
              },
            ],
          },
        ],
      },
      {
        origin_stop: stopsCatalog[0],
        destination_stop: stopsCatalog[1],
        scheduled_departure_time: '2026-11-10T13:15:00Z',
        scheduled_arrival_time: '2026-11-10T13:57:00Z',
        estimated_departure_time: null,
        estimated_arrival_time: null,
        duration_minutes: 42,
        walking_minutes: 2,
        transfers: 2,
        legs: [
          {
            mode: 'transit',
            trip_id: 105,
            route: routes[0],
            service_date: '2026-11-10',
            from_stop_id: 21,
            from_stop_name: 'Central Station',
            from_stop_sequence: 1,
            to_stop_id: 23,
            to_stop_name: 'University Terminal',
            to_stop_sequence: 3,
            stop_count: 2,
            scheduled_departure_time: '2026-11-10T13:15:00Z',
            scheduled_arrival_time: '2026-11-10T13:25:00Z',
            estimated_departure_time: null,
            estimated_arrival_time: null,
            duration_minutes: 10,
            stops: [
              {
                route_stop_id: 3101,
                stop_id: 21,
                stop_name: 'Central Station',
                stop_sequence: 1,
                scheduled_arrival_time: '2026-11-10T13:15:00Z',
                scheduled_departure_time: '2026-11-10T13:15:00Z',
              },
              {
                route_stop_id: 3102,
                stop_id: 23,
                stop_name: 'University Terminal',
                stop_sequence: 3,
                scheduled_arrival_time: '2026-11-10T13:25:00Z',
                scheduled_departure_time: '2026-11-10T13:25:00Z',
              },
            ],
          },
          {
            mode: 'transit',
            trip_id: 205,
            route: routes[1],
            service_date: '2026-11-10',
            from_stop_id: 23,
            from_stop_name: 'University Terminal',
            from_stop_sequence: 1,
            to_stop_id: 24,
            to_stop_name: 'Midtown Hub',
            to_stop_sequence: 2,
            stop_count: 1,
            scheduled_departure_time: '2026-11-10T13:30:00Z',
            scheduled_arrival_time: '2026-11-10T13:40:00Z',
            estimated_departure_time: null,
            estimated_arrival_time: null,
            duration_minutes: 10,
            stops: [
              {
                route_stop_id: 3201,
                stop_id: 23,
                stop_name: 'University Terminal',
                stop_sequence: 1,
                scheduled_arrival_time: '2026-11-10T13:30:00Z',
                scheduled_departure_time: '2026-11-10T13:30:00Z',
              },
              {
                route_stop_id: 3202,
                stop_id: 24,
                stop_name: 'Midtown Hub',
                stop_sequence: 2,
                scheduled_arrival_time: '2026-11-10T13:40:00Z',
                scheduled_departure_time: '2026-11-10T13:40:00Z',
              },
            ],
          },
          {
            mode: 'transit',
            trip_id: 305,
            route: routes[2],
            service_date: '2026-11-10',
            from_stop_id: 24,
            from_stop_name: 'Midtown Hub',
            from_stop_sequence: 1,
            to_stop_id: 25,
            to_stop_name: 'Opposite Fairway',
            to_stop_sequence: 3,
            stop_count: 2,
            scheduled_departure_time: '2026-11-10T13:44:00Z',
            scheduled_arrival_time: '2026-11-10T13:55:00Z',
            estimated_departure_time: null,
            estimated_arrival_time: null,
            duration_minutes: 11,
            stops: [
              {
                route_stop_id: 3301,
                stop_id: 24,
                stop_name: 'Midtown Hub',
                stop_sequence: 1,
                scheduled_arrival_time: '2026-11-10T13:44:00Z',
                scheduled_departure_time: '2026-11-10T13:44:00Z',
              },
              {
                route_stop_id: 3302,
                stop_id: 25,
                stop_name: 'Opposite Fairway',
                stop_sequence: 3,
                scheduled_arrival_time: '2026-11-10T13:55:00Z',
                scheduled_departure_time: '2026-11-10T13:55:00Z',
              },
            ],
          },
          {
            mode: 'walk',
            from_stop_id: 25,
            from_stop_name: 'Opposite Fairway',
            to_stop_id: 22,
            to_stop_name: 'Fairway Station',
            distance_meters: 113,
            scheduled_departure_time: '2026-11-10T13:55:00Z',
            scheduled_arrival_time: '2026-11-10T13:57:00Z',
            estimated_departure_time: null,
            estimated_arrival_time: null,
            duration_minutes: 2,
          },
        ],
      },
    ],
  }
}

export const activeVehicles = [
  {
    vehicle_id: 501,
    vehicle_number: '2104',
    is_accessible: true,
    route_id: 1,
    route_number: '1',
    route_name: 'Queen-River',
    direction: 'direction_0',
    trip_id: 100,
    latitude: 43.4516,
    longitude: -80.4981,
    heading: 125,
    speed_kmh: 34,
    status: 'in_service',
    source_service: 'simulator',
    is_simulated: true,
    recorded_at: '2026-11-10T13:12:00Z',
    received_at: '2026-11-10T13:12:01Z',
  },
  {
    vehicle_id: 507,
    vehicle_number: '2207',
    is_accessible: true,
    route_id: 7,
    route_number: '7',
    route_name: 'King',
    direction: 'direction_0',
    trip_id: 200,
    latitude: 43.4723,
    longitude: -80.5449,
    heading: 210,
    speed_kmh: 22,
    status: 'delayed',
    source_service: 'admin',
    is_simulated: false,
    recorded_at: '2026-11-10T13:13:00Z',
    received_at: '2026-11-10T13:13:01Z',
  },
]

export function routeShape(routeId = 7) {
  const route = routes.find((item) => item.id === Number(routeId)) ?? routes[2]
  return {
    route_id: route.id,
    route_number: route.number,
    route_name: route.name,
    direction: route.direction,
    headsign: 'Fairway Station',
    coordinates: [
      { lat: 43.4723, lng: -80.5449 },
      { lat: 43.4516, lng: -80.4981 },
      { lat: 43.4209, lng: -80.4388 },
    ],
  }
}

export function simulatorStatus(overrides = {}) {
  return {
    running: true,
    vehicle_count: 8,
    route_count: 4,
    tick_interval_ms: 3000,
    speed_multiplier: 1,
    last_tick_at: '2026-11-10T13:12:00Z',
    total_updates_sent: 48,
    ...overrides,
  }
}

export function estimatedArrivals(tripId = 100) {
  const isKing = Number(tripId) === 200
  return [
    {
      trip_id: Number(tripId),
      route_id: isKing ? 7 : 1,
      route_number: isKing ? '7' : '1',
      route_name: isKing ? 'King' : 'Queen-River',
      direction: 'direction_0',
      stop_id: 21,
      stop_name: 'Central Station',
      stop_sequence: 2,
      vehicle_id: isKing ? 507 : 501,
      vehicle_number: isKing ? '2207' : '2104',
      vehicle_status: isKing ? 'delayed' : 'in_service',
      scheduled_arrival_time: '2026-11-10T13:15:00Z',
      estimated_arrival_time: '2026-11-10T13:18:00Z',
      delay_minutes: 3,
      distance_meters: 640,
      is_simulated: !isKing,
      recorded_at: '2026-11-10T13:12:00Z',
    },
    {
      trip_id: Number(tripId),
      route_id: isKing ? 7 : 1,
      route_number: isKing ? '7' : '1',
      route_name: isKing ? 'King' : 'Queen-River',
      direction: 'direction_0',
      stop_id: 22,
      stop_name: 'Fairway Station',
      stop_sequence: 5,
      vehicle_id: isKing ? 507 : 501,
      vehicle_number: isKing ? '2207' : '2104',
      vehicle_status: isKing ? 'delayed' : 'in_service',
      scheduled_arrival_time: '2026-11-10T13:30:00Z',
      estimated_arrival_time: '2026-11-10T13:30:00Z',
      delay_minutes: 0,
      distance_meters: 2450,
      is_simulated: !isKing,
      recorded_at: '2026-11-10T13:12:00Z',
    },
  ]
}

export const activeDisruptions = [
  {
    id: 301,
    type: 'detour',
    title: 'King St N watermain construction detour',
    description: 'Route 7 buses detour via Weber St N due to watermain replacement near University Ave.',
    affected_area: 'King St N & University Ave, Waterloo',
    start_time: '2026-11-10T11:00:00Z',
    end_time: '2026-11-10T18:00:00Z',
    status: 'active',
    source_service: 'utilities',
    external_reference: 'UTIL-WR-2026-101',
    received_at: '2026-11-10T11:02:00Z',
    affected_routes: [
      { route_id: 7, route_number: '7', route_name: 'King', direction: 'direction_0' },
    ],
    affected_stops: [
      { stop_id: 23, stop_name: 'University Terminal', route_id: 7 },
    ],
    affected_trips: [200],
    affected_vehicles: [507],
    affected_segments: ['King St N between University Ave and Columbia St'],
  },
  {
    id: 302,
    type: 'stop_closure',
    title: 'Central Station platform closure for emergency response',
    description: 'Emergency crews staging on King St W; board Route 1 at temporary stop on Victoria St.',
    affected_area: 'Central Station, Kitchener',
    start_time: '2026-11-10T12:15:00Z',
    end_time: null,
    status: 'active',
    source_service: 'emergency_services',
    external_reference: 'EMS-WR-2026-201',
    received_at: '2026-11-10T12:16:00Z',
    affected_routes: [
      { route_id: 1, route_number: '1', route_name: 'Queen-River', direction: 'direction_0' },
    ],
    affected_stops: [
      { stop_id: 21, stop_name: 'Central Station', route_id: 1 },
    ],
    affected_trips: [100],
    affected_vehicles: [501],
    affected_segments: ['King St W at Victoria St'],
  },
]

export const allDisruptions = [
  ...activeDisruptions,
  {
    id: 303,
    type: 'construction',
    title: 'Fairway Station overnight resurfacing',
    description: 'Scheduled overnight lane restrictions near Fairway Station.',
    affected_area: 'Fairway Rd S, Kitchener',
    start_time: '2026-11-11T03:00:00Z',
    end_time: '2026-11-11T09:00:00Z',
    status: 'scheduled',
    source_service: 'transit',
    external_reference: null,
    received_at: '2026-11-10T10:00:00Z',
    affected_routes: [
      { route_id: 1, route_number: '1', route_name: 'Queen-River', direction: 'direction_0' },
    ],
    affected_stops: [
      { stop_id: 22, stop_name: 'Fairway Station', route_id: 1 },
    ],
    affected_trips: [],
    affected_vehicles: [],
    affected_segments: ['Fairway Rd S terminal loop'],
  },
]

export function affectedServicesSummary({ disruptions = activeDisruptions } = {}) {
  const active = disruptions.filter((item) => item.status === 'active')
  const routeMap = new Map()
  const stopMap = new Map()
  const tripMap = new Map()
  const vehicleMap = new Map()
  const segments = []

  for (const d of active) {
    const opStatus =
      d.type === 'detour'
        ? 'detoured'
        : d.type === 'stop_closure'
          ? 'closed'
          : d.type === 'delay'
            ? 'delayed'
            : 'disrupted'

    for (const r of d.affected_routes ?? []) {
      if (!routeMap.has(r.route_id)) {
        routeMap.set(r.route_id, {
          route_id: r.route_id,
          route_number: r.route_number,
          route_name: r.route_name,
          direction: r.direction,
          operational_status: opStatus,
          disruption_ids: [d.id],
        })
      } else {
        routeMap.get(r.route_id).disruption_ids.push(d.id)
      }
    }

    for (const s of d.affected_stops ?? []) {
      if (!stopMap.has(s.stop_id)) {
        stopMap.set(s.stop_id, {
          stop_id: s.stop_id,
          stop_name: s.stop_name,
          operational_status: d.type === 'stop_closure' ? 'closed' : opStatus,
          disruption_ids: [d.id],
        })
      } else {
        stopMap.get(s.stop_id).disruption_ids.push(d.id)
      }
    }

    for (const tripId of d.affected_trips ?? []) {
      if (!tripMap.has(tripId)) {
        tripMap.set(tripId, {
          trip_id: tripId,
          route_id: d.affected_routes?.[0]?.route_id ?? 1,
          operational_status: opStatus,
          disruption_ids: [d.id],
        })
      }
    }

    for (const vehicleId of d.affected_vehicles ?? []) {
      if (!vehicleMap.has(vehicleId)) {
        const vehicle = activeVehicles.find((v) => v.vehicle_id === vehicleId)
        vehicleMap.set(vehicleId, {
          vehicle_id: vehicleId,
          vehicle_number: vehicle?.vehicle_number ?? String(vehicleId),
          operational_status: opStatus,
          disruption_ids: [d.id],
        })
      }
    }

    for (const seg of d.affected_segments ?? []) {
      segments.push({
        disruption_id: d.id,
        route_id: d.affected_routes?.[0]?.route_id ?? null,
        segment: seg,
        operational_status: opStatus,
      })
    }
  }

  return {
    generated_at: '2026-11-10T13:15:00Z',
    active_disruption_count: active.length,
    affected_routes: [...routeMap.values()],
    affected_stops: [...stopMap.values()],
    affected_trips: [...tripMap.values()],
    affected_vehicles: [...vehicleMap.values()],
    affected_segments: segments,
  }
}

export function serviceStatusResponse({ disruptions = activeDisruptions } = {}) {
  const active = disruptions.filter((item) => item.status === 'active')
  const summary = affectedServicesSummary({ disruptions: active })
  return {
    generated_at: summary.generated_at,
    overall_status: active.length > 0 ? 'disrupted' : 'normal',
    active_disruption_count: active.length,
    active_disruptions: active,
    detours: active.filter((item) => item.type === 'detour'),
    affected_routes: summary.affected_routes,
    affected_stops: summary.affected_stops,
    affected_trips: summary.affected_trips,
    affected_vehicles: summary.affected_vehicles,
    affected_segments: summary.affected_segments,
  }
}

export function routeOperationalStatus(routeId = 7, { disruptions = activeDisruptions } = {}) {
  const id = Number(routeId)
  const matching = disruptions.filter(
    (item) =>
      item.status === 'active' &&
      Array.isArray(item.affected_routes) &&
      item.affected_routes.some((r) => r.route_id === id),
  )
  const firstType = matching[0]?.type
  const opStatus =
    matching.length === 0
      ? 'normal'
      : firstType === 'detour'
        ? 'detoured'
        : firstType === 'delay'
          ? 'delayed'
          : 'disrupted'
  return {
    entity_type: 'route',
    entity_id: id,
    operational_status: opStatus,
    active_disruptions: matching,
  }
}
