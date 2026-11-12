import { describe, expect, it } from 'vitest'
import { dayOffset, departureEstimate, directionLabel, formatTime, getStops, groupRoutes, inPeriod, isServiceDate, serviceDate } from './transit.js'
import { departures, routes, timetable } from '../test/fixtures.js'

describe('agency date and time', () => {
  it('uses Toronto rather than the browser timezone around midnight and DST', () => {
    expect(serviceDate(new Date('2026-11-11T03:00:00Z'))).toBe('2026-11-10')
    expect(formatTime('2026-03-08T07:30:00Z')).toBe('03:30')
    expect(formatTime('2026-11-01T06:30:00Z')).toBe('01:30')
    expect(formatTime(null)).toBe('—')
    expect(dayOffset('2026-11-11T06:10:00Z', '2026-11-10')).toBe(1)
  })
  it('validates dates before requesting or formatting them', () => {
    for (const value of ['', '0000-01-01', '2026-02-29', '2026-1-01', '10000-01-01']) expect(isServiceDate(value)).toBe(false)
    expect(isServiceDate('2028-02-29')).toBe(true)
  })
})

describe('route and timetable presentation', () => {
  it('groups traversal directions and sorts route numbers naturally', () => {
    const grouped = groupRoutes([...routes, { ...routes[2], number: '10' }])
    expect(grouped.map((group) => group.number)).toEqual(['1', '7', '10', '90'])
    expect(grouped[0].directions.map((route) => route.id)).toEqual([1, 2])
    expect(directionLabel('direction_0')).toBe('Direction 1')
    expect(directionLabel('direction_1')).toBe('Direction 2')
  })
  it('deduplicates physical stops for filtering without altering trip occurrences', () => {
    const data = timetable(1, '2026-11-10')
    data.trips[0].stop_times.push({ ...data.trips[0].stop_times[0], route_stop_id: 12, stop_sequence: 3 })
    expect(getStops(data.trips)).toHaveLength(2)
    expect(data.trips[0].stop_times).toHaveLength(3)
    expect(inPeriod(data.trips[0], 'morning')).toBe(true)
    expect(inPeriod(data.trips[0], 'afternoon')).toBe(false)
  })
})

it('preserves schedules while handling absent, delayed, early and malformed estimates', () => {
  const [estimated, scheduled] = departures()
  const original = estimated.scheduled_departure_time
  expect(departureEstimate(estimated).delay).toBe(5)
  expect(estimated.scheduled_departure_time).toBe(original)
  expect(departureEstimate(scheduled)).toBe(null)
  expect(departureEstimate({ ...estimated, estimated_departure_time: 'unavailable' })).toBe(null)
  expect(departureEstimate({ ...estimated, estimated_departure_time: '2026-11-10T13:08:00Z' }).delay).toBe(-2)
})
