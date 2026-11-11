# Database Design

**Status:** Initial design with offline GRT location import
**Database:** PostgreSQL  
**Implementation:** [GORM models](../../backend/internal/models)

The initial schema includes the GRT location tables and metadata. The GRT-specific
fields and compatibility rules are detailed in section 7. Update the GORM models
to change the schema; the backend runs AutoMigrate on startup.

## 1. Purpose and scope

This schema stores the persistent information needed for route and stop management, recurring timetables, vehicle identification, and route-centric service disruption notices. It supports the planned React UI and Go REST API, including route browsing, schedules, trip planning, and a live bus map.

**Outside the database:** live vehicle positions and current trip associations are held in Go memory and refreshed from GPS updates or a simulator. The database does **not** persist GPS history, manage vehicle dispatch, payments, payroll, or other services' internal records.

This document describes the design; the GORM models define the column types and constraints.

## 2. ERD and relationships

Place the approved ERD image alongside this document as `docs/database/erd.png`.

| Parent        | Child               | Relationship       | Why                                                              |
| ------------- | ------------------- | ------------------ | ---------------------------------------------------------------- |
| `routes`      | `route_stops`       | 1 to 0..many       | A route has ordered stop occurrences                             |
| `stops`       | `route_stops`       | 1 to 0..many       | One physical stop may serve several routes, even twice on a loop |
| `routes`      | `trips`             | 1 to 0..many       | A route has scheduled departures                                 |
| `trips`       | `stop_times`        | 1 to 0..many       | Each recurring departure has stop-specific times                 |
| `route_stops` | `stop_times`        | 1 to 0..many       | A stop occurrence is visited by multiple scheduled trips         |
| `disruptions` | `disruption_routes` | 1 to 0..many in DB | An event can affect multiple route/stop combinations             |
| `routes`      | `disruption_routes` | 1 to 0..many       | Notices are shown on affected routes                             |
| `stops`       | `disruption_routes` | 0..1 to 0..many    | An impact may optionally target a particular stop                |

`vehicles` has no stored FK to a route or trip by design; its current operating trip is runtime state.

## 3. Table catalogue

### `routes`

| Column      | Meaning                                               |
| ----------- | ----------------------------------------------------- |
| `id` (PK)   | Internal identifier for one direction-specific route  |
| `number`    | Public-facing route number; shared between directions |
| `name`      | Rider-facing route name                               |
| `direction` | Published GTFS traversal ID: `direction_0` or `direction_1` |
| `is_active` | Whether the route remains part of normal service      |

A route number can appear more than once, but `(number, direction)` must be unique. The current model treats opposite directions as separate route records, which duplicates shared names in exchange for a simpler six-week implementation.

### `stops`

| Column                  | Meaning                                    |
| ----------------------- | ------------------------------------------ |
| `id` (PK)               | Physical stop identifier                   |
| `name`                  | Display name                               |
| `latitude`, `longitude` | Position for stop displays and map markers |
| `is_active`             | Whether the stop is part of normal service |
| `wheelchair_boarding`   | Accessibility: 0 unknown, 1 accessible, 2 inaccessible |

Temporary closures or accessibility problems are **disruptions**, not reasons to change the stop's normal accessibility. `wheelchair_boarding` is the only stored stop accessibility field, preserving unknown separately from inaccessible. Geographic ranges are checked in SQL.

### `route_stops`

| Column          | Meaning                                     |
| --------------- | ------------------------------------------- |
| `id` (PK)       | Identifies one stop _occurrence_ on a route |
| `route_id` (FK) | Route containing the stop                   |
| `stop_id` (FK)  | Physical stop                               |
| `stop_sequence` | Position in that route's traversal          |

`UNIQUE(route_id, stop_sequence)` prevents two stops at the same position. Deliberately **no** `UNIQUE(route_id, stop_id)`: loops such as A → B → C → A must be possible. Adding or reordering stops should happen in a transaction. Sequence gaps are acceptable as long as ordering remains correct.

