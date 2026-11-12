import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import App from './App.jsx'
import { sanitizeRedirectPath } from './lib/routerState.js'
import { jsonResponse, routes as fixtureRoutes } from './test/fixtures.js'

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
    expect(screen.getByRole('heading', { name: 'Schedules & departures' })).toBeVisible()
  } finally {
    vi.useRealTimers()
  }
})
