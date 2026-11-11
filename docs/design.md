# Public Transport Service Design

## Implementation status

The application has a React frontend, a containerized Go HTTP service, and
PostgreSQL persistence through GORM. The backend exposes health, route CRUD,
resident timetables, upcoming departures, and stop detail endpoints. See the
[backend API contract](../backend/README.md#resident-schedules-and-departures).

## Components

| Component | Responsibility | Implementation |
| --- | --- | --- |
| Frontend | Display service status | `frontend/src/App.jsx` |
| Development proxy | Forward `/api` requests to port 8080 | `frontend/vite.config.js` |
| Backend | Register HTTP routes and serve JSON | `backend/cmd/api/main.go`, `backend/internal/router/` |
| Transit reads | Validate schedule queries, select recurring service, enrich optional estimates | `backend/internal/transit/` |
| Database | Persist transit data | PostgreSQL and `backend/internal/models/` |

## Request flow

On mounting, the React component requests `/api/health`. During local development,
Vite forwards the request to the backend at `http://localhost:8080`. The backend
returns a JSON object with `status` set to `ok`. The frontend displays the status,
or `Connection failed` if the request fails. No database is queried by this endpoint.

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
