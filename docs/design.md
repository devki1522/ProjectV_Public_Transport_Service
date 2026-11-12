# Public Transport Service Design

## Implementation status

The application has a React frontend, a containerized Go HTTP service, and
PostgreSQL persistence through GORM. The backend exposes health, route CRUD,
resident timetables, upcoming departures, and stop detail endpoints. See the
[backend API contract](../backend/README.md#resident-schedules-and-departures).

## Components

| Component | Responsibility | Implementation |
| --- | --- | --- |
| Frontend | Resident workflows and Transit Administrator portal | `frontend/src/App.jsx`, `frontend/src/auth/`, `frontend/src/pages/`, `frontend/src/features/` |
| Development proxy | Forward `/api` requests to port 8080 | `frontend/vite.config.js` |
| Backend | Register HTTP routes, auth/admin wiring, and serve JSON | `backend/cmd/api/main.go`, `backend/internal/router/` |
| Authentication & CSRF | Provider abstraction (`mock`/`registration`), HS256 JWT validation, in-memory sessions, origin & CSRF token checks, login throttling | `backend/internal/auth/`, `backend/internal/config/` |
| Admin & Transit writes | Dashboard route inventory counts and audited route mutations | `backend/internal/handlers/admin.go`, `backend/internal/handlers/transit_route.go`, `backend/internal/services/` |
| Transit reads | Validate schedule queries, select recurring service, enrich optional estimates | `backend/internal/transit/` |
| Database | Persist transit data | PostgreSQL and `backend/internal/models/` |

## Request flow

The React app loads active routes from `/api/routes`, groups their public route
numbers, and keeps direction-specific IDs for requests. Selecting a date loads
`/api/routes/{id}/schedule?date=...`; trip disclosures show ordered stop times.
The departure tab requests `/api/departures` with route, optional stop/trip,
and absolute time-window filters. Optional estimates supplement scheduled times.

The API module owns transport, a shared keyed request hook handles cancellation,
retries and visible-page refresh, and feature components own their display state.
Formatting uses the agency timezone rather than the device timezone. The frontend
has explicit loading, empty, error and stale-result states; it supplies no demo
data when the backend is unavailable. See the [frontend guide](../frontend/README.md)
and [UI design system](../frontend/DESIGN.md).

Vite forwards `/api` to the Compose backend by default. `API_PROXY_TARGET` can
point host development at `http://localhost:8080` without changing source.
The health endpoint remains available without querying the database.

## Health API

`GET /api/health` returns HTTP 200 with `Content-Type: application/json`:

```json
{"status":"ok"}
```

## Authentication and administrator portal

- **Backend-owned sessions**: `POST /api/auth/login` authenticates credentials via a startup-selected `auth.Provider` (`mock` in `APP_ENV=development` or `registration` outside development), validates the returned `HS256` JWT strictly against a single active trust configuration, creates an in-memory session keyed by SHA-256 hash of a 32+ byte opaque ID, sets an `HttpOnly`, `SameSite=Lax`, `Path=/api` cookie (`Secure=true` outside development), and returns `{ user, expiresAtUtc, csrfToken }` without ever returning the JWT.
- **Expiration and revalidation**: Session expiration is bounded by the signed 15-minute JWT `exp` claim (`rememberMe` does not extend lifetime). Authenticated requests revalidate the stored JWT against the fixed trust configuration and clock.
- **Authorization and CSRF**: Resident reads (`GET /api/routes`, `GET /api/routes/{id}`, schedules, departures, stops) remain public. `POST`/`PATCH`/`DELETE /api/routes*` and `/api/admin/*` require `RequireAuthenticated` and `RequireRole("TransitAdministrator")`; all state-changing browser session requests require a trusted `Origin` (`AUTH_TRUSTED_ORIGINS`) and constant-time `X-CSRF-Token` validation.
- **In-memory & audit limitations**: `InMemorySessionStore` is isolated behind `SessionStore` for future Redis migration; backend restarts invalidate sessions and multi-instance deployments require a shared store. Route mutations emit structured JSON audit events to stdout without sensitive bodies/secrets, which is not a durable compliance audit store.
- **Registration Service status**: Implemented and verified against `CmRegistrationService` contract and fake HTTP servers (`POST /api/Login/authenticate`, `HS256`, string/array `role` claims, ignoring top-level `role`). Live verification against the deployed Registration Service remains pending until the Registration Service team creates and assigns `TransitAdministrator` and supplies deployment credentials/URL.

## Documentation maintenance

Work items 164, 166 and 167 are implemented through the resident read endpoints.
PT-FR-004 is covered by route timetables with date-selected recurring trips and
ordered stop times. PT-FR-006 is covered by stop details and upcoming departures.
PT-FR-007 is covered by stop/route/trip departure filters, route direction,
scheduled timestamps, and optional estimated departure timestamps. Input
validation and JSON error responses address PT-NFR-005/006 for these reads.

The HTTP layer calls an application service backed by a repository interface.
The PostgreSQL implementation uses parameterized reads and agency-local service
dates. Estimates are supplied through an optional provider keyed by service
date, trip, and stop occurrence; unavailable estimates preserve scheduled times.
No live provider is configured by default. Schedule rules and limitations of the
recurring seed are documented in the backend API contract. Tests cover HTTP,
service and repository behavior, with optional PostgreSQL integration checks.

Update these design pages when components, endpoints, or data flows change, and
keep requirement and work-item references current.

The static website documentation for this repo can be accessed in build/docs. To run a dynamic website use Go pkgsite
