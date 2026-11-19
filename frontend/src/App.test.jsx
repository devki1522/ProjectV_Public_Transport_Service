import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, it, vi } from 'vitest'

const leafletSpies = vi.hoisted(() => {
  const markers = []
  const maps = []
  const tileLayers = []
  const polylines = []
  const divIcons = []

  const L = {
    map: vi.fn((container, options) => {
      const instance = {
        container,
        options,
        fitBounds: vi.fn(),
        setView: vi.fn(),
        panTo: vi.fn(),
        remove: vi.fn(),
      }
      maps.push(instance)
      return instance
    }),
    tileLayer: vi.fn((url, options) => {
      const layer = {
        url,
        options,
        addTo: vi.fn().mockReturnThis(),
      }
      tileLayers.push(layer)
      return layer
    }),
    polyline: vi.fn((latLngs, options) => {
      const line = {
        latLngs,
        options,
        addTo: vi.fn().mockReturnThis(),
        remove: vi.fn(),
        getBounds: vi.fn(() => ({ isValid: () => true })),
      }
      polylines.push(line)
      return line
    }),
    divIcon: vi.fn((options) => {
      divIcons.push(options)
      return { options }
    }),
    marker: vi.fn((latLng, options) => {
      let currentLatLng = latLng
      let currentPopup = null
      const handlers = {}
      const instance = {
        options,
        addTo: vi.fn().mockReturnThis(),
        remove: vi.fn(),
        setLatLng: vi.fn((next) => {
          currentLatLng = next
          return instance
        }),
        getLatLng: vi.fn(() => currentLatLng),
        setIcon: vi.fn().mockReturnThis(),
        setZIndexOffset: vi.fn().mockReturnThis(),
        bindPopup: vi.fn((content) => {
          currentPopup = content
          return instance
        }),
        getPopup: vi.fn(() => currentPopup),
        setPopupContent: vi.fn((content) => {
          currentPopup = content
          return instance
        }),
        openPopup: vi.fn().mockReturnThis(),
        on: vi.fn((event, handler) => {
          handlers[event] = handler
          return instance
        }),
        trigger: (event) => handlers[event]?.(),
      }
      markers.push(instance)
      return instance
    }),
  }

  return { L, markers, maps, tileLayers, polylines, divIcons }
})

vi.mock('leaflet', () => ({ default: leafletSpies.L }))

import App from './App.jsx'
import {
  activeDisruptions,
  activeVehicles,
  affectedServicesSummary,
  departures,
  estimatedArrivals,
  journeyResult,
  jsonResponse,
  routeOperationalStatus,
  routeShape,
  routes,
  serviceStatusResponse,
  simulatorStatus,
  stopsCatalog,
  timetable,
} from './test/fixtures.js'
import { formatTime } from './lib/transit.js'

beforeEach(() => {
  localStorage.clear()
  delete document.documentElement.dataset.theme
  leafletSpies.markers.length = 0
  leafletSpies.maps.length = 0
  leafletSpies.tileLayers.length = 0
  leafletSpies.polylines.length = 0
  leafletSpies.divIcons.length = 0
  vi.clearAllMocks()
})

