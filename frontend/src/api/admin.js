export class AdminApiError extends Error {
  constructor(message, status) {
    super(message)
    this.name = 'AdminApiError'
    this.status = status
  }
}

async function readErrorBody(response) {
  try {
    const body = await response.json()
    if (body && typeof body.error === 'string' && body.error.trim()) {
      return body.error
    }
  } catch {
    // Ignore non-JSON body.
  }
  return null
}

async function adminRequest(path, { method = 'GET', body, csrfToken, signal } = {}) {
  const headers = { Accept: 'application/json' }
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
  }
  if (method !== 'GET' && csrfToken) {
    headers['X-CSRF-Token'] = csrfToken
  }

  let response
  try {
    response = await fetch(path, {
      method,
      credentials: 'same-origin',
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
      signal,
    })
  } catch (error) {
    if (error.name === 'AbortError') throw error
    throw new AdminApiError('Transit administration service is unavailable. Check your connection and try again.', 0)
  }

  if (!response.ok) {
    if (response.status === 401) {
      throw new AdminApiError('Your session has expired. Sign in again to continue.', 401)
    }
    if (response.status === 403) {
      const detail = await readErrorBody(response)
      throw new AdminApiError(detail || 'You do not have permission to perform this action.', 403)
    }
    if (response.status === 404) {
      throw new AdminApiError('The requested route could not be found.', 404)
    }
    if (response.status === 409) {
      const detail = await readErrorBody(response)
      throw new AdminApiError(detail || 'A route with that number and direction already exists.', 409)
    }
    if (response.status === 400) {
      const detail = await readErrorBody(response)
      throw new AdminApiError(detail || 'Check the route fields and try again.', 400)
    }
    throw new AdminApiError('The administration request could not be completed. Please try again.', response.status)
  }

  if (response.status === 204) {
    return null
  }

  try {
    return await response.json()
  } catch {
    throw new AdminApiError('The service returned an unreadable response.', 502)
  }
}

export async function getAdminDashboard(signal) {
  const data = await adminRequest('/api/admin/dashboard', { signal })
  if (
    !data ||
    typeof data.total_routes !== 'number' ||
    typeof data.active_routes !== 'number' ||
    typeof data.inactive_routes !== 'number'
  ) {
    throw new AdminApiError('Dashboard summary returned unexpected data.', 502)
  }
  return data
}

export async function getAllRoutes(signal) {
  const data = await adminRequest('/api/routes', { signal })
  if (!Array.isArray(data)) {
    throw new AdminApiError('Route list returned unexpected data.', 502)
  }
  return data
}

export function createAdminRoute(input, csrfToken) {
  return adminRequest('/api/routes', {
    method: 'POST',
    body: input,
    csrfToken,
  })
}

export function updateAdminRoute(id, input, csrfToken) {
  return adminRequest(`/api/routes/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: input,
    csrfToken,
  })
}

export function deactivateAdminRoute(id, csrfToken) {
  return adminRequest(`/api/routes/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    csrfToken,
  })
}
