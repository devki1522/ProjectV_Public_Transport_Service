import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import App from './App.jsx'
import { sanitizeRedirectPath } from './lib/routerState.js'
import {
  activeDisruptions as fixtureActiveDisruptions,
  activeVehicles as fixtureVehicles,
  allDisruptions as fixtureAllDisruptions,
  jsonResponse,
  routes as fixtureRoutes,
  serviceStatusResponse,
  simulatorStatus,
} from './test/fixtures.js'

const adminSession = {
  isAdmin: true,
  user: { id: 'dev-transit-admin-001', username: 'transit.admin', roles: ['TransitAdministrator'] },
  expiresAtUtc: new Date(Date.now() + 15 * 60_000).toISOString(),
  csrfToken: 'csrf-admin-token-123',
}

const normalUserSession = {
  isAdmin: false,
  user: { id: 'dev-transit-user-001', username: 'transit.user', roles: ['User'] },
  expiresAtUtc: new Date(Date.now() + 15 * 60_000).toISOString(),
  csrfToken: 'csrf-user-token-456',
}

const dashboardData = {
  total_routes: 4,
  active_routes: 3,
  inactive_routes: 1,
}

it('rejects unsafe redirect paths and keeps safe internal paths', () => {
  expect(sanitizeRedirectPath('/admin/transit')).toBe('/admin/transit')
  expect(sanitizeRedirectPath('/admin?tab=1')).toBe('/admin?tab=1')
  expect(sanitizeRedirectPath('https://evil.example')).toBe('/admin')
  expect(sanitizeRedirectPath('//evil.example/admin')).toBe('/admin')
  expect(sanitizeRedirectPath('/\\evil.example')).toBe('/admin')
  expect(sanitizeRedirectPath('javascript:alert(1)')).toBe('/admin')
  expect(sanitizeRedirectPath('/login')).toBe('/admin')
  expect(sanitizeRedirectPath('/unauthorized')).toBe('/admin')
})

it('waits for session restoration on protected routes and redirects unauthenticated users to login', async () => {
  let resolveMe
  const fetchMock = vi.fn((path) => {
    if (path === '/api/auth/me') {
      return new Promise((resolve) => {
        resolveMe = resolve
      })
    }
    throw new Error(`Unexpected request: ${path}`)
  })
  vi.stubGlobal('fetch', fetchMock)

  render(<App initialPath="/admin" />)
  expect(screen.getByRole('status', { name: 'Restoring administrator session' })).toBeInTheDocument()
  expect(screen.queryByRole('heading', { name: 'Administrator sign in' })).not.toBeInTheDocument()

  resolveMe(jsonResponse({ error: 'authentication required' }, 401))

  await screen.findByRole('heading', { name: 'Administrator sign in' })
  expect(window.location.pathname).toBe('/login')
  expect(window.location.search).toBe('?next=%2Fadmin')
})

it('signs in an administrator, reaches dashboard, and falls back safely on unsafe next path', async () => {
  const fetchMock = vi.fn(async (path, options = {}) => {
    if (path === '/api/auth/me') return jsonResponse({ error: 'unauthenticated' }, 401)
    if (path === '/api/auth/login') {
      expect(JSON.parse(options.body)).toEqual({
        username: 'transit.admin',
        password: 'admin-password',
        rememberMe: false,
      })
      return jsonResponse(adminSession, 200)
    }
    if (path === '/api/admin/dashboard') return jsonResponse(dashboardData, 200)
    throw new Error(`Unexpected request: ${path}`)
  })
  vi.stubGlobal('fetch', fetchMock)

  const user = userEvent.setup()
  render(<App initialPath="/login?next=https://evil.example" />)

  await waitFor(() => expect(screen.getByLabelText('Username')).toBeEnabled())
  await user.type(screen.getByLabelText('Username'), 'transit.admin')
  await user.type(screen.getByLabelText('Password'), 'admin-password')
  await user.click(screen.getByRole('button', { name: 'Sign in' }))

  await screen.findByRole('heading', { name: 'Transit dashboard' })
  expect(window.location.pathname).toBe('/admin')
  expect(screen.getByText('4')).toBeVisible()
  expect(screen.getByText('3')).toBeVisible()
  expect(screen.getByText('1')).toBeVisible()
  expect(screen.getByText('transit.admin')).toBeVisible()
})