### `trips`

| Column          | Meaning                                  |
| --------------- | ---------------------------------------- |
| `id` (PK)       | One recurring scheduled journey          |
| `route_id` (FK) | Route used by that journey               |
| `schedule_type` | `weekday`, `weekend`, or `daily`         |
| `is_active`     | Whether the recurring journey is enabled |

Trips are **not created once per calendar date**. For example, one weekday trip can represent Route 7 leaving at 08:00 every weekday. Its first departure time comes from its earliest `stop_times` row, so it is not stored again in `trips`.

### `stop_times`

| Column                            | Meaning                                     |
| --------------------------------- | ------------------------------------------- |
| `id` (PK)                         | Stop-time record identifier                 |
| `trip_id` (FK)                    | Scheduled journey                           |
| `route_stop_id` (FK)              | Specific route-stop occurrence              |
| `route_id` (supporting FK column) | Enforces the same route for both references |
| `arrival_time`, `departure_time`  | Offsets from the service day's midnight     |

Both times are `INTERVAL`, not clock-only `TIME`, so a journey can extend past midnight. Departure must be no earlier than arrival. `UNIQUE(trip_id, route_stop_id)` blocks duplicates. The composite FKs involving `route_id` prevent a Route 7 trip from referencing a Route 12 stop occurrence. `route_id` exists for integrity enforcement, not as an independent source of route information.

### `vehicles`

| Column            | Meaning                                |
| ----------------- | -------------------------------------- |
| `id` (PK)         | Internal bus identifier                |
| `number` (unique) | Fleet/public bus identifier            |
| `is_active`       | Whether the bus is in the active fleet |
| `is_accessible`   | Normal vehicle accessibility           |

No `vehicle_assignments` table is required for the agreed live-tracking scope. A GPS feed or simulator supplies `vehicle_id`, current `trip_id`, coordinates, and a timestamp to the Go backend. This data is transient; the system must tolerate missing/stale GPS reports and backend restarts.

### `disruptions`

| Column                   | Meaning                                               |
| ------------------------ | ----------------------------------------------------- |
| `id` (PK)                | Notice identifier                                     |
| `type`                   | Delay, detour, cancellation, closure, etc.            |
| `title`, `description`   | Passenger-facing announcement                         |
| `start_time`, `end_time` | Validity window; `end_time` may be unknown            |
| `status`                 | Scheduled, active, or resolved                        |
| `source_service`         | Public Transport, Utilities, Emergency Services, etc. |

The time window indicates **when** an announcement applies; its route associations indicate **where**. It can describe holiday cancellations, although it does not provide a separate holiday timetable.

### `disruption_routes`

| Column                  | Meaning                                             |
| ----------------------- | --------------------------------------------------- |
| `id` (PK)               | Association identifier                              |
| `disruption_id` (FK)    | Announcement being displayed                        |
| `route_id` (FK)         | Affected direction-specific route                   |
| `stop_id` (nullable FK) | Specific affected stop on that route, if applicable |

Each row means one **route-wide alert** (`stop_id IS NULL`) or one **route-and-stop alert** (`stop_id` set). Several rows can target multiple routes and multiple stops in one announcement. Partial unique indexes prevent duplicated identical targets. The backend must check that a specified stop actually belongs to the route.

## 4. Data integrity and lifecycle

- All keys reference existing parents; normal routes/stops are **deactivated** rather than deleted when possible.
- The models specify restrictive deletion for referenced routes and stops, avoiding accidental loss of schedules and alerts. Deleting a trip removes its stop times; deleting a disruption removes its route associations.
- The backend should require at least one route association before a disruption becomes visible. A normal FK alone cannot guarantee that a parent has children.
- An **announcement** does not, by itself, remove a trip from planning results. Implementing cancellation/closure effects needs explicit filtering logic based on impact type, target, and time range.

## 5. Known limitations and decisions

