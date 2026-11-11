# Backend

The Go API connects to PostgreSQL before starting its HTTP server. Use the
[project setup](../README.md#setup) to run it with Docker Compose.

## API

`GET http://localhost:8080/api/health` returns HTTP 200 with:

```json
{ "status": "ok" }
```

The health handler confirms that the server is running without querying the
database. Route CRUD is available at `/api/routes` and `/api/routes/{id}`.

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
- `internal/handlers/` and `internal/router/`: HTTP validation, responses and routing.
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