it('shows invalid credential and provider-unavailable messages on login without leaving login', async () => {
  const fetchMock = vi.fn(async (path) => {
    if (path === '/api/auth/me') return jsonResponse({}, 401)
    if (path === '/api/auth/login') return jsonResponse({ error: 'invalid username or password' }, 401)
    throw new Error(`Unexpected request: ${path}`)
  })
  vi.stubGlobal('fetch', fetchMock)

  const user = userEvent.setup()
  render(<App initialPath="/login" />)

  const passwordInput = screen.getByLabelText('Password')
  await waitFor(() => expect(screen.getByLabelText('Username')).toBeEnabled())
  await user.type(screen.getByLabelText('Username'), 'transit.admin')
  await user.type(passwordInput, 'wrong-pw')
  await user.click(screen.getByRole('button', { name: 'Sign in' }))

  await screen.findByText('Invalid username or password.')
  expect(window.location.pathname).toBe('/login')
  expect(passwordInput).toHaveValue('')

  fetchMock.mockImplementationOnce(async (path) => {
    if (path === '/api/auth/login') return jsonResponse({ error: 'unavailable' }, 503)
    throw new Error(`Unexpected request: ${path}`)
  })
  await user.type(passwordInput, 'any-pw')
  await user.click(screen.getByRole('button', { name: 'Sign in' }))
  await screen.findByText(/provider is currently unavailable/i)
})

it('routes a signed-in normal user to the unauthorized page', async () => {
  const fetchMock = vi.fn(async (path) => {
    if (path === '/api/auth/me') return jsonResponse({}, 401)
    if (path === '/api/auth/login') return jsonResponse(normalUserSession, 200)
    throw new Error(`Unexpected request: ${path}`)
  })
  vi.stubGlobal('fetch', fetchMock)

  const user = userEvent.setup()
  render(<App initialPath="/login" />)

  await waitFor(() => expect(screen.getByLabelText('Username')).toBeEnabled())
  await user.type(screen.getByLabelText('Username'), 'transit.user')
  await user.type(screen.getByLabelText('Password'), 'user-pw')
  await user.click(screen.getByRole('button', { name: 'Sign in' }))

  await screen.findByRole('heading', { name: 'Access denied' })
  expect(window.location.pathname).toBe('/unauthorized')
  expect(screen.getByText(/transit\.user/)).toBeVisible()
})

it('shows dashboard error state and retries cleanly', async () => {
  let failDashboard = true
  const fetchMock = vi.fn(async (path) => {
    if (path === '/api/auth/me') return jsonResponse(adminSession, 200)
    if (path === '/api/admin/dashboard') {
      if (failDashboard) return jsonResponse({ error: 'db down' }, 500)
      return jsonResponse(dashboardData, 200)
    }
    throw new Error(`Unexpected request: ${path}`)
  })
  vi.stubGlobal('fetch', fetchMock)

  const user = userEvent.setup()
  render(<App initialPath="/admin" />)

  await screen.findByRole('alert')
  failDashboard = false
  await user.click(screen.getByRole('button', { name: 'Try again' }))
  await screen.findByText('4')
})

