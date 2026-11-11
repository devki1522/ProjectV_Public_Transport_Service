# Public Transport Service Design

## Implementation status

The application has a React frontend, a containerized Go HTTP service, and
PostgreSQL persistence through GORM. The backend exposes health, route CRUD,
resident timetables, upcoming departures, and stop detail endpoints. See the
[backend API contract](../backend/README.md#resident-schedules-and-departures).

## Components

| Component | Responsibility | Implementation |
| --- | --- | --- |
| Frontend | Resident timetable and departure workflows | `frontend/src/App.jsx`, `frontend/src/features/` |
| Development proxy | Forward `/api` requests to port 8080 | `frontend/vite.config.js` |
| Backend | Register HTTP routes and serve JSON | `backend/cmd/api/main.go`, `backend/internal/router/` |
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

Doxygen generates these Markdown pages as HTML. It does not extract Go or React
API symbols from this repository. Go source should use standard Go documentation
comments; `go doc` can display those separately. Confirm with the instructor
whether manually maintained design pages meet the expected submission scope.