1. **Simplified schedule types.** Only weekday/weekend/daily are supported. Holiday cancellations can be represented by disruptions; special replacement schedules would require a future extension.
2. **Route-centric disruptions.** We deliberately do not target individual trip instances. A notice can identify a route and affected stops, but automatically cancelling just one specific departure is not modelled. This is a deliberate simplification that should be reconciled with the SRS before claiming full trip-cancellation support.
3. **Route geometry.** Stop order does not define the actual road path. Imported GTFS polylines are stored as GeoJSON in `route_shapes`; use those for route maps.
4. **In-memory live state.** Vehicle locations and trip associations are lost on process restart until refreshed; stale updates must not be shown as current.
5. **Directions.** Routes use only GTFS traversal IDs (`direction_0` / `direction_1`). GTFS IDs do not encode compass bearings; patterns retain destination headsigns for display.
6. **Single-source of truth.** Other enterprise services retain ownership of budgets, personnel and incident records. Public Transport stores its own transit-facing disruption announcements only.

## 6. Implementation and verification

1. Review this ERD and the above limitations as a team.
2. Start PostgreSQL and the backend; wait for startup AutoMigrate to complete.
3. Add test records for two routes sharing a stop, a circular route, and two scheduled trips.
4. Test duplicate stop sequences, invalid coordinates, invalid direction/schedule type, trip-to-route-stop mismatch, departure-before-arrival, and foreign-key deletion behaviour.
5. Test a multi-route disruption with multiple stops; verify invalid route/stop pairs are rejected by the Go API.
6. Update the GORM models when changing the schema and restart the backend to apply changes.

## 7. Offline GRT location data

The [database guide](../../database/README.md) documents the seed contents,
counts, source licence, import commands and validation. The fixed, converted GRT
snapshot is stored directly in `database/seeds/grt.sql`. After the backend has
created the tables, the seed loader imports data in one transaction; it refuses to merge
an initial import into existing location records. Repeating the same snapshot
preserves records and team edits.

The initial schema includes the following structures to preserve the location data:

| Extension | Reason |
| --- | --- |
| `routes.source_id`, `route_type` | Separate external identity from generated PKs; distinguish bus and light rail |
| `direction_0` / `direction_1` direction values | Preserve opposite traversals consistently without assigning compass bearings |
| `stops.source_id`, `stop_code`, `platform_code`, `stop_url`, `location_type`, `parent_station_id`, `wheelchair_boarding` | Retain stop/platform numbers, location links, station relationships and unknown accessibility |
| `route_shapes` | Store actual GTFS polylines as GeoJSON without PostGIS |
| `route_patterns` and `route_pattern_stops` | Preserve each branch/short turn and repeated stop occurrences separately |
| `seed_imports` | Identify a loaded snapshot and prevent repeat imports from overwriting team edits |

`routes` remains direction-specific. A route has many patterns, each pattern
references one shape and has ordered stop occurrences linked to `stops`.
Exactly one pattern per imported route is marked primary by the importer;
a partial unique index prevents two primaries. Existing `route_stops` contains
that primary pattern (longest traversal, frequency/hash tie breaks). It is a
compatibility list, not an assertion that other branches do not exist. Full
route browsing must use patterns when branch coverage matters.

The direction representation and route geometry are addressed for
this import. For class scope, the existing simplified schedule model is retained:
6,666 trips and 190,327 stop times are seeded from representative published
weekday and Saturday services. Weekday repeats Monday–Friday; Saturday repeats
on both weekend days. Holidays and seasonal/date exceptions are ignored. Only
departures with the exact primary ordered stop list are scheduled, preserving
the existing trip/route-stop foreign keys and after-midnight `INTERVAL` values.
No new schedule tables are required. The location-only seed can
be upgraded by rerunning the seed; a separate import marker prevents duplicate
timetables or overwriting existing schedules.

Only if accurate date-aware planning or branch-specific timetables are required
should service calendars and trip-to-pattern associations be added. For the class
scope, no further schedule, fleet, dispatch, GPS-history or fare-management
redesign is needed.
