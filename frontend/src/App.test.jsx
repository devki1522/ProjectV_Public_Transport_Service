import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import App from './App.jsx'
import { departures, jsonResponse, routes, timetable } from './test/fixtures.js'
import { formatTime } from './lib/transit.js'

function mockTransit({ tripCount = 2, estimate = true, empty = false } = {}) {
  const fetch = vi.fn(async (path) => {
    const url = new URL(path, 'https://transit.test')
    if (url.pathname === '/api/routes') return jsonResponse(routes)
    if (url.pathname.endsWith('/schedule')) return jsonResponse(timetable(url.pathname.split('/')[3], url.searchParams.get('date'), empty ? 0 : tripCount))
    if (url.pathname === '/api/departures') {
      const data = empty ? [] : departures(url.searchParams.get('from'))
      return jsonResponse(estimate ? data : data.map((row) => ({ ...row, estimated_departure_time: null })))
    }
    throw new Error(`Unexpected test request: ${path}`)
  })
  vi.stubGlobal('fetch', fetch)
  return fetch
}

it('lets a resident choose a date and read ordered arrival/departure times', async () => {
  const fetch = mockTransit()
  const user = userEvent.setup()
  render(<App />)
  await screen.findByText('Trip 100')
  fireEvent.change(screen.getByLabelText('Travel date'), { target: { value: '2026-11-10' } })
  await screen.findByRole('heading', { name: 'Tuesday, November 10' })
  await screen.findByText('Trip 100')
  await user.click(screen.getByText('Trip 100').closest('summary'))
  const table = screen.getByRole('table', { name: /Stop times for trip 100, 2026-11-10/ })
  expect(within(table).getByRole('columnheader', { name: 'Arrival' })).toBeVisible()
  expect(within(table).getByRole('columnheader', { name: 'Departure' })).toBeVisible()
  expect(within(table).getByText('Central Station')).toBeVisible()
  expect(within(table).getByText('08:00')).toBeVisible()
  expect(within(table).getByText('08:02')).toBeVisible()
  expect(fetch.mock.calls.some(([path]) => path === '/api/routes/1/schedule?date=2026-11-10')).toBe(true)
})

it('searches public route numbers and requests the chosen traversal direction', async () => {
  const fetch = mockTransit()
  const user = userEvent.setup()
  render(<App />)
  await screen.findByRole('heading', { name: 'Queen-River' })
  expect(screen.queryByRole('button', { name: /Inactive/ })).not.toBeInTheDocument()
  await user.selectOptions(screen.getByLabelText('Travel direction'), '2')
  await waitFor(() => expect(fetch.mock.calls.some(([path]) => path.startsWith('/api/routes/2/schedule'))).toBe(true))
  const search = screen.getByRole('searchbox', { name: 'Search routes by number or name' })
  await user.type(search, 'king')
  await user.click(screen.getByRole('button', { name: /Route 7 King/ }))
  await screen.findByRole('heading', { name: 'King' })
  await waitFor(() => expect(fetch.mock.calls.some(([path]) => path.startsWith('/api/routes/7/schedule'))).toBe(true))
})

it('filters upcoming departures by stop and trip, and preserves the scheduled time beside an estimate', async () => {
  const fetch = mockTransit()
  const user = userEvent.setup()
  render(<App />)
  await screen.findByText('Trip 100')
  await user.click(screen.getByRole('tab', { name: 'Upcoming departures' }))
  await screen.findByText('Estimated')
  const rows = screen.getAllByRole('article')
  expect(within(rows[0]).getByText('5 min later')).toBeVisible()
  const url = new URL(fetch.mock.calls.find(([path]) => path.startsWith('/api/departures'))[0], 'https://transit.test')
  const original = departures(url.searchParams.get('from'))[0]
  expect(within(rows[0]).getByText(formatTime(original.scheduled_departure_time))).toBeVisible()
  expect(within(rows[0]).getByText(formatTime(original.estimated_departure_time))).toBeVisible()
  await user.selectOptions(screen.getByLabelText('Stop'), '21')
  await user.selectOptions(screen.getByLabelText('Trip'), '100')
  await user.selectOptions(screen.getByLabelText('Show departures'), '6')
  await waitFor(() => {
    const latest = new URL(fetch.mock.calls.filter(([path]) => path.startsWith('/api/departures')).at(-1)[0], 'https://transit.test')
    expect(latest.searchParams.get('stop_id')).toBe('21')
    expect(latest.searchParams.get('trip_id')).toBe('100')
    expect(Date.parse(latest.searchParams.get('until')) - Date.parse(latest.searchParams.get('from'))).toBe(6 * 3600_000)
  })
})