it('supports creating, editing, and deactivating routes with CSRF headers and confirmation', async () => {
  let catalog = [...fixtureRoutes]
  const mutationCalls = []

  const fetchMock = vi.fn(async (path, options = {}) => {
    const method = options.method || 'GET'
    if (path === '/api/auth/me') return jsonResponse(adminSession, 200)
    if (path === '/api/routes' && method === 'GET') return jsonResponse(catalog, 200)
    if (path === '/api/routes' && method === 'POST') {
      mutationCalls.push({ path, method, headers: options.headers, body: JSON.parse(options.body) })
      const created = { id: 99, ...JSON.parse(options.body), is_active: true }
      catalog = [...catalog, created]
      return jsonResponse(created, 201)
    }
    if (path === '/api/routes/7' && method === 'PATCH') {
      mutationCalls.push({ path, method, headers: options.headers, body: JSON.parse(options.body) })
      const patch = JSON.parse(options.body)
      catalog = catalog.map((r) => (r.id === 7 ? { ...r, ...patch } : r))
      return jsonResponse(catalog.find((r) => r.id === 7), 200)
    }
    if (path === '/api/routes/7' && method === 'DELETE') {
      mutationCalls.push({ path, method, headers: options.headers })
      catalog = catalog.map((r) => (r.id === 7 ? { ...r, is_active: false } : r))
      return { ok: true, status: 204, json: async () => ({}) }
    }
    throw new Error(`Unexpected request: ${method} ${path}`)
  })
  vi.stubGlobal('fetch', fetchMock)

  const user = userEvent.setup()
  render(<App initialPath="/admin/transit" />)

  // Inactive route 90 is visible in admin route catalog
  await screen.findByText('ID 90')
  expect(screen.getAllByText('Inactive')).toHaveLength(2)

  // Recoverable 409 conflict preserves form values
  fetchMock.mockImplementationOnce(async (path, options = {}) => {
    if (path === '/api/routes' && options.method === 'POST') {
      return jsonResponse({ error: 'a route with that number and direction already exists' }, 409)
    }
    throw new Error(`Unexpected request: ${options.method} ${path}`)
  })
  await user.type(screen.getByLabelText('Route number'), '7')
  await user.type(screen.getByLabelText('Route name'), 'King')
  await user.click(screen.getByRole('button', { name: 'Create route' }))
  await screen.findByText(/already exists/i)
  expect(screen.getByLabelText('Route number')).toHaveValue('7')
  expect(screen.getByLabelText('Route name')).toHaveValue('King')
  await user.clear(screen.getByLabelText('Route number'))
  await user.clear(screen.getByLabelText('Route name'))

  // Create a route
  await user.type(screen.getByLabelText('Route number'), '12')
  await user.type(screen.getByLabelText('Route name'), 'University')
  await user.click(screen.getByRole('button', { name: 'Create route' }))
  await screen.findByText('Created route 12 (University).')
  expect(screen.getByText('University')).toBeVisible()

  // Edit route 7
  const kingRow = screen.getByText('King').closest('tr')
  await user.click(within(kingRow).getByRole('button', { name: 'Edit' }))
  const editName = within(screen.getByLabelText('Edit selected route')).getByLabelText('Route name')
  await user.clear(editName)
  await user.type(editName, 'King Express')
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  await screen.findByText('Updated route 7 (King Express).')

  // Deactivate route 7 with confirmation explaining schedules are preserved
  const updatedRow = screen.getByText('King Express').closest('tr')
  await user.click(within(updatedRow).getByRole('button', { name: 'Deactivate' }))
  const dialog = screen.getByRole('alertdialog')
  expect(within(dialog).getByText(/preserving all related schedules/i)).toBeVisible()
  await user.click(within(dialog).getByRole('button', { name: 'Confirm deactivate' }))
  await screen.findByText(/Deactivated route 7/)

  expect(mutationCalls).toHaveLength(3)
  for (const call of mutationCalls) {
    expect(call.headers['X-CSRF-Token']).toBe(adminSession.csrfToken)
  }
})

it('clears session on backend 401 and displays access denial on backend 403', async () => {
  let nextDashStatus = 401
  const fetchMock = vi.fn(async (path) => {
    if (path === '/api/auth/me') return jsonResponse(adminSession, 200)
    if (path === '/api/admin/dashboard') return jsonResponse({ error: 'denied' }, nextDashStatus)
    throw new Error(`Unexpected request: ${path}`)
  })
  vi.stubGlobal('fetch', fetchMock)

  const { unmount } = render(<App initialPath="/admin" />)
  await screen.findByRole('heading', { name: 'Administrator sign in' })
  unmount()

  nextDashStatus = 403
  render(<App initialPath="/admin" />)
  await screen.findByRole('heading', { name: 'Access denied' })
})

