import { afterEach, expect, it, vi } from 'vitest'
import {
  getActiveDisruptions,
  getActiveVehicles,
  getAffectedServices,
  getDepartures,
  getJourneys,
  getRouteShape,
  getRouteStatus,
  getRoutes,
  getServiceStatus,
  getSimulatorStatus,
  getStopArrivals,
  getStopsCatalog,
  getTimetable,
  getTripArrivals,
} from './transit.js'
import {
  activeDisruptions,
  activeVehicles,
  affectedServicesSummary,
  estimatedArrivals,
  journeyResult,
  jsonResponse,
  routeOperationalStatus,
  routeShape,
  routes,
  serviceStatusResponse,
  simulatorStatus,
  stopsCatalog,
} from '../test/fixtures.js'

afterEach(() => vi.useRealTimers())

it('loads only active routes and passes the request cancellation signal', async () => {
  const fetch = vi.fn().mockResolvedValue(jsonResponse(routes))
  vi.stubGlobal('fetch', fetch)
  const signal = new AbortController().signal
  expect(await getRoutes(signal)).toHaveLength(3)
  expect(fetch).toHaveBeenCalledWith('/api/transport/v1/routes', expect.objectContaining({ signal }))
})

it('requests a bounded departure window and combines optional filters', async () => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date('2026-11-10T13:00:00Z'))
  const fetch = vi.fn().mockResolvedValue(jsonResponse([]))
  vi.stubGlobal('fetch', fetch)
  await getDepartures({ routeId: 7, stopId: 21, tripId: 100, hours: 6 })
  const url = new URL(fetch.mock.calls[0][0], 'https://transit.test')
  expect(Object.fromEntries(url.searchParams)).toEqual({ route_id: '7', stop_id: '21', trip_id: '100', from: '2026-11-10T13:00:00.000Z', until: '2026-11-10T19:00:00.000Z', limit: '100' })
})

it('reports server, network and malformed-response errors without exposing internals', async () => {
  const fetch = vi.fn()
    .mockResolvedValueOnce(jsonResponse({ error: 'secret database error' }, 500))
    .mockResolvedValueOnce(jsonResponse({}, 404))
    .mockRejectedValueOnce(new TypeError('Network failed'))
    .mockResolvedValueOnce({ ok: true, json: async () => { throw new Error('bad json') } })
    .mockResolvedValueOnce(jsonResponse({}))
  vi.stubGlobal('fetch', fetch)
  await expect(getRoutes()).rejects.toThrow('Please try again')
  await expect(getRoutes()).rejects.toThrow('no longer available')
  await expect(getRoutes()).rejects.toThrow('Check your connection')
  await expect(getRoutes()).rejects.toThrow('unreadable response')
  await expect(getTimetable(7, '2026-11-10')).rejects.toThrow('timetable is unavailable')
})

it('loads stop suggestions with optional search query and cancellation signal', async () => {
  const fetch = vi.fn().mockResolvedValue(jsonResponse(stopsCatalog))
  vi.stubGlobal('fetch', fetch)
  const signal = new AbortController().signal
  expect(await getStopsCatalog('Station', signal)).toHaveLength(3)
  expect(fetch).toHaveBeenCalledWith('/api/transport/v1/stops?limit=100&q=Station', expect.objectContaining({ signal }))
})

it('requests journey options using text locations or stop IDs and validates the response shape', async () => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date('2026-11-10T13:00:00Z'))
  const fetch = vi.fn()
    .mockResolvedValueOnce(jsonResponse(journeyResult()))
    .mockResolvedValueOnce(jsonResponse(journeyResult()))
    .mockResolvedValueOnce(jsonResponse({}))
  vi.stubGlobal('fetch', fetch)

  const textResult = await getJourneys({ origin: 'Central Station', destination: 'Fairway Station', hours: 6 })
  expect(textResult.itineraries).toHaveLength(3)
  const textUrl = new URL(fetch.mock.calls[0][0], 'https://transit.test')
  expect(Object.fromEntries(textUrl.searchParams)).toEqual({
    from: '2026-11-10T13:00:00.000Z',
    until: '2026-11-10T19:00:00.000Z',
    limit: '5',
    max_transfers: '3',
    max_walk_meters: '600',
    origin: 'Central Station',
    destination: 'Fairway Station',
  })

  await getJourneys({ originStopId: 21, destinationStopId: 22, hours: 2, maxTransfers: 2, maxWalkMeters: 400 })
  const idUrl = new URL(fetch.mock.calls[1][0], 'https://transit.test')
  expect(Object.fromEntries(idUrl.searchParams)).toEqual({
    from: '2026-11-10T13:00:00.000Z',
    until: '2026-11-10T15:00:00.000Z',
    limit: '5',
    max_transfers: '2',
    max_walk_meters: '400',
    origin_stop_id: '21',
    destination_stop_id: '22',
  })

  await expect(getJourneys({ origin: 'A', destination: 'B' })).rejects.toThrow('Journey results are unavailable')
})

