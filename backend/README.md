# Backend

The Go API connects to PostgreSQL before starting its HTTP server. Use the
[project setup](../README.md#setup) to run it with Docker Compose.

## API

`GET http://localhost:8080/api/health` returns HTTP 200 with:

```json
{ "status": "ok" }
```

The health handler confirms that the server is running without querying the
database. Public route reads are available at `GET /api/routes` and
`GET /api/routes/{id}`; route mutations (`POST`, `PATCH`, `DELETE`) require an
authenticated `TransitAdministrator` session and CSRF validation.

### Authentication and administrator portal

The backend owns browser sessions, HS256 JWT validation, CSRF verification, and
authorization. The frontend never receives the JWT or raw session identifier.

| Endpoint | Auth / CSRF | Response |
| --- | --- | --- |
| `POST /api/auth/login` | Trusted `Origin`, JSON body, rate-limited | Sets opaque `HttpOnly` session cookie; returns `{ user, expiresAtUtc, csrfToken, isAdmin }` |
| `GET /api/auth/me` | Valid session cookie | Returns `{ user, expiresAtUtc, csrfToken, isAdmin }` or `401` |
| `POST /api/auth/logout` | Trusted `Origin`; `X-CSRF-Token` when session active | Invalidates local session and clears cookie |
| `GET /api/admin/health` | Session + `TransitAdministrator` | `200 { "status": "ok" }` |
| `GET /api/admin/dashboard` | Session + `TransitAdministrator` | `200 { total_routes, active_routes, inactive_routes }` from PostgreSQL |
| `POST /api/routes` | Session + `TransitAdministrator` + CSRF | Creates a route (`201`) and logs a structured audit event |
| `PATCH /api/routes/{id}` | Session + `TransitAdministrator` + CSRF | Updates a route (`200`) and logs a structured audit event |
| `DELETE /api/routes/{id}` | Session + `TransitAdministrator` + CSRF | Soft-deactivates a route (`204`, schedules preserved) and logs a structured audit event |

Auth and admin responses include `Cache-Control: no-store`. Error codes use JSON:
`400` invalid request, `401` invalid credentials or missing/expired session,
`403` missing role or CSRF/origin failure, `429` throttled login, `502` invalid
upstream response, `503` provider unavailable, and `504` upstream timeout.

#### Session, CSRF, and expiration rules

- **Session store**: In-memory store (`internal/auth.InMemorySessionStore`) behind a `SessionStore` interface ready for Redis. Backend restarts invalidate active sessions, and multiple backend instances require a shared store.
- **Fifteen-minute expiration**: Both providers issue 15-minute JWTs. Session expiration is bounded by the JWT `exp` claim and never slides or extends (`rememberMe` is ignored).
- **Cookie attributes**: `HttpOnly=true`, `SameSite=Lax`, `Path=/api`, no `Domain` attribute. Use `AUTH_COOKIE_SECURE=false` only for local `http://localhost`; `AUTH_COOKIE_SECURE=true` is enforced outside development.
- **CSRF protection**: Login and all authenticated mutations require an `Origin` matching `AUTH_TRUSTED_ORIGINS`. Authenticated mutations (and logout when a session exists) also require the `X-CSRF-Token` header returned by `/api/auth/login` and `/api/auth/me`.
- **Audit logging**: Route mutations emit structured JSON events (`timestamp`, `subject`, `operation`, `resource_type`, `resource_id`, `outcome`) to stdout without credentials or tokens. Application logs are not a durable compliance audit store.

#### Providers and Registration Service integration

- **Mock mode** (`APP_ENV=development`, `AUTH_PROVIDER=mock`): Authenticates `transit.admin` (`TransitAdministrator`) against `MOCK_ADMIN_PASSWORD` and signs real 15-minute `HS256` JWTs with `MOCK_JWT_SECRET` (>= 32 bytes). Mock mode and `MOCK_*` variables are rejected outside `APP_ENV=development`.
- **Registration Service** (`AUTH_PROVIDER=registration`, default outside development): Posts credentials once (no retries, redirects rejected) to `{REGISTRATION_SERVICE_URL}/api/Login/authenticate`, validates the returned `HS256` JWT locally against `REGISTRATION_JWT_ISSUER`, `REGISTRATION_JWT_AUDIENCE`, and `REGISTRATION_JWT_SECRET`, and reads authorization exclusively from the signed `role` claim (string or array), ignoring top-level `role`. HTTPS is required outside explicit local development.
- **External team dependency**: Inspected source (`CmRegistrationService`) seeds only `Administrator` and `User` roles. The Registration Service team must create and assign `TransitAdministrator` and provide production URL/issuer/audience/secret values before live integration verification.

### Resident schedules and departures

These read-only endpoints implement work items 164, 166 and 167 and the relevant
SRS requirements PT-FR-004, PT-FR-006, PT-FR-007, PT-NFR-005 and PT-NFR-006.
No authentication is required for resident reads.

| GET endpoint                                | Response                                                                         |
| ------------------------------------------- | -------------------------------------------------------------------------------- |
| `/api/routes/{id}/schedule?date=2026-11-10` | Route, service date, timezone, and trips with ordered stop times                 |
| `/api/departures?stop_id=1`                 | Upcoming departures filtered by stop, route, and/or trip                         |
| `/api/stops/{id}/departures`                | Upcoming departures at a stop                                                    |
| `/api/routes/{id}/departures`               | Upcoming stop departures along a route                                           |
| `/api/trips/{id}/departures`                | Upcoming stop departures for a recurring trip                                    |
| `/api/stops/{id}`                           | Stop details (coordinates, accessibility, active status) and upcoming departures |

Timetables require a valid `date` in `YYYY-MM-DD` format. They return
`{ "route": {...}, "service_date": "2026-11-10", "timezone": "America/Toronto", "trips": [...] }`.
Each trip includes `id`, `schedule_type`, `status` (`scheduled`), and `stop_times`.
Each stop time includes `route_stop_id`, `stop_id`, `stop_name`, `stop_sequence`,
`scheduled_arrival_time`, and `scheduled_departure_time`.

Departure endpoints accept the following query parameters. The stop detail
endpoint accepts the same parameters. Filters are combined with AND; an ID in
the path fixes that filter, and a conflicting query ID is rejected.

| Parameter                        | Meaning / default                                                    |
| -------------------------------- | -------------------------------------------------------------------- |
| `stop_id`, `route_id`, `trip_id` | Positive 32-bit identifiers; `/api/departures` requires at least one |
| `from`                           | RFC3339 timestamp with timezone; defaults to now                     |
| `until`                          | RFC3339 timestamp with timezone; defaults to `from` plus 2 hours     |
| `limit`                          | 1–100 results; defaults to 20                                        |

The departure window includes `from` and excludes `until`, must be positive,
and cannot exceed 24 hours. Results are ordered by scheduled departure, then
route, trip, and stop sequence. Times are RFC3339 absolute timestamps; display
them in `America/Toronto` for local transit time. Encode `+` as `%2B` in query
timezone offsets, or use UTC (`Z`). Unknown, repeated, and empty parameters
are rejected. Example:

```http
GET /api/departures?stop_id=1&route_id=7&from=2026-11-10T12:00:00Z&until=2026-11-10T14:00:00Z&limit=20
```

```json
[
  {
    "trip_id": 42,
    "route": {
      "id": 7,
      "number": "7",
      "name": "Main",
      "direction": "direction_0",
      "is_active": true
    },
    "route_stop_id": 100,
    "stop_id": 1,
    "stop_name": "Station",
    "stop_sequence": 3,
    "service_date": "2026-11-10",
    "scheduled_departure_time": "2026-11-10T12:10:00Z",
    "estimated_departure_time": null
  }
]
```

IDs and values in this example are illustrative. `route.direction` supplies the
direction required by PT-FR-007; GTFS direction codes are not compass directions.
Loop stops retain their distinct `route_stop_id` and sequence. Stop details use
`{ "stop": {...}, "departures": [...] }`; accessibility uses
`wheelchair_boarding` (0 unknown, 1 accessible, 2 inaccessible).

Missing resources return 404, invalid input returns 400, and database failures
return a generic 500 JSON error without database details. Existing resources
without matching service return empty arrays. Departures exclude inactive
routes, trips, and stops; an inactive route has an empty timetable.

The current database represents recurring weekday, weekend, and daily trips.
Timetables select service by the agency-local date, preserving after-midnight
intervals such as `25:10:00` as next-day timestamps. Upcoming departures also
include previous service days when their stop times fall within the requested
window. PostgreSQL resolves daylight-saving gaps/overlaps in the agency zone.
The fixed GRT seed has no holiday exceptions, date-specific cancellations, or
live predictions; see the [seed limits](../database/README.md#seed-contents-and-limits).

Estimated departures are nullable and never replace scheduled times. A live
integration can implement `transit.EstimateProvider` and supply it through
`router.NewWithEstimates`. Estimates are keyed by trip, route-stop occurrence,
and service date to avoid mixing recurring trips or loop stops. Missing
predictions, zero predictions, or provider failures leave estimates null and
scheduled times available. The default server has no live provider configured;
this change does not calculate predictions from GPS data (PT-FR-011).

## Develop outside Docker

After project setup, leave the database running and stop the containerized API
from the repository root so port 8080 is available:

```bash
docker compose stop backend
```

In `backend/.env`, change the `DATABASE_URL` hostname from `database` to
`localhost`, then run:

```bash
cd backend
go run ./cmd/api
```

Run from `backend/` so the API loads its `.env`. Restore the hostname to
`database` before returning to Compose. Available settings and pool defaults
are listed in [`.env.example`](.env.example).

## Code and tests

- `cmd/api/`: startup, dependency wiring and HTTP routes.
- `internal/config/`: environment parsing and validation.
- `internal/database/`: database connection and pool setup.
- `internal/models/`: GORM models defining the database tables and constraints.
- `internal/auth/`: authentication providers (`mock`, `registration`), HS256 JWT validation, session store, CSRF, throttling, and HTTP middleware/handlers.
- `internal/handlers/` and `internal/router/`: HTTP validation, responses, admin dashboard, and routing.
- `internal/transit/`: resident schedule logic, PostgreSQL reads, and estimate-provider interface.

Keep startup small. As endpoints grow, separate HTTP handling, application
logic and database access under `internal/`, passing dependencies explicitly.

Run backend tests from this directory:

```bash
go test -count=1 ./...
```

Tests live beside their implementation as `*_test.go` files and need no database
or credentials. Add tests for behavior changes and format Go files with `gofmt`.

The optional `TestPostgresSchedules` integration test runs when
`TEST_DATABASE_URL` is set to an accessible PostgreSQL database. It uses temporary
tables in a rolled-back transaction and leaves application tables untouched:

```powershell
$env:TEST_DATABASE_URL = 'postgres://projectv:password@localhost:5432/public_transport?sslmode=disable'
go test -count=1 -run TestPostgresSchedules ./internal/transit
```

The API runs GORM AutoMigrate after connecting to PostgreSQL and before serving
HTTP requests. Update the models to change the schema; add new models to the
AutoMigrate call in `cmd/api/main.go`. Startup stops if AutoMigrate fails.

## HTTP API documentation (Swag)

Swagger UI is served by the backend at http://localhost:8080/swagger/index.html.
Use the backend URL; the current Vite proxy forwards only `/api`.
The UI serves bundled assets and loads its specification from `/swagger/doc.json`.
No external CDN is required. Requests target the same host that serves the UI.

From the repository root, regenerate the committed specification after changing
HTTP annotations or request/response types:

```bash
npm run docs:api
```

This runs Swag v1.16.6 and produces `backend/docs/docs.go`, `swagger.json`, and
`swagger.yaml`. Go must be on PATH; the first run downloads the pinned generator.
Commit all three generated files with their source changes. CI regenerates them
and rejects stale output. Docker builds use these committed files.

Keep ordinary Go doc comments above Swag annotations. Swag documents the HTTP
contract; the existing `npm run docs:build` pkgsite build documents Go packages,
types and functions and remains independent. Swagger annotations may appear as
text in pkgsite. Swag output uses Swagger 2.0.

The interactive UI can execute real requests, including route writes. Those
writes require an administrator session and CSRF protection; DELETE deactivates rather than removes
a route. Use a development database when trying mutations.

### Session capability and logout recovery

Login and `/api/auth/me` return `isAdmin`, derived by the backend from the signed
JWT roles and `AUTH_REQUIRED_ROLE`. The frontend uses this capability for page
access; all administrative APIs still enforce the configured role independently.
The frontend returns to sign-in at session expiration. If a logout request fails,
it keeps the session and CSRF token available and shows an error so sign-out can
be retried; it does not report successful logout before backend invalidation.

Run `npm run auth:setup` from the repository root for local mock setup. It preserves
existing nonempty environment values and database configuration. Mock passwords
are stored only in the ignored `backend/.env`.

### Registration Service source verification

The supplied `CmRegistrationService-master.zip` was inspected directly:
`LoginController.cs` issues HS256 tokens with configured issuer/audience, 15-minute
expiration, and signed `role` claims. Its response's first-role field is ignored.
`RememberMe` does not extend JWT lifetime. `DatabaseSeeder.cs` seeds only
`Administrator` and `User`; the other team must create and assign
`TransitAdministrator`. Its logout clears its own cookie without revoking JWTs.
No refresh tokens are implemented. Automated fake-server tests verify our adapter,
not a deployed Registration Service. Deployment URL, verification credentials,
and an assigned administrator account are still required for live verification.

Swagger UI remains at `/swagger/index.html`. The specification includes auth and
admin endpoints and documents session/CSRF requirements on route writes. Swagger
2.0 does not natively model cookie authentication: sign in via the portal on the
same host, then supply the CSRF token and trusted Origin where needed. The server
does not relax CSRF rules for Swagger. Use `npm run docs:api` after contract edits.

The development mock provider accepts only `transit.admin`. Non-administrator
identities are isolated automated-test fixtures used to verify access denial;
they are not development login accounts. Resident pages remain public.