it('logs out with CSRF header, removes protected content, and returns to login', async () => {
  const logoutCalls = []
  const fetchMock = vi.fn(async (path, options = {}) => {
    if (path === '/api/auth/me') return jsonResponse(adminSession, 200)
    if (path === '/api/admin/dashboard') return jsonResponse(dashboardData, 200)
    if (path === '/api/auth/logout') {
      logoutCalls.push(options.headers)
      return jsonResponse({ status: 'logged_out' }, 200)
    }
    throw new Error(`Unexpected request: ${path}`)
  })
  vi.stubGlobal('fetch', fetchMock)

  const user = userEvent.setup()
  render(<App initialPath="/admin" />)
  await screen.findByRole('heading', { name: 'Transit dashboard' })

  await user.click(screen.getByRole('button', { name: 'Sign out' }))
  await waitFor(() => expect(screen.getByRole('heading', { name: 'Administrator sign in' })).toBeVisible())
  expect(screen.queryByRole('heading', { name: 'Transit dashboard' })).not.toBeInTheDocument()
  expect(logoutCalls[0]['X-CSRF-Token']).toBe(adminSession.csrfToken)
})

for (const failure of ['network', 403, 500]) {
  it(`keeps sign out retryable when logout fails with ${failure}`, async () => {
    let fail = true
    vi.stubGlobal('fetch', vi.fn(async (path) => {
      if (path === '/api/auth/me') return jsonResponse(adminSession, 200)
      if (path === '/api/admin/dashboard') return jsonResponse(dashboardData, 200)
      if (path === '/api/auth/logout') {
        if (fail && failure === 'network') throw new Error('offline')
        return jsonResponse({}, fail ? failure : 200)
      }
      throw new Error(`Unexpected request: ${path}`)
    }))
    const user = userEvent.setup()
    render(<App initialPath="/admin" />)
    await screen.findByRole('heading', { name: 'Transit dashboard' })
    await user.click(screen.getByRole('button', { name: 'Sign out' }))
    await screen.findByText(/Sign out could not be confirmed/)
    expect(window.location.pathname).toBe('/admin')
    expect(screen.getByRole('heading', { name: 'Transit dashboard' })).toBeVisible()
    fail = false
    await user.click(screen.getByRole('button', { name: 'Sign out' }))
    await screen.findByRole('heading', { name: 'Administrator sign in' })
  })
}

it('uses the backend capability for a configured administrator role', async () => {
  const customSession = { ...adminSession, user: { ...adminSession.user, roles: ['TransportOperator'] } }
  vi.stubGlobal('fetch', vi.fn(async (path) => {
    if (path === '/api/auth/me') return jsonResponse(customSession, 200)
    if (path === '/api/admin/dashboard') return jsonResponse(dashboardData, 200)
    throw new Error(`Unexpected request: ${path}`)
  }))
  render(<App initialPath="/admin" />)
  await screen.findByRole('heading', { name: 'Transit dashboard' })
})

it('denies the UI when the backend capability is false even if the default role is present', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => jsonResponse({ ...adminSession, isAdmin: false }, 200)))
  render(<App initialPath="/admin" />)
  await screen.findByRole('heading', { name: 'Access denied' })
})

it('waits for session restoration before enabling the sign-in form', async () => {
  let resolveMe
  vi.stubGlobal('fetch', vi.fn(() => new Promise((resolve) => { resolveMe = resolve })))
  render(<App initialPath="/login" />)
  expect(screen.getByLabelText('Username')).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Sign in' })).toBeDisabled()
  resolveMe(jsonResponse({}, 401))
  await waitFor(() => expect(screen.getByLabelText('Username')).toBeEnabled())
})

it('returns an expired administrator session to login without waiting for a mutation', async () => {
  vi.useFakeTimers()
  try {
    const session = { ...adminSession, expiresAtUtc: new Date(Date.now() + 15 * 60_000).toISOString() }
    vi.stubGlobal('fetch', vi.fn(async (path) => {
      if (path === '/api/auth/me') return jsonResponse(session, 200)
      if (path === '/api/admin/dashboard') return jsonResponse(dashboardData, 200)
      throw new Error(`Unexpected request: ${path}`)
    }))
    await act(async () => { render(<App initialPath="/admin" />) })
    expect(screen.getByRole('heading', { name: 'Transit dashboard' })).toBeVisible()
    await act(async () => { await vi.advanceTimersByTimeAsync(15 * 60_000) })
    expect(screen.getByRole('heading', { name: 'Administrator sign in' })).toBeVisible()
  } finally {
    vi.useRealTimers()
  }
})