function mockTransit({
  tripCount = 2,
  estimate = true,
  empty = false,
  emptyJourneys = false,
  journeyEstimate = false,
  excludedDisruptions = [],
  vehicles = activeVehicles,
  simulator = simulatorStatus(),
  arrivalsByTrip = null,
  disruptions = [],
  serviceStatus = null,
} = {}) {
  const fetch = vi.fn(async (path) => {
    const url = new URL(path, 'https://transit.test')
    if (url.pathname === '/api/transport/v1/routes') return jsonResponse(routes)
    if (url.pathname === '/api/transport/v1/stops') return jsonResponse(stopsCatalog)
    if (url.pathname === '/api/transport/v1/vehicles/simulator') return jsonResponse(simulator)
    if (url.pathname === '/api/transport/v1/service-status') {
      return jsonResponse(
        serviceStatus ?? serviceStatusResponse({ disruptions: empty ? [] : disruptions }),
      )
    }
    if (url.pathname === '/api/transport/v1/disruptions/active') {
      const type = url.searchParams.get('type')
      const routeId = url.searchParams.get('route_id')
      const stopId = url.searchParams.get('stop_id')
      const list = (empty ? [] : disruptions).filter((item) => {
        if (item.status !== 'active') return false
        if (type && item.type !== type) return false
        if (routeId && !item.affected_routes?.some((r) => r.route_id === Number(routeId))) return false
        if (stopId && !item.affected_stops?.some((s) => s.stop_id === Number(stopId))) return false
        return true
      })
      return jsonResponse(list)
    }
    if (url.pathname === '/api/transport/v1/disruptions/affected') {
      return jsonResponse(affectedServicesSummary({ disruptions: empty ? [] : disruptions }))
    }
    if (url.pathname.startsWith('/api/transport/v1/routes/') && url.pathname.endsWith('/status')) {
      return jsonResponse(
        routeOperationalStatus(url.pathname.split('/')[5], {
          disruptions: empty ? [] : disruptions,
        }),
      )
    }
    if (url.pathname === '/api/transport/v1/vehicles') {
      const routeId = url.searchParams.get('route_id')
      const list = empty ? [] : routeId
        ? vehicles.filter((vehicle) => vehicle.route_id === Number(routeId))
        : vehicles
      return jsonResponse(list)
    }
    if (url.pathname.endsWith('/shape')) {
      return jsonResponse(routeShape(url.pathname.split('/')[5]))
    }
    if (url.pathname.startsWith('/api/transport/v1/trips/') && url.pathname.endsWith('/arrivals')) {
      const tripId = Number(url.pathname.split('/')[5])
      if (arrivalsByTrip && tripId in arrivalsByTrip) {
        return jsonResponse(arrivalsByTrip[tripId])
      }
      return jsonResponse(empty ? [] : estimatedArrivals(tripId))
    }
    if (url.pathname.startsWith('/api/transport/v1/stops/') && url.pathname.endsWith('/arrivals')) {
      return jsonResponse(estimatedArrivals(100).filter((arrival) => arrival.stop_id === Number(url.pathname.split('/')[5])))
    }
    if (url.pathname === '/api/transport/v1/journeys') {
      return jsonResponse(journeyResult({
        origin: url.searchParams.get('origin') || 'Central Station',
        destination: url.searchParams.get('destination') || 'Fairway Station',
        empty: empty || emptyJourneys,
        estimate: journeyEstimate,
        excludedDisruptions,
      }))
    }
    if (url.pathname.endsWith('/schedule')) return jsonResponse(timetable(url.pathname.split('/')[5], url.searchParams.get('date'), empty ? 0 : tripCount))
    if (url.pathname === '/api/transport/v1/departures') {
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
  fireEvent.click(screen.getByRole('tab', { name: 'Schedules & Departures' }))
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
  expect(fetch.mock.calls.some(([path]) => path === '/api/transport/v1/routes/1/schedule?date=2026-11-10')).toBe(true)
})

it('searches public route numbers and requests the chosen traversal direction', async () => {
  const fetch = mockTransit()
  const user = userEvent.setup()
  render(<App />)
  fireEvent.click(screen.getByRole('tab', { name: 'Schedules & Departures' }))
  await screen.findByRole('heading', { name: 'Queen-River' })
  expect(screen.queryByRole('button', { name: /Inactive/ })).not.toBeInTheDocument()
  await user.selectOptions(screen.getByLabelText('Travel direction'), '2')
  await waitFor(() => expect(fetch.mock.calls.some(([path]) => path.startsWith('/api/transport/v1/routes/2/schedule'))).toBe(true))
  const search = screen.getByRole('searchbox', { name: 'Search routes by number or name' })
  await user.type(search, 'king')
  await user.click(screen.getByRole('button', { name: /Route 7 King/ }))
  await screen.findByRole('heading', { name: 'King' })
  await waitFor(() => expect(fetch.mock.calls.some(([path]) => path.startsWith('/api/transport/v1/routes/7/schedule'))).toBe(true))
})

it('filters upcoming departures by stop and trip, and preserves the scheduled time beside an estimate', async () => {
  const fetch = mockTransit()
  const user = userEvent.setup()
  render(<App />)
  fireEvent.click(screen.getByRole('tab', { name: 'Schedules & Departures' }))
  await screen.findByText('Trip 100')
  await user.click(screen.getByRole('tab', { name: 'Upcoming Departures' }))
  await screen.findByText('Estimated')
  const rows = screen.getAllByRole('article')
  expect(within(rows[0]).getByText('5 min later')).toBeVisible()
  const url = new URL(fetch.mock.calls.find(([path]) => path.startsWith('/api/transport/v1/departures'))[0], 'https://transit.test')
  const original = departures(url.searchParams.get('from'))[0]
  expect(within(rows[0]).getByText(formatTime(original.scheduled_departure_time))).toBeVisible()
  expect(within(rows[0]).getByText(formatTime(original.estimated_departure_time))).toBeVisible()
  await user.selectOptions(screen.getByLabelText('Stop'), '21')
  await user.selectOptions(screen.getByLabelText('Trip'), '100')
  await user.selectOptions(screen.getByLabelText('Show departures'), '6')
  await waitFor(() => {
    const latest = new URL(fetch.mock.calls.filter(([path]) => path.startsWith('/api/transport/v1/departures')).at(-1)[0], 'https://transit.test')
    expect(latest.searchParams.get('stop_id')).toBe('21')
    expect(latest.searchParams.get('trip_id')).toBe('100')
    expect(Date.parse(latest.searchParams.get('until')) - Date.parse(latest.searchParams.get('from'))).toBe(6 * 3600_000)
  })
})

it('labels scheduled-only departures honestly and supports keyboard tab navigation', async () => {
  mockTransit({ estimate: false })
  const user = userEvent.setup()
  render(<App />)
  fireEvent.click(screen.getByRole('tab', { name: 'Schedules & Departures' }))
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
  fireEvent.click(screen.getByRole('tab', { name: 'Schedules & Departures' }))
  await screen.findByRole('heading', { name: 'No trips for this selection' })
  await user.click(screen.getByRole('tab', { name: 'Upcoming Departures' }))
  await screen.findByRole('heading', { name: 'No departures in this window' })
})

it('paginates long timetables and resets results when a time filter changes', async () => {
  mockTransit({ tripCount: 12 })
  const user = userEvent.setup()
  render(<App />)
  fireEvent.click(screen.getByRole('tab', { name: 'Schedules & Departures' }))
  await screen.findByText('Showing 1–8 of 12 trips')
  await user.click(screen.getByRole('button', { name: 'Next trips' }))
  expect(screen.getByText('Showing 9–12 of 12 trips')).toBeVisible()
  await user.selectOptions(screen.getByLabelText('Time of day'), 'morning')
  expect(screen.getByText('Showing 1–8 of 12 trips')).toBeVisible()
})

it('recovers from a failed route request without displaying a false connection status', async () => {
  const fetch = mockTransit()
  const originalFetch = fetch.getMockImplementation()
  let failed = false
  fetch.mockImplementation((path, options) => {
    if (path === '/api/transport/v1/routes' && !failed) {
      failed = true
      return Promise.resolve(jsonResponse({}, 500))
    }
    return originalFetch(path, options)
  })
  const user = userEvent.setup()
  render(<App />)
  fireEvent.click(screen.getByRole('tab', { name: 'Schedules & Departures' }))
  await screen.findByRole('alert')
  await user.click(screen.getByRole('button', { name: 'Try again' }))
  await screen.findByRole('heading', { name: 'Queen-River' })
})

it('handles an empty route catalog', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse([])))
  render(<App />)
  fireEvent.click(screen.getByRole('tab', { name: 'Schedules & Departures' }))
  await screen.findByRole('heading', { name: 'No routes available' })
})

