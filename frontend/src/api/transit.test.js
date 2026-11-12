import { afterEach, expect, it, vi } from 'vitest'
import { getDepartures, getRoutes, getTimetable } from './transit.js'
import { jsonResponse, routes } from '../test/fixtures.js'

afterEach(() => vi.useRealTimers())

it('loads only active routes and passes the request cancellation signal', async () => {
  const fetch = vi.fn().mockResolvedValue(jsonResponse(routes))
  vi.stubGlobal('fetch', fetch)
  const signal = new AbortController().signal
  expect(await getRoutes(signal)).toHaveLength(3)
  expect(fetch).toHaveBeenCalledWith('/api/routes', expect.objectContaining({ signal }))
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