it('keeps the resident page public when a previous administrator session expires', async () => {
  vi.useFakeTimers()
  try {
    const session = { ...adminSession, expiresAtUtc: new Date(Date.now() + 15 * 60_000).toISOString() }
    vi.stubGlobal('fetch', vi.fn(async (path) => {
      if (path === '/api/auth/me') return jsonResponse(session, 200)
      if (path === '/api/admin/dashboard') return jsonResponse(dashboardData, 200)
      if (path === '/api/routes') return jsonResponse([], 200)
      throw new Error(`Unexpected request: ${path}`)
    }))
    await act(async () => { render(<App initialPath="/admin" />) })
    await act(async () => { fireEvent.click(screen.getByRole('link', { name: 'Public schedules' })) })
    await act(async () => { await vi.advanceTimersByTimeAsync(15 * 60_000) })
    expect(window.location.pathname).toBe('/')
    expect(screen.getByRole('heading', { name: 'Schedules & Departures' })).toBeVisible()
  } finally {
    vi.useRealTimers()
  }
})

it('supports /admin/vehicles US 177 location updates with presets, validation, CSRF headers, and simulator controls', async () => {
  let fleet = [fixtureVehicles[0]]
  let sim = simulatorStatus({ running: false, speed_multiplier: 1, total_updates_sent: 10 })
  const mutationCalls = []

  const fetchMock = vi.fn(async (path, options = {}) => {
    const method = options.method || 'GET'
    if (path === '/api/auth/me') return jsonResponse(adminSession, 200)
    if (path === '/api/routes' && method === 'GET') return jsonResponse(fixtureRoutes, 200)
    if (path === '/api/vehicles' && method === 'GET') return jsonResponse(fleet, 200)
    if (path === '/api/vehicles/simulator' && method === 'GET') return jsonResponse(sim, 200)
    if (path === '/api/vehicles/locations' && method === 'POST') {
      const body = JSON.parse(options.body)
      mutationCalls.push({ path, method, headers: options.headers, body })
      const route = fixtureRoutes.find((r) => r.id === body.route_id) ?? fixtureRoutes[2]
      const created = {
        vehicle_id: 901,
        vehicle_number: body.vehicle_number,
        is_accessible: body.is_accessible,
        route_id: route.id,
        route_number: route.number,
        route_name: route.name,
        direction: route.direction,
        trip_id: body.trip_id ?? null,
        latitude: body.latitude,
        longitude: body.longitude,
        heading: body.heading ?? null,
        speed_kmh: body.speed_kmh ?? null,
        status: body.status,
        source_service: 'transit.admin',
        is_simulated: false,
        recorded_at: '2026-11-10T13:15:00Z',
        received_at: '2026-11-10T13:15:00Z',
      }
      fleet = [...fleet, created]
      return jsonResponse(created, 201)
    }
    if (path === '/api/admin/vehicles/simulator/start' && method === 'POST') {
      const body = options.body ? JSON.parse(options.body) : {}
      mutationCalls.push({ path, method, headers: options.headers, body })
      sim = { ...sim, running: true, speed_multiplier: body.speed_multiplier ?? 1 }
      return jsonResponse(sim, 200)
    }
    if (path === '/api/admin/vehicles/simulator/step' && method === 'POST') {
      mutationCalls.push({ path, method, headers: options.headers })
      sim = { ...sim, total_updates_sent: sim.total_updates_sent + 8 }
      return jsonResponse(sim, 200)
    }
    if (path === '/api/admin/vehicles/simulator/stop' && method === 'POST') {
      mutationCalls.push({ path, method, headers: options.headers })
      sim = { ...sim, running: false }
      return jsonResponse(sim, 200)
    }
    if (path === '/api/admin/vehicles/simulator/reset' && method === 'POST') {
      mutationCalls.push({ path, method, headers: options.headers })
      sim = { ...sim, running: false, total_updates_sent: 0 }
      fleet = fleet.filter((v) => !v.is_simulated)
      return jsonResponse(sim, 200)
    }
    throw new Error(`Unexpected request: ${method} ${path}`)
  })
  vi.stubGlobal('fetch', fetchMock)

  const user = userEvent.setup()
  render(<App initialPath="/admin/vehicles" />)

  await screen.findByRole('heading', { name: 'Live fleet & vehicle tracking' })
  await screen.findByText('Bus #2104')
  expect(screen.getByRole('link', { name: 'Live fleet' })).toHaveAttribute('aria-current', 'page')

  // Validation blocks submission when vehicle number or coordinates are missing
  await user.click(screen.getByRole('button', { name: 'Submit location update' }))
  expect(screen.getByRole('alert')).toHaveTextContent(/Vehicle number is required/i)

  // Fill location update using Waterloo Region 1-click preset
  await user.selectOptions(screen.getByLabelText('Route'), '7')
  await user.type(screen.getByLabelText('Vehicle number'), 'BUS-707')
  await user.type(screen.getByLabelText('Trip ID (optional)'), '200')
  await user.click(screen.getByRole('button', { name: 'University of Waterloo' }))
  expect(screen.getByLabelText('Latitude')).toHaveValue(43.4723)
  expect(screen.getByLabelText('Longitude')).toHaveValue(-80.5449)
  await user.type(screen.getByLabelText('Heading (°)'), '180')
  await user.type(screen.getByLabelText('Speed (km/h)'), '38')
  await user.selectOptions(screen.getByLabelText('Status'), 'delayed')
  await user.click(screen.getByRole('button', { name: 'Submit location update' }))

  await screen.findByText('Submitted location update for bus #BUS-707 on Route 7.')
  await screen.findByText('Bus #BUS-707')

  // Exercise simulator controls: Start at 2x, Step once, Pause, Reset
  await user.selectOptions(screen.getByLabelText('Speed'), '2')
  await user.click(screen.getByRole('button', { name: 'Start simulator' }))
  await screen.findByText('Live bus simulator started at 2x speed.')

  await user.click(screen.getByRole('button', { name: 'Step once' }))
  await screen.findByText(/Advanced simulated vehicles by one tick/)

  await user.click(screen.getByRole('button', { name: 'Pause simulator' }))
  await screen.findByText('Live bus simulator paused.')

  await user.click(screen.getByRole('button', { name: 'Reset simulated buses' }))
  await screen.findByText('Reset simulated bus positions.')
  await waitFor(() => expect(screen.queryByText('Bus #2104')).not.toBeInTheDocument())
  expect(screen.getByText('Bus #BUS-707')).toBeVisible()

  expect(mutationCalls).toHaveLength(5)
  expect(mutationCalls[0].body).toEqual({
    vehicle_number: 'BUS-707',
    route_id: 7,
    trip_id: 200,
    latitude: 43.4723,
    longitude: -80.5449,
    heading: 180,
    speed_kmh: 38,
    status: 'delayed',
    is_accessible: true,
  })
  expect(mutationCalls[1].body).toEqual({ speed_multiplier: 2 })
  for (const call of mutationCalls) {
    expect(call.headers['X-CSRF-Token']).toBe(adminSession.csrfToken)
  }
})