it('allows stop and trip filtering when today has no timetable but upcoming service exists', async () => {
  const fetch = mockTransit({ tripCount: 0 })
  const originalFetch = fetch.getMockImplementation()
  fetch.mockImplementation(async (path, options) => {
    const url = new URL(path, 'https://transit.test')
    if (url.pathname === '/api/transport/v1/departures' && url.searchParams.has('stop_id')) return jsonResponse([])
    return originalFetch(path, options)
  })
  const user = userEvent.setup()
  render(<App />)
  fireEvent.click(screen.getByRole('tab', { name: 'Schedules & Departures' }))
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
  fireEvent.click(screen.getByRole('tab', { name: 'Schedules & Departures' }))
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

  const journeyCall = fetch.mock.calls.find(([path]) => path.startsWith('/api/transport/v1/journeys'))
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
  fireEvent.click(screen.getByRole('tab', { name: 'Schedules & Departures' }))
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
  fireEvent.click(screen.getByRole('tab', { name: 'Schedules & Departures' }))
  await screen.findByText('Trip 100')
  screen.getByRole('tab', { name: 'Schedules & Departures' }).focus()
  await user.keyboard('{ArrowRight}')
  expect(screen.getByRole('tab', { name: 'Journey Search' })).toHaveFocus()
  await user.keyboard('{ArrowLeft}')
  expect(screen.getByRole('tab', { name: 'Schedules & Departures' })).toHaveFocus()
  await user.keyboard('{End}')
  expect(screen.getByRole('tab', { name: 'Service Alerts' })).toHaveFocus()
  await user.keyboard('{Home}')
  expect(screen.getByRole('tab', { name: 'Live Map' })).toHaveFocus()
  await user.keyboard('{ArrowRight}{ArrowRight}')
  expect(screen.getByRole('tab', { name: 'Journey Search' })).toHaveFocus()

  await user.click(screen.getByRole('button', { name: 'Find journeys' }))
  expect(screen.getByRole('alert')).toHaveTextContent('Enter both an origin and a destination to search.')

  await user.type(screen.getByLabelText('Origin'), 'Central Station')
  await user.type(screen.getByLabelText('Destination'), 'central station')
  await user.click(screen.getByRole('button', { name: 'Find journeys' }))
  expect(screen.getByRole('alert')).toHaveTextContent('Origin and destination must be different locations.')
  expect(fetch.mock.calls.some(([path]) => path.startsWith('/api/transport/v1/journeys'))).toBe(false)
})

