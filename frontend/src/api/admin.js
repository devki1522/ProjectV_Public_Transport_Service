import { apiPath } from './base.js'

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
    response = await fetch(apiPath(path), {
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
      const detail = await readErrorBody(response)
      throw new AdminApiError(detail || 'The requested resource could not be found.', 404)
    }
    if (response.status === 409) {
      const detail = await readErrorBody(response)
      throw new AdminApiError(detail || 'A route with that number and direction already exists.', 409)
    }
    if (response.status === 400) {
      const detail = await readErrorBody(response)
      throw new AdminApiError(detail || 'Check the submitted fields and try again.', 400)
    }
    if (response.status === 503) {
      const detail = await readErrorBody(response)
      throw new AdminApiError(detail || 'This administration service is currently unavailable.', 503)
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
  const data = await adminRequest('/admin/dashboard', { signal })
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
  const data = await adminRequest('/routes', { signal })
  if (!Array.isArray(data)) {
    throw new AdminApiError('Route list returned unexpected data.', 502)
  }
  return data
}

export function createAdminRoute(input, csrfToken) {
  return adminRequest('/routes', {
    method: 'POST',
    body: input,
    csrfToken,
  })
}

export function updateAdminRoute(id, input, csrfToken) {
  return adminRequest(`/routes/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: input,
    csrfToken,
  })
}

export function deactivateAdminRoute(id, csrfToken) {
  return adminRequest(`/routes/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    csrfToken,
  })
}

export function submitVehicleLocation(input, csrfToken) {
  return adminRequest('/vehicles/locations', {
    method: 'POST',
    body: input,
    csrfToken,
  })
}

export function startVehicleSimulator(options, csrfToken) {
  let body
  if (options && typeof options === 'object') {
    const speedMultiplier = options.speed_multiplier ?? options.speedMultiplier
    if (speedMultiplier !== undefined) {
      body = { speed_multiplier: speedMultiplier }
    }
  }
  return adminRequest('/admin/vehicles/simulator/start', {
    method: 'POST',
    body,
    csrfToken,
  })
}

export function stopVehicleSimulator(csrfToken) {
  return adminRequest('/admin/vehicles/simulator/stop', {
    method: 'POST',
    csrfToken,
  })
}

export function stepVehicleSimulator(csrfToken) {
  return adminRequest('/admin/vehicles/simulator/step', {
    method: 'POST',
    csrfToken,
  })
}

export function resetVehicleSimulator(csrfToken) {
  return adminRequest('/admin/vehicles/simulator/reset', {
    method: 'POST',
    csrfToken,
  })
}

export async function getAllDisruptions({ status, type } = {}, signal) {
  const params = new URLSearchParams()
  if (typeof status === 'string' && status.trim() && status !== 'all') params.set('status', status.trim())
  if (typeof type === 'string' && type.trim() && type !== 'all') params.set('type', type.trim())
  const query = params.toString()
  const data = await adminRequest(query ? `/disruptions?${query}` : '/disruptions', { signal })
  if (!Array.isArray(data)) {
    throw new AdminApiError('Disruption list returned unexpected data.', 502)
  }
  return data
}

export function createDisruption(input, csrfToken) {
  return adminRequest('/disruptions', {
    method: 'POST',
    body: input,
    csrfToken,
  })
}

export function updateDisruption(id, input, csrfToken) {
  return adminRequest(`/disruptions/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: input,
    csrfToken,
  })
}

export function submitUtilitiesNotice(input, csrfToken) {
  return adminRequest('/integrations/utilities/notices', {
    method: 'POST',
    body: input,
    csrfToken,
  })
}

export function submitEmergencyIncident(input, csrfToken) {
  return adminRequest('/integrations/emergency/incidents', {
    method: 'POST',
    body: input,
    csrfToken,
  })
}

export function submitSecurityTrafficAlert(input, csrfToken) {
  return adminRequest('/integrations/security/traffic-alerts', {
    method: 'POST',
    body: input,
    csrfToken,
  })
}

export async function getEnterpriseServiceStatus(signal) {
  const data = await adminRequest('/integrations/service-status', { signal })
  if (!data || typeof data !== 'object' || Array.isArray(data) || !Array.isArray(data.active_disruptions)) {
    throw new AdminApiError('Enterprise service status returned unexpected data.', 502)
  }
  return data
}

export async function getAdminStaff(department = '', signal) {
  const params = new URLSearchParams()
  if (typeof department === 'string' && department.trim()) {
    params.set('department', department.trim())
  }
  const query = params.toString()
  const data = await adminRequest(query ? `/admin/staff?${query}` : '/admin/staff', { signal })
  if (
    !data ||
    typeof data !== 'object' ||
    typeof data.active_count !== 'number' ||
    !Array.isArray(data.employees)
  ) {
    throw new AdminApiError('Staff & HR summary returned unexpected data.', 502)
  }
  return data
}

export function addAdminEmployee(input, csrfToken) {
  return adminRequest('/admin/staff/employees', {
    method: 'POST',
    body: input,
    csrfToken,
  })
}