it('labels scheduled-only departures honestly and supports keyboard tab navigation', async () => {
  mockTransit({ estimate: false })
  const user = userEvent.setup()
  render(<App />)
  await screen.findByText('Trip 100')
  screen.getByRole('tab', { name: 'Timetable' }).focus()
  await user.keyboard('{ArrowRight}')
  expect(screen.getByRole('tab', { name: 'Upcoming departures' })).toHaveFocus()
  await screen.findByText('Showing scheduled times. Estimates will appear here when available.')
  expect(screen.queryByText('Estimated')).not.toBeInTheDocument()
  expect(screen.getAllByRole('article')).toHaveLength(2)
})

it('shows actionable empty timetables and departure windows', async () => {
  mockTransit({ empty: true })
  const user = userEvent.setup()
  render(<App />)
  await screen.findByRole('heading', { name: 'No trips for this selection' })
  await user.click(screen.getByRole('tab', { name: 'Upcoming departures' }))
  await screen.findByRole('heading', { name: 'No departures in this window' })
})

it('paginates long timetables and resets results when a time filter changes', async () => {
  mockTransit({ tripCount: 12 })
  const user = userEvent.setup()
  render(<App />)
  await screen.findByText('Showing 1–8 of 12 trips')
  await user.click(screen.getByRole('button', { name: 'Next trips' }))
  expect(screen.getByText('Showing 9–12 of 12 trips')).toBeVisible()
  await user.selectOptions(screen.getByLabelText('Time of day'), 'morning')
  expect(screen.getByText('Showing 1–8 of 12 trips')).toBeVisible()
})

it('recovers from a failed route request without displaying a false connection status', async () => {
  const fetch = mockTransit()
  fetch.mockResolvedValueOnce(jsonResponse({}, 500))
  const user = userEvent.setup()
  render(<App />)
  await screen.findByRole('alert')
  await user.click(screen.getByRole('button', { name: 'Try again' }))
  await screen.findByRole('heading', { name: 'Queen-River' })
})

it('handles an empty route catalog', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse([])))
  render(<App />)
  await screen.findByRole('heading', { name: 'No routes available' })
})

it('allows stop and trip filtering when today has no timetable but upcoming service exists', async () => {
  const fetch = mockTransit({ tripCount: 0 })
  const originalFetch = fetch.getMockImplementation()
  fetch.mockImplementation(async (path, options) => {
    const url = new URL(path, 'https://transit.test')
    if (url.pathname === '/api/departures' && url.searchParams.has('stop_id')) return jsonResponse([])
    return originalFetch(path, options)
  })
  const user = userEvent.setup()
  render(<App />)
  await screen.findByRole('heading', { name: 'No trips for this selection' })
  await user.click(screen.getByRole('tab', { name: 'Upcoming departures' }))
  await screen.findByText('Estimated')
  const stopSelect = screen.getByLabelText('Stop')
  expect(stopSelect).toBeEnabled()
  expect(within(stopSelect).getByRole('option', { name: 'Central Station' })).toBeInTheDocument()
  expect(within(screen.getByLabelText('Trip')).getByRole('option', { name: 'Trip 100 · upcoming' })).toBeInTheDocument()
  await user.selectOptions(stopSelect, '21')
  await screen.findByRole('heading', { name: 'No departures in this window' })
  expect(stopSelect).toBeEnabled()
  expect(stopSelect).toHaveValue('21')
  expect(within(stopSelect).getByRole('option', { name: 'Central Station' })).toBeInTheDocument()
})