it('shows ordered leg stops, scheduled-only times, and lets residents select an itinerary', async () => {
  mockTransit()
  const user = userEvent.setup()
  render(<App />)
  fireEvent.click(screen.getByRole('tab', { name: 'Schedules & Departures' }))
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
  fireEvent.click(screen.getByRole('tab', { name: 'Schedules & Departures' }))
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

it('renders the Live Map with Leaflet markers, filters active vehicles by route and status, and shows upcoming stop ETAs (US 178, US 180, US 183)', async () => {
  const fetch = mockTransit()
  const user = userEvent.setup()
  const { unmount } = render(<App />)
  fireEvent.click(screen.getByRole('tab', { name: 'Schedules & Departures' }))
  await screen.findByText('Trip 100')

  await user.click(screen.getByRole('tab', { name: 'Live Map' }))
  await screen.findByRole('heading', { name: 'Live Map' })
  await screen.findByText(/Bus #2104/)
  expect(screen.getByText(/Bus #2207/)).toBeVisible()

  // Simulation notice banner appears honestly when simulator is running
  expect(screen.getByText(/Simulated fleet movement is active/i)).toBeVisible()

  // Leaflet map initialized on Waterloo Region with OSM tiles and bus markers
  expect(leafletSpies.L.map).toHaveBeenCalledWith(
    expect.any(HTMLElement),
    expect.objectContaining({ center: [43.4516, -80.4925], zoom: 12 }),
  )
  expect(leafletSpies.L.tileLayer).toHaveBeenCalledWith(
    'https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png',
    expect.objectContaining({ attribution: '© OpenStreetMap contributors' }),
  )
  expect(leafletSpies.markers).toHaveLength(2)

  // Tapping a bus marker displays its full route as well as upcoming arrivals.
  act(() => leafletSpies.markers.find((marker) => marker.options.title.includes('2104')).trigger('click'))
  await waitFor(() => {
    expect(fetch.mock.calls.some(([path]) => path === '/api/transport/v1/trips/100/arrivals')).toBe(true)
    expect(fetch.mock.calls.some(([path]) => path === '/api/transport/v1/routes/1/shape')).toBe(true)
  })
  const arrivalsList = await screen.findByRole('list', { name: 'Estimated arrivals for Trip 100' })
  expect(within(arrivalsList).getByText('Central Station')).toBeVisible()
  expect(within(arrivalsList).getByText('08:18')).toBeVisible()
  expect(within(arrivalsList).getByText('3 min later')).toBeVisible()
  expect(within(arrivalsList).getByText('Fairway Station')).toBeVisible()
  expect(within(arrivalsList).getByText('On time')).toBeVisible()

  // Filter active vehicles by route (US 178) and verify route polyline + fitBounds
  await user.selectOptions(screen.getByLabelText('Route'), '7')
  await waitFor(() => {
    expect(fetch.mock.calls.some(([path]) => path === '/api/transport/v1/vehicles?route_id=7')).toBe(true)
    expect(fetch.mock.calls.some(([path]) => path === '/api/transport/v1/routes/7/shape')).toBe(true)
  })
  await screen.findByText(/Bus #2207/)
  expect(screen.queryByText(/Bus #2104/)).not.toBeInTheDocument()
  expect(leafletSpies.L.polyline).toHaveBeenCalled()
  expect(leafletSpies.maps.at(-1).fitBounds).toHaveBeenCalledTimes(2)

  // Filtering status to Layover shows actionable empty state with "Show all routes" button
  await user.selectOptions(screen.getByLabelText('Status'), 'layover')
  await screen.findByRole('heading', { name: 'No active vehicles match this filter' })
  await user.click(screen.getAllByRole('button', { name: 'Show all routes' })[0])
  await screen.findByText(/Bus #2104/)

  // Pause and resume live auto-refresh toggle
  const liveToggle = screen.getByRole('button', { name: 'Live · 3s' })
  expect(liveToggle).toHaveAttribute('aria-pressed', 'true')
  await user.click(liveToggle)
  expect(screen.getByRole('button', { name: 'Paused' })).toHaveAttribute('aria-pressed', 'false')

  // Unmount cleans up Leaflet map instance
  unmount()
  expect(leafletSpies.maps[0].remove).toHaveBeenCalledTimes(1)
})

it('opens Live Map pre-filtered from the route Live buses shortcut and builds XSS-safe DOM popups', async () => {
  const unsafeVehicle = {
    ...activeVehicles[1],
    vehicle_number: '<img src=x onerror=alert(1)>',
    route_name: '<script>alert(1)</script>King',
  }
  const fetch = mockTransit({ vehicles: [activeVehicles[0], unsafeVehicle] })
  const user = userEvent.setup()
  render(<App />)
  fireEvent.click(screen.getByRole('tab', { name: 'Schedules & Departures' }))

  await screen.findByRole('heading', { name: 'Queen-River' })
  const search = screen.getByRole('searchbox', { name: 'Search routes by number or name' })
  await user.type(search, 'king')
  await user.click(screen.getByRole('button', { name: /Route 7 King/ }))
  await screen.findByRole('heading', { name: 'King' })

  await user.click(screen.getByRole('button', { name: 'Live buses' }))
  await screen.findByRole('heading', { name: 'Live Map' })
  expect(screen.getByLabelText('Route')).toHaveValue('7')
  await waitFor(() => {
    expect(fetch.mock.calls.some(([path]) => path === '/api/transport/v1/vehicles?route_id=7')).toBe(true)
  })

  // Verify popup content was constructed with safe DOM nodes (no injected img/script elements)
  await waitFor(() => expect(leafletSpies.markers.length).toBeGreaterThan(0))
  const popupNode = leafletSpies.markers.at(-1).bindPopup.mock.calls.at(-1)[0]
  expect(popupNode).toBeInstanceOf(HTMLElement)
  expect(popupNode.querySelector('img')).toBeNull()
  expect(popupNode.querySelector('script')).toBeNull()
  expect(popupNode.textContent).toContain('<img src=x onerror=alert(1)>')
  expect(popupNode.textContent).toContain('<script>alert(1)</script>King')
})

it('shows route operational status banner in Schedules and opens Service Alerts pre-filtered to that route (US 192)', async () => {
  mockTransit({ disruptions: activeDisruptions })
  const user = userEvent.setup()
  render(<App />)
  fireEvent.click(screen.getByRole('tab', { name: 'Schedules & Departures' }))

  await screen.findByRole('heading', { name: 'Queen-River' })
  const banner = await screen.findByRole('status', { name: 'Route service disruption notice' })
  expect(banner).toHaveTextContent('Route 1 status: Closed')
  expect(banner).toHaveTextContent('Central Station platform closure for emergency response')

  await user.click(within(banner).getByRole('button', { name: 'View service alerts' }))
  await screen.findByRole('heading', { name: 'Service Alerts' })
  expect(screen.getByLabelText('Route')).toHaveValue('1')

  const activeList = screen.getByRole('region', { name: 'Active disruptions' })
  expect(within(activeList).getByText('Central Station platform closure for emergency response')).toBeVisible()
  expect(within(activeList).queryByText('King St N watermain construction detour')).not.toBeInTheDocument()
})

it('filters active disruptions by type and route, lists affected routes and stops, and jumps to route schedule or live map (US 194, US 195)', async () => {
  mockTransit({ disruptions: activeDisruptions })
  const user = userEvent.setup()
  render(<App />)
  fireEvent.click(screen.getByRole('tab', { name: 'Schedules & Departures' }))

  await screen.findByText('Trip 100')
  await user.click(screen.getByRole('tab', { name: 'Service Alerts' }))
  await screen.findByRole('heading', { name: 'Service Alerts' })

  const activeSection = await screen.findByRole('region', { name: 'Active disruptions' })
  expect(within(activeSection).getByText('King St N watermain construction detour')).toBeVisible()
  expect(within(activeSection).getByText('Central Station platform closure for emergency response')).toBeVisible()
  expect(within(activeSection).getByText('Utilities')).toBeVisible()
  expect(within(activeSection).getByText('Emergency Services')).toBeVisible()

  const affectedSection = screen.getByRole('region', { name: 'Affected routes and stops' })
  expect(within(affectedSection).getByText('Affected routes (2)')).toBeVisible()
  expect(within(affectedSection).getByText('Affected stops (2)')).toBeVisible()
  expect(within(affectedSection).getByText('University Terminal')).toBeVisible()
  expect(within(affectedSection).getByText('Central Station')).toBeVisible()

  // Filter by disruption type: Detour
  await user.selectOptions(screen.getByLabelText('Disruption type'), 'detour')
  expect(within(activeSection).getByText('King St N watermain construction detour')).toBeVisible()
  expect(within(activeSection).queryByText('Central Station platform closure for emergency response')).not.toBeInTheDocument()
  expect(within(affectedSection).getByText('Affected routes (1)')).toBeVisible()

  // Filter to a type with no matches and use "Show all disruptions"
  await user.selectOptions(screen.getByLabelText('Disruption type'), 'cancellation')
  await screen.findByRole('heading', { name: 'No disruptions match this filter' })
  await user.click(screen.getByRole('button', { name: 'Show all disruptions' }))
  expect(within(activeSection).getByText('King St N watermain construction detour')).toBeVisible()
  expect(within(activeSection).getByText('Central Station platform closure for emergency response')).toBeVisible()

  // Jump from Affected routes -> Route 7 schedule
  const route7Item = within(affectedSection).getByText('King').closest('li')
  await user.click(within(route7Item).getByRole('button', { name: 'View schedule' }))
  await screen.findByRole('heading', { name: 'King' })

  // Return to Service Alerts and jump from Route 7 -> Live map
  await user.click(screen.getByRole('tab', { name: 'Service Alerts' }))
  const affectedAgain = await screen.findByRole('region', { name: 'Affected routes and stops' })
  const route7Again = within(affectedAgain).getByText('King').closest('li')
  await user.click(within(route7Again).getByRole('button', { name: 'Live map' }))
  await screen.findByRole('heading', { name: 'Live Map' })
  expect(screen.getByLabelText('Route')).toHaveValue('7')
})

it('shows the all-services-normal empty state in Service Alerts when there are no active disruptions', async () => {
  mockTransit({ disruptions: [] })
  const user = userEvent.setup()
  render(<App />)
  fireEvent.click(screen.getByRole('tab', { name: 'Schedules & Departures' }))

  await screen.findByText('Trip 100')
  await user.click(screen.getByRole('tab', { name: 'Service Alerts' }))
  await screen.findByRole('heading', { name: 'All services operating normally' })
})


it('colours connecting routes separately and shows estimates with scheduled context in the preview', async () => {
  mockTransit({ journeyEstimate: true })
  const user = userEvent.setup()
  render(<App />)
  await user.click(screen.getByRole('tab', { name: 'Journey Search' }))
  await user.type(screen.getByLabelText('Origin'), 'Central Station')
  await user.type(screen.getByLabelText('Destination'), 'Fairway Station')
  await user.click(screen.getByRole('button', { name: 'Find journeys' }))
  const preview = await screen.findByRole('region', { name: 'Journey map preview' })
  expect(within(preview).getAllByText('08:08').length).toBeGreaterThan(0)
  expect(within(preview).getAllByText('Scheduled 08:05').length).toBeGreaterThan(0)
  expect(within(preview).getAllByText('Arrive · Estimated').length).toBeGreaterThan(0)
  await user.click(within(screen.getAllByRole('article')[1]).getByRole('button', { name: 'Select itinerary' }))
  const legend = within(preview).getByRole('list', { name: 'Journey map legend and times' })
  const rows = within(legend).getAllByRole('listitem')
  expect(rows).toHaveLength(2)
  const colors = rows.map((row) => row.style.getPropertyValue('--route-color'))
  expect(new Set(colors).size).toBe(2)
  await waitFor(() => expect(leafletSpies.polylines.slice(-2).map((line) => line.options.color)).toEqual(colors))
  expect(within(preview).queryByText('Arrive · Estimated')).not.toBeInTheDocument()
  expect(within(preview).getAllByText('Arrive · Scheduled').length).toBeGreaterThan(0)
})

it('opens with the map first and draws the full selected route in schedules', async () => {
  mockTransit()
  render(<App />)
  expect(screen.getAllByRole('tab')[0]).toHaveTextContent('Live Map')
  expect(screen.getByRole('tab', { name: 'Live Map' })).toHaveAttribute('aria-selected', 'true')
  fireEvent.click(screen.getByRole('tab', { name: 'Schedules & Departures' }))
  await waitFor(() => expect(leafletSpies.L.polyline).toHaveBeenCalledWith(
    [routeShape(1).coordinates.map((point) => [point.lat, point.lng])], expect.any(Object),
  ))
})


it('persists a theme choice across reloads and exposes a system appearance option', async () => {
  mockTransit()
  const user = userEvent.setup()
  const mounted = render(<App />)
  await user.selectOptions(screen.getByRole('combobox', { name: 'Appearance' }), 'dark')
  expect(document.documentElement).toHaveAttribute('data-theme', 'dark')
  expect(JSON.parse(localStorage.getItem('transit-theme'))).toBe('dark')
  mounted.unmount()
  render(<App />)
  expect(screen.getByRole('combobox', { name: 'Appearance' })).toHaveValue('dark')
  await user.selectOptions(screen.getByRole('combobox', { name: 'Appearance' }), 'light')
  expect(document.documentElement).toHaveAttribute('data-theme', 'light')
  await user.selectOptions(screen.getByRole('combobox', { name: 'Appearance' }), 'system')
  expect(screen.getByRole('combobox', { name: 'Appearance' })).toHaveValue('system')
})

it('saves stops locally, restores them and loads their estimated arrivals without an account', async () => {
  const fetch = mockTransit()
  const user = userEvent.setup()
  const mounted = render(<App />)
  fireEvent.click(screen.getByRole('tab', { name: 'Schedules & Departures' }))
  await user.click(screen.getByRole('button', { name: 'Saved stops' }))
  await user.type(screen.getByRole('searchbox', { name: 'Search stops to save' }), 'Central')
  await user.click(await screen.findByRole('button', { name: 'Save Central Station' }))
  expect(JSON.parse(localStorage.getItem('transit-saved-stops'))).toEqual([{ id: 21, name: 'Central Station' }])
  mounted.unmount()
  render(<App />)
  fireEvent.click(screen.getByRole('tab', { name: 'Schedules & Departures' }))
  await user.click(screen.getByRole('button', { name: /Saved stops/ }))
  const pins = screen.getByRole('list', { name: 'Pinned stops' })
  await user.click(within(pins).getByRole('button', { name: /Central Station.*View arrivals/ }))
  const arrivals = await screen.findByRole('region', { name: 'Arrivals at Central Station' })
  expect(await within(arrivals).findByText('08:18')).toBeVisible()
  expect(within(arrivals).getByText('Estimated')).toBeVisible()
  expect(fetch.mock.calls.some(([path]) => path === '/api/transport/v1/stops/21/arrivals')).toBe(true)
  await user.click(screen.getByRole('button', { name: 'Remove Central Station from saved stops' }))
  expect(JSON.parse(localStorage.getItem('transit-saved-stops'))).toEqual([])
})

it('pins a stop in the primary board and draws a departure route with keyboard selection', async () => {
  mockTransit()
  const user = userEvent.setup()
  render(<App />)
  await user.click(await screen.findByRole('button', { name: 'Pin Central Station' }))
  expect(JSON.parse(localStorage.getItem('transit-saved-stops'))).toEqual([{ id: 21, name: 'Central Station' }])
  const pins = screen.getByRole('group', { name: 'Pinned stops' })
  expect(within(pins).getByRole('button', { name: 'Central Station' })).toHaveAttribute('aria-pressed', 'true')
  const board = await screen.findByRole('list', { name: 'Departures from Central Station' })
  const row = await within(board).findByRole('button', { name: /Route 1 to Fairway Station/ })
  row.focus()
  await user.keyboard('{Enter}')
  expect(row).toHaveAttribute('aria-pressed', 'true')
  await waitFor(() => expect(leafletSpies.L.polyline).toHaveBeenCalled())
  await user.click(screen.getByRole('button', { name: 'Unpin Central Station' }))
  expect(JSON.parse(localStorage.getItem('transit-saved-stops'))).toEqual([])
})

it('announces offline mode and retains keyboard-operable map and list selection', async () => {
  mockTransit()
  const user = userEvent.setup()
  render(<App />)
  vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(false)
  fireEvent(window, new Event('offline'))
  expect(screen.getByText(/offline. Live positions and arrivals may be out of date/)).toBeInTheDocument()
  const switcher = screen.getByRole('group', { name: 'Map or vehicle list' })
  const listButton = within(switcher).getByRole('button', { name: 'Departures' })
  listButton.focus()
  await user.keyboard('{Enter}')
  expect(listButton).toHaveAttribute('aria-pressed', 'true')
  expect(within(switcher).getByRole('button', { name: 'Map' })).toHaveAttribute('aria-pressed', 'false')
  vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(true)
  fireEvent(window, new Event('online'))
  expect(screen.queryByText(/offline. Live positions and arrivals may be out of date/)).not.toBeInTheDocument()
})