it('loads active vehicles with optional route and trip filters and validates array response', async () => {
  const fetch = vi.fn()
    .mockResolvedValueOnce(jsonResponse(activeVehicles))
    .mockResolvedValueOnce(jsonResponse([activeVehicles[1]]))
    .mockResolvedValueOnce(jsonResponse({ unexpected: true }))
  vi.stubGlobal('fetch', fetch)
  const signal = new AbortController().signal

  expect(await getActiveVehicles({}, signal)).toHaveLength(2)
  expect(fetch).toHaveBeenNthCalledWith(1, '/api/transport/v1/vehicles', expect.objectContaining({ signal }))

  expect(await getActiveVehicles({ routeId: 7, routeNumber: ' 7 ', tripId: 200 }, signal)).toHaveLength(1)
  const filteredUrl = new URL(fetch.mock.calls[1][0], 'https://transit.test')
  expect(Object.fromEntries(filteredUrl.searchParams)).toEqual({
    route_id: '7',
    route_number: '7',
    trip_id: '200',
  })

  await expect(getActiveVehicles()).rejects.toThrow('unexpected data')
})

it('loads route geometry shape and simulator status and rejects malformed payloads', async () => {
  const fetch = vi.fn()
    .mockResolvedValueOnce(jsonResponse(routeShape(7)))
    .mockResolvedValueOnce(jsonResponse({}))
    .mockResolvedValueOnce(jsonResponse(simulatorStatus()))
    .mockResolvedValueOnce(jsonResponse({}))
  vi.stubGlobal('fetch', fetch)
  const signal = new AbortController().signal

  const shape = await getRouteShape(7, signal)
  expect(shape.coordinates).toHaveLength(3)
  expect(fetch).toHaveBeenNthCalledWith(1, '/api/transport/v1/routes/7/shape', expect.objectContaining({ signal }))
  await expect(getRouteShape(7)).rejects.toThrow('Route map path is unavailable')

  const status = await getSimulatorStatus(signal)
  expect(status.running).toBe(true)
  expect(fetch).toHaveBeenNthCalledWith(3, '/api/transport/v1/vehicles/simulator', expect.objectContaining({ signal }))
  await expect(getSimulatorStatus()).rejects.toThrow('Simulator status is unavailable')
})

it('loads trip arrivals and stop arrivals with optional route filter', async () => {
  const fetch = vi.fn()
    .mockResolvedValueOnce(jsonResponse(estimatedArrivals(100)))
    .mockResolvedValueOnce(jsonResponse(estimatedArrivals(100)))
    .mockResolvedValueOnce(jsonResponse(estimatedArrivals(200)))
  vi.stubGlobal('fetch', fetch)
  const signal = new AbortController().signal

  expect(await getTripArrivals(100, signal)).toHaveLength(2)
  expect(fetch).toHaveBeenNthCalledWith(1, '/api/transport/v1/trips/100/arrivals', expect.objectContaining({ signal }))

  expect(await getStopArrivals(21, undefined, signal)).toHaveLength(2)
  expect(fetch).toHaveBeenNthCalledWith(2, '/api/transport/v1/stops/21/arrivals', expect.objectContaining({ signal }))

  expect(await getStopArrivals(21, 7, signal)).toHaveLength(2)
  expect(fetch).toHaveBeenNthCalledWith(3, '/api/transport/v1/stops/21/arrivals?route_id=7', expect.objectContaining({ signal }))
})

it('loads active disruptions, service status, affected services, and route operational status', async () => {
  const fetch = vi.fn()
    .mockResolvedValueOnce(jsonResponse(activeDisruptions))
    .mockResolvedValueOnce(jsonResponse([activeDisruptions[0]]))
    .mockResolvedValueOnce(jsonResponse({ bad: true }))
    .mockResolvedValueOnce(jsonResponse(serviceStatusResponse()))
    .mockResolvedValueOnce(jsonResponse([]))
    .mockResolvedValueOnce(jsonResponse(affectedServicesSummary()))
    .mockResolvedValueOnce(jsonResponse({}))
    .mockResolvedValueOnce(jsonResponse(routeOperationalStatus(7)))
    .mockResolvedValueOnce(jsonResponse({}))
  vi.stubGlobal('fetch', fetch)
  const signal = new AbortController().signal

  expect(await getActiveDisruptions({}, signal)).toHaveLength(2)
  expect(fetch).toHaveBeenNthCalledWith(1, '/api/transport/v1/disruptions/active', expect.objectContaining({ signal }))

  expect(await getActiveDisruptions({ type: ' detour ', routeId: 7, stopId: 23 }, signal)).toHaveLength(1)
  const url = new URL(fetch.mock.calls[1][0], 'https://transit.test')
  expect(Object.fromEntries(url.searchParams)).toEqual({
    type: 'detour',
    route_id: '7',
    stop_id: '23',
  })
  await expect(getActiveDisruptions()).rejects.toThrow('unexpected data')

  const status = await getServiceStatus(signal)
  expect(status.overall_status).toBe('disrupted')
  expect(status.active_disruptions).toHaveLength(2)
  expect(fetch).toHaveBeenNthCalledWith(4, '/api/transport/v1/service-status', expect.objectContaining({ signal }))
  await expect(getServiceStatus()).rejects.toThrow('Service status is unavailable')

  const affected = await getAffectedServices(signal)
  expect(affected.affected_routes).toHaveLength(2)
  expect(fetch).toHaveBeenNthCalledWith(6, '/api/transport/v1/disruptions/affected', expect.objectContaining({ signal }))
  await expect(getAffectedServices()).rejects.toThrow('Affected service information is unavailable')

  const routeStatus = await getRouteStatus(7, signal)
  expect(routeStatus.operational_status).toBe('detoured')
  expect(fetch).toHaveBeenNthCalledWith(8, '/api/transport/v1/routes/7/status', expect.objectContaining({ signal }))
  await expect(getRouteStatus(7)).rejects.toThrow('Route status is unavailable')
})