it('supports /admin/disruptions for US 190 create/resolve disruption, US 186/187/188 external incident feeds with CSRF, and US 196 enterprise service status', async () => {
  let disruptionsCatalog = [...fixtureAllDisruptions]
  const mutationCalls = []

  const fetchMock = vi.fn(async (path, options = {}) => {
    const method = options.method || 'GET'
    const url = new URL(path, 'https://transit.test')
    if (url.pathname === '/api/auth/me') return jsonResponse(adminSession, 200)
    if (url.pathname === '/api/routes' && method === 'GET') return jsonResponse(fixtureRoutes, 200)
    if (url.pathname === '/api/disruptions' && method === 'GET') {
      return jsonResponse(disruptionsCatalog, 200)
    }
    if (url.pathname === '/api/integrations/service-status' && method === 'GET') {
      return jsonResponse(serviceStatusResponse({ disruptions: disruptionsCatalog }), 200)
    }
    if (url.pathname === '/api/disruptions' && method === 'POST') {
      const body = JSON.parse(options.body)
      mutationCalls.push({ path: url.pathname, method, headers: options.headers, body })
      const created = {
        id: 410,
        type: body.type,
        title: body.title,
        description: body.description || '',
        affected_area: body.affected_area || '',
        start_time: body.start_time || '2026-11-10T14:00:00Z',
        end_time: body.end_time || null,
        status: body.status || 'active',
        source_service: 'admin',
        external_reference: null,
        received_at: '2026-11-10T14:00:00Z',
        affected_routes: (body.route_ids ?? []).map((id) => {
          const r = fixtureRoutes.find((item) => item.id === id) ?? fixtureRoutes[0]
          return { route_id: r.id, route_number: r.number, route_name: r.name, direction: r.direction }
        }),
        affected_stops: (body.stop_ids ?? []).map((stopId) => ({
          stop_id: stopId,
          stop_name: `Stop #${stopId}`,
          route_id: body.route_ids?.[0] ?? 1,
        })),
        affected_trips: body.trip_ids ?? [],
        affected_vehicles: body.vehicle_ids ?? [],
        affected_segments: body.affected_segments ?? [],
      }
      disruptionsCatalog = [created, ...disruptionsCatalog]
      return jsonResponse(created, 201)
    }
    if (url.pathname.startsWith('/api/disruptions/') && method === 'PATCH') {
      const id = Number(url.pathname.split('/')[3])
      const body = JSON.parse(options.body)
      mutationCalls.push({ path: url.pathname, method, headers: options.headers, body })
      disruptionsCatalog = disruptionsCatalog.map((item) =>
        item.id === id ? { ...item, ...body } : item,
      )
      return jsonResponse(disruptionsCatalog.find((item) => item.id === id), 200)
    }
    if (
      (url.pathname === '/api/integrations/utilities/notices' ||
        url.pathname === '/api/integrations/emergency/incidents' ||
        url.pathname === '/api/integrations/security/traffic-alerts') &&
      method === 'POST'
    ) {
      const body = JSON.parse(options.body)
      mutationCalls.push({ path: url.pathname, method, headers: options.headers, body })
      const sourceService = url.pathname.includes('utilities')
        ? 'utilities'
        : url.pathname.includes('emergency')
          ? 'emergency_services'
          : 'security_police'
      const disruption = {
        id: 500 + mutationCalls.length,
        type: body.notice_type || body.incident_type || body.alert_type || 'road_closure',
        title: body.title,
        description: body.description || '',
        affected_area: body.affected_area || '',
        start_time: '2026-11-10T14:05:00Z',
        end_time: null,
        status: body.status || 'active',
        source_service: sourceService,
        external_reference: body.external_id || null,
        received_at: '2026-11-10T14:05:00Z',
        affected_routes: [
          { route_id: 7, route_number: '7', route_name: 'King', direction: 'direction_0' },
        ],
        affected_stops: [
          { stop_id: 23, stop_name: 'University Terminal', route_id: 7 },
        ],
        affected_trips: body.trip_ids ?? [],
        affected_vehicles: [],
        affected_segments: body.affected_segments ?? [],
      }
      disruptionsCatalog = [disruption, ...disruptionsCatalog]
      return jsonResponse(
        {
          created: true,
          disruption,
          matched_route_ids: body.route_ids ?? [7],
          matched_stop_ids: body.stop_ids ?? [23],
          transit_impact: `Impacts Route 7 and University Terminal via ${sourceService}`,
        },
        201,
      )
    }
    throw new Error(`Unexpected request: ${method} ${path}`)
  })
  vi.stubGlobal('fetch', fetchMock)

  const user = userEvent.setup()
  render(<App initialPath="/admin/disruptions" />)

  await screen.findByRole('heading', { name: 'Transit disruptions & incident feeds' })
  expect(screen.getByRole('link', { name: 'Disruptions' })).toHaveAttribute('aria-current', 'page')

  // US 196 Enterprise service status snapshot is loaded from GET /api/integrations/service-status
  const enterprisePanel = screen.getByRole('region', {
    name: 'Enterprise service status snapshot (US 196)',
  })
  expect(await within(enterprisePanel).findByText('Disrupted')).toBeVisible()
  expect(
    within(enterprisePanel).getByText(/Route 7 \(King\) · Detoured/),
  ).toBeVisible()
  await screen.findByText('King St N watermain construction detour')
  expect(fixtureActiveDisruptions).toHaveLength(2)

  // Validation blocks creating a disruption without a title
  await user.click(screen.getByRole('button', { name: 'Create disruption' }))
  expect(screen.getByRole('alert')).toHaveTextContent(/Disruption title is required/i)

  // US 190, 191, 192: Create a disruption with affected route, stop, trip, vehicle, and segment
  await user.selectOptions(screen.getByLabelText('Disruption type'), 'delay')
  await user.type(screen.getByLabelText('Title'), 'Route 7 signal delay at University')
  await user.type(screen.getByLabelText('Description'), 'Expect 10 minute delays due to signal repair.')
  await user.type(screen.getByLabelText('Affected area'), 'University Ave & King St N')
  await user.click(screen.getByLabelText(/Route 7 · King/))
  await user.type(screen.getByLabelText('Affected stop IDs (optional)'), '23')
  await user.type(screen.getByLabelText('Affected trip IDs (optional)'), '200')
  await user.type(screen.getByLabelText('Affected vehicle IDs (optional)'), '507')
  await user.type(
    screen.getByLabelText('Affected route segments (optional)'),
    'King St N at University Ave',
  )
  await user.click(screen.getByRole('button', { name: 'Create disruption' }))

  await screen.findByText(/Created delay disruption "Route 7 signal delay at University"/i)
  expect(screen.getByText('Route 7 signal delay at University')).toBeVisible()

  // Resolve an active disruption from the table
  const createdRow = screen.getByText('Route 7 signal delay at University').closest('tr')
  await user.click(within(createdRow).getByRole('button', { name: 'Resolve' }))
  await screen.findByText(/Marked disruption #410 .* as Resolved/i)

  // US 186 Utilities notice submission with 1-click Waterloo Region template
  await user.click(screen.getByRole('button', { name: 'Weber St Hydro Closure' }))
  expect(screen.getByLabelText('External reference ID')).toHaveValue('UTIL-WR-2026-102')
  await user.click(screen.getByRole('button', { name: 'Submit utilities notice' }))
  const integrationResult = await screen.findByRole('status', {
    name: 'External integration result',
  })
  expect(integrationResult).toHaveTextContent('Impacts Route 7 and University Terminal via utilities')

  // US 187 Emergency Services incident submission
  await user.click(screen.getByRole('tab', { name: 'Emergency Services (US 187)' }))
  await user.click(screen.getByRole('button', { name: 'Central Station Fire Response' }))
  await user.click(screen.getByRole('button', { name: 'Submit emergency incident' }))
  await within(integrationResult).findByText(
    'Impacts Route 7 and University Terminal via emergency_services',
  )

  // US 188 Security / Police traffic alert submission
  await user.click(screen.getByRole('tab', { name: 'Security / Police (US 188)' }))
  await user.click(screen.getByRole('button', { name: 'Uptown Waterloo Traffic Restriction' }))
  await user.click(screen.getByRole('button', { name: 'Submit security traffic alert' }))
  await within(integrationResult).findByText(
    'Impacts Route 7 and University Terminal via security_police',
  )

  expect(mutationCalls).toHaveLength(5)
  expect(mutationCalls.map((c) => `${c.method} ${c.path}`)).toEqual([
    'POST /api/disruptions',
    'PATCH /api/disruptions/410',
    'POST /api/integrations/utilities/notices',
    'POST /api/integrations/emergency/incidents',
    'POST /api/integrations/security/traffic-alerts',
  ])
  for (const call of mutationCalls) {
    expect(call.headers['X-CSRF-Token']).toBe(adminSession.csrfToken)
  }
})
