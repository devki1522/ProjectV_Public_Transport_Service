import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import App from './App.jsx'
import { departures, journeyResult, jsonResponse, routes, stopsCatalog, timetable } from './test/fixtures.js'
import { formatTime } from './lib/transit.js'

function mockTransit({
  tripCount = 2,
  estimate = true,
  empty = false,
  emptyJourneys = false,
  journeyEstimate = false,
  excludedDisruptions = [],
} = {}) {
  const fetch = vi.fn(async (path) => {
    const url = new URL(path, 'https://transit.test')
    if (url.pathname === '/api/routes') return jsonResponse(routes)
    if (url.pathname === '/api/stops') return jsonResponse(stopsCatalog)
    if (url.pathname === '/api/journeys') {
      return jsonResponse(journeyResult({
        origin: url.searchParams.get('origin') || 'Central Station',
        destination: url.searchParams.get('destination') || 'Fairway Station',
        empty: empty || emptyJourneys,
        estimate: journeyEstimate,
        excludedDisruptions,
      }))
    }
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
  await user.click(screen.getByRole('tab', { name: 'Upcoming Departures' }))
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
  expect(screen.getByRole('tab', { name: 'Upcoming Departures' })).toHaveFocus()
  await screen.findByText('Showing scheduled times. Estimates will appear here when available.')
  expect(screen.queryByText('Estimated')).not.toBeInTheDocument()
  expect(screen.getAllByRole('article')).toHaveLength(2)
})

it('shows actionable empty timetables and departure windows', async () => {
  mockTransit({ empty: true })
  const user = userEvent.setup()
  render(<App />)
  await screen.findByRole('heading', { name: 'No trips for this selection' })
  await user.click(screen.getByRole('tab', { name: 'Upcoming Departures' }))
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
  await user.click(screen.getByRole('tab', { name: 'Upcoming Departures' }))
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

it('searches between two locations and displays direct and connecting itinerary options', async () => {
  const fetch = mockTransit()
  const user = userEvent.setup()
  render(<App />)
  await screen.findByText('Trip 100')
  await user.click(screen.getByRole('tab', { name: 'Journey Search' }))
  await screen.findByRole('heading', { name: 'Journey Search' })
  expect(screen.queryByRole('heading', { name: 'Queen-River' })).not.toBeInTheDocument()
  expect(screen.queryByRole('searchbox', { name: 'Search routes by number or name' })).not.toBeInTheDocument()
  await screen.findByRole('heading', { name: 'Plan a transit journey' })

  const originInput = screen.getByLabelText('Origin')
  const destinationInput = screen.getByLabelText('Destination')
  await user.type(originInput, 'Fairway Station')
  await user.type(destinationInput, 'Central Station')
  await user.click(screen.getByRole('button', { name: 'Swap origin and destination' }))
  expect(originInput).toHaveValue('Central Station')
  expect(destinationInput).toHaveValue('Fairway Station')

  await user.selectOptions(screen.getByLabelText('Search window'), '2')
  await user.selectOptions(screen.getByLabelText('Max transfers'), '2')
  await user.click(screen.getByRole('button', { name: 'Find journeys' }))

  await screen.findByRole('heading', { name: 'Central Station to Fairway Station' })
  const cards = screen.getAllByRole('article')
  expect(cards).toHaveLength(3)
  expect(within(cards[0]).getByText('Direct')).toBeVisible()
  expect(within(cards[0]).getByText('20 min')).toBeVisible()
  expect(within(cards[1]).getByText('1 transfer')).toBeVisible()
  expect(within(cards[1]).getByText(/Transfer at University Terminal/)).toBeVisible()
  expect(within(cards[2]).getByText('2 transfers')).toBeVisible()
  expect(within(cards[2]).getByText('2 min walk')).toBeVisible()
  expect(within(cards[2]).getByText('Walk 2 min (~113 m)')).toBeVisible()

  const journeyCall = fetch.mock.calls.find(([path]) => path.startsWith('/api/journeys'))
  expect(journeyCall).toBeTruthy()
  const url = new URL(journeyCall[0], 'https://transit.test')
  expect(url.searchParams.get('origin')).toBe('Central Station')
  expect(url.searchParams.get('destination')).toBe('Fairway Station')
  expect(url.searchParams.get('max_transfers')).toBe('2')
  expect(url.searchParams.get('max_walk_meters')).toBe('600')
  expect(Date.parse(url.searchParams.get('until')) - Date.parse(url.searchParams.get('from'))).toBe(2 * 3600_000)
})

it('displays a meaningful message when no itinerary can be found between two locations', async () => {
  mockTransit({ emptyJourneys: true })
  const user = userEvent.setup()
  render(<App />)
  await screen.findByText('Trip 100')
  await user.click(screen.getByRole('tab', { name: 'Journey Search' }))
  await user.type(screen.getByLabelText('Origin'), 'Central Station')
  await user.type(screen.getByLabelText('Destination'), 'Fairway Station')
  await user.click(screen.getByRole('button', { name: 'Find journeys' }))

  await screen.findByRole('heading', { name: 'No journeys found' })
  expect(screen.getByText('No transit itineraries found between the selected origin and destination in this time window.')).toBeVisible()
})

it('validates missing or identical origin and destination before requesting journeys', async () => {
  const fetch = mockTransit()
  const user = userEvent.setup()
  render(<App />)
  await screen.findByText('Trip 100')
  screen.getByRole('tab', { name: 'Schedules & Departures' }).focus()
  await user.keyboard('{ArrowRight}')
  expect(screen.getByRole('tab', { name: 'Journey Search' })).toHaveFocus()
  await user.keyboard('{ArrowLeft}')
  expect(screen.getByRole('tab', { name: 'Schedules & Departures' })).toHaveFocus()
  await user.keyboard('{End}')
  expect(screen.getByRole('tab', { name: 'Journey Search' })).toHaveFocus()

  await user.click(screen.getByRole('button', { name: 'Find journeys' }))
  expect(screen.getByRole('alert')).toHaveTextContent('Enter both an origin and a destination to search.')

  await user.type(screen.getByLabelText('Origin'), 'Central Station')
  await user.type(screen.getByLabelText('Destination'), 'central station')
  await user.click(screen.getByRole('button', { name: 'Find journeys' }))
  expect(screen.getByRole('alert')).toHaveTextContent('Origin and destination must be different locations.')
  expect(fetch.mock.calls.some(([path]) => path.startsWith('/api/journeys'))).toBe(false)
})

it('shows ordered leg stops, scheduled-only times, and lets residents select an itinerary', async () => {
  mockTransit()
  const user = userEvent.setup()
  render(<App />)
  await screen.findByText('Trip 100')
  await user.click(screen.getByRole('tab', { name: 'Journey Search' }))
  await user.type(screen.getByLabelText('Origin'), 'Central Station')
  await user.type(screen.getByLabelText('Destination'), 'Fairway Station')
  await user.click(screen.getByRole('button', { name: 'Find journeys' }))

  await screen.findByRole('heading', { name: 'Central Station to Fairway Station' })
  expect(screen.getByText('Showing scheduled times. Estimates will appear here when available.')).toBeVisible()

  const cards = screen.getAllByRole('article')
  expect(within(cards[0]).getByText('Scheduled')).toBeVisible()
  expect(within(cards[0]).queryByText('Midway Stop')).not.toBeVisible()

  await user.click(within(cards[0]).getByText('View stops (3 stops)'))
  expect(within(cards[0]).getByText('Midway Stop')).toBeVisible()
  expect(within(cards[0]).getAllByText('08:15')).toHaveLength(2)

  await user.click(within(cards[0]).getByRole('button', { name: 'Select itinerary' }))
  const selectedButton = within(cards[0]).getByRole('button', { name: 'Selected' })
  expect(selectedButton).toHaveAttribute('aria-pressed', 'true')
  expect(cards[0]).toHaveClass('is-selected')

  const selectedBanner = screen.getByRole('status', { name: 'Selected itinerary' })
  expect(selectedBanner).toHaveTextContent('Selected itinerary (Option 1)')
  expect(selectedBanner).toHaveTextContent('Route 1')

  await user.click(within(selectedBanner).getByRole('button', { name: 'Clear selection' }))
  expect(screen.queryByRole('status', { name: 'Selected itinerary' })).not.toBeInTheDocument()
  expect(within(cards[0]).getByRole('button', { name: 'Select itinerary' })).toHaveAttribute('aria-pressed', 'false')
})

it('displays live journey estimates beside scheduled times and surfaces avoided disruptions', async () => {
  mockTransit({
    journeyEstimate: true,
    excludedDisruptions: [
      {
        id: 91,
        type: 'stop_closure',
        title: 'University Terminal closed for utility work',
        status: 'active',
        start_time: '2026-11-10T12:00:00Z',
      },
    ],
  })
  const user = userEvent.setup()
  render(<App />)
  await screen.findByText('Trip 100')
  await user.click(screen.getByRole('tab', { name: 'Journey Search' }))
  await user.type(screen.getByLabelText('Origin'), 'Central Station')
  await user.type(screen.getByLabelText('Destination'), 'Fairway Station')
  await user.click(screen.getByRole('button', { name: 'Find journeys' }))

  await screen.findByRole('heading', { name: 'Central Station to Fairway Station' })
  const disruptionBanner = screen.getByRole('status', { name: 'Excluded disruptions' })
  expect(disruptionBanner).toHaveTextContent('University Terminal closed for utility work')
  expect(disruptionBanner).toHaveTextContent('stop closure · active')

  expect(screen.getByText('Live estimates are shown where available alongside scheduled times.')).toBeVisible()

  const cards = screen.getAllByRole('article')
  expect(within(cards[0]).getByText('Estimated')).toBeVisible()
  expect(within(cards[0]).getAllByText('08:08').length).toBeGreaterThanOrEqual(2)
  expect(within(cards[0]).getAllByText('08:28').length).toBeGreaterThanOrEqual(2)
  expect(within(cards[0]).getByText('Scheduled 08:05 → 08:25')).toBeVisible()
  expect(within(cards[0]).getByText('Sched. 08:05')).toBeVisible()
  expect(within(cards[0]).getByText('Sched. 08:25')).toBeVisible()
  expect(within(cards[1]).getByText('Scheduled')).toBeVisible()
})
