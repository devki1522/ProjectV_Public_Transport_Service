export class AuthApiError extends Error {
  constructor(message, status, code = 'error') {
    super(message)
    this.name = 'AuthApiError'
    this.status = status
    this.code = code
  }
}

async function parseErrorMessage(response) {
  try {
    const data = await response.json()
    if (data && typeof data.error === 'string' && data.error.trim()) {
      return data.error
    }
  } catch {
    // Ignore non-JSON error bodies.
  }
  return null
}

function validateSessionPayload(data) {
  if (
    !data ||
    typeof data !== 'object' ||
    !data.user ||
    typeof data.user.username !== 'string' ||
    !Array.isArray(data.user.roles) ||
    typeof data.csrfToken !== 'string' ||
    !data.csrfToken
  ) {
    throw new AuthApiError('The authentication service returned an unexpected response.', 502, 'invalid_response')
  }
  return data
}

export async function fetchSession(signal) {
  let response
  try {
    response = await fetch('/api/auth/me', {
      method: 'GET',
      credentials: 'same-origin',
      headers: { Accept: 'application/json' },
      signal,
    })
  } catch (error) {
    if (error.name === 'AbortError') throw error
    throw new AuthApiError('Unable to reach the authentication service. Please try again.', 0, 'network')
  }

  if (response.status === 401) {
    throw new AuthApiError('Sign in to continue.', 401, 'unauthorized')
  }
  if (!response.ok) {
    throw new AuthApiError('Authentication status could not be verified. Please try again.', response.status, 'unavailable')
  }

  try {
    return validateSessionPayload(await response.json())
  } catch (error) {
    if (error instanceof AuthApiError) throw error
    throw new AuthApiError('The authentication service returned an unreadable response.', 502, 'invalid_response')
  }
}

export async function loginWithCredentials({ username, password }) {
  let response
  try {
    response = await fetch('/api/auth/login', {
      method: 'POST',
      credentials: 'same-origin',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'application/json',
      },
      body: JSON.stringify({
        username,
        password,
        rememberMe: false,
      }),
    })
  } catch {
    throw new AuthApiError('Authentication service is unavailable. Check your connection and try again.', 0, 'network')
  }

  if (!response.ok) {
    if (response.status === 401) {
      throw new AuthApiError('Invalid username or password.', 401, 'invalid_credentials')
    }
    if (response.status === 429) {
      throw new AuthApiError('Too many sign-in attempts. Wait a moment and try again.', 429, 'throttled')
    }
    if (response.status === 502 || response.status === 503 || response.status === 504) {
      throw new AuthApiError('The authentication provider is currently unavailable. Please try again shortly.', response.status, 'unavailable')
    }
    const detail = await parseErrorMessage(response)
    throw new AuthApiError(detail || 'Unable to sign in. Check your details and try again.', response.status, 'invalid_request')
  }

  try {
    return validateSessionPayload(await response.json())
  } catch (error) {
    if (error instanceof AuthApiError) throw error
    throw new AuthApiError('The authentication service returned an unreadable response.', 502, 'invalid_response')
  }
}

export async function logoutSession(csrfToken) {
  const headers = { Accept: 'application/json' }
  if (csrfToken) {
    headers['X-CSRF-Token'] = csrfToken
  }
  try {
    await fetch('/api/auth/logout', {
      method: 'POST',
      credentials: 'same-origin',
      headers,
    })
  } catch {
    // Clearing local auth state still proceeds so the user can sign in again.
  }
}
