# Frontend

The React/Vite UI provides resident route timetables, upcoming departures,
journey search, live vehicle tracking/arrivals, service alerts/affected transit
services, and enterprise disruption administration for work items 164, 166, 167,
177, 178, 180, 183, 186, 187, 188, 190, 191, 192, 194, 195 and 196 (SRS
PT-FR-004 through PT-FR-021).
Use the [project setup](../README.md#setup) to run it with Docker Compose.

## Resident workflows

- Search routes by public number or name, then select a travel direction.
- In **Timetable**, select a date and optional time-of-day filter. Expand a trip
  to see its ordered arrival and departure times at every stop. Longer lists
  have pages of eight trips. Use the **Live buses** button in the route heading
  to jump straight to the Live Map pre-filtered to that route. When the selected
  route has an active disruption or detour (US 192), its operational status pill
  and a disruption alert banner appear with a **View service alerts** shortcut.
- In **Upcoming Departures**, filter the selected route by stop, trip and a
  2-, 6- or 24-hour window. Refresh manually or leave the tab open for updates
  every minute while the browser page is visible.
- Switch to the top-level **Journey Search** tab to enter an origin and
  destination stop or station (with autocomplete suggestions from active stops
  and a swap control), choose a 2-, 6- or 24-hour search window, and set a
  transfer limit (up to 3 transfers) to view direct, multi-transfer, and
  walking-connected itinerary options with expandable per-leg stop lists,
  scheduled vs. estimated departure/arrival times, interactive itinerary
  selection, avoided-disruption notices, or an explicit message when no
  itinerary is available.
- Switch to the top-level **Live Map** tab to view active Waterloo Region buses
  on an interactive Leaflet + OpenStreetMap map (US 178, US 180). Filter active
  vehicles by route (drawing the selected route's polyline path) or operating
  status, toggle 3-second auto-refresh (`Live · 3s` / `Paused`), and select an
  active bus on the map or list to view upcoming stop **Estimated** arrival
  times from `GET /api/trips/{id}/arrivals` (US 183). When simulated fleet
  movement is active, a notice banner states this explicitly.
- Switch to the top-level **Service Alerts** tab to inspect active transit
  disruptions (US 194) and affected routes, stops, trips, vehicles, and route
  segments (US 195) loaded from `GET /api/service-status`. Filter alerts by
  disruption type (`Delay`, `Detour`, `Cancellation`, `Stop closure`,
  `Road closure`, `Construction`, `Emergency`, `Traffic restriction`,
  `Accessibility`) or route, and jump directly from any affected route to its
  **View schedule** or **Live map** view.
- An available estimate is labeled **Estimated** and shown beside the published
  scheduled time. Missing estimates stay **Scheduled**; the UI does not infer
  live data.

All times and today's date use `America/Toronto`, independent of the user's
device timezone. Overnight timetable times have a +1 (or later day) label;
upcoming departures on another date show that date. The fixed seed has no
holiday exceptions. `Direction 1` and `Direction 2` refer to the imported GTFS
traversal codes, not compass directions.

On mobile, a native route selector replaces the searchable route rail. Native
date/select/disclosure controls, keyboard tabs, focus indicators, a skip link,
and explicit loading, empty, error and stale-refresh states support the main
flows. Requests are cancelled when the selection changes.

The UI uses the real [backend API](../backend/README.md#resident-schedules-and-departures).
It contains no demo fallback or hard-coded schedule data. Test fixtures are
isolated under `src/test/` and are not imported by application code.

## Development

Run UI commands from `frontend/`:

```bash
npm run dev -- --port 3000
```

`vite.config.js` forwards `/api` requests to `http://backend:8080` for Compose.
When running Vite on your host, set the proxy target without editing source:

```powershell
$env:API_PROXY_TARGET = 'http://localhost:8080'
npm run dev -- --port 3000
```

Ensure the backend is running. Stop the Compose frontend first to free port
3000. In bash, use `API_PROXY_TARGET=http://localhost:8080 npm run dev -- --port 3000`.
The default remains the Compose backend hostname.

## Client routes and administrator portal

- `/`: public resident timetable, departure, journey search, live map, and service alerts workspace (no login required).
- `/login`: administrator sign-in page (`rememberMe=false`, CSRF-protected same-origin login).
- `/admin`: protected administrator dashboard showing live route inventory counts.
- `/admin/transit`: protected route management (list including inactive routes, create, edit, activate/deactivate with confirmation).
- `/admin/vehicles`: protected live fleet page (US 177) for submitting vehicle GPS location updates (with Waterloo Region location presets and validation), controlling the built-in live bus simulator (1x/2x/5x start/pause, step once, reset), and inspecting the active vehicles table.
- `/admin/disruptions`: protected disruptions & external incident feeds page for creating and updating transit disruptions, affected services, and operational statuses (US 190, US 191, US 192), testing external incident ingestion from Utilities (`POST /api/integrations/utilities/notices`, US 186), Emergency Services (`POST /api/integrations/emergency/incidents`, US 187), and Security/Police (`POST /api/integrations/security/traffic-alerts`, US 188) with Waterloo Region templates, and inspecting the enterprise service status snapshot (`GET /api/integrations/service-status`, US 196).
- `/unauthorized`: access-denied page for authenticated users lacking `TransitAdministrator`.

Authentication restores via `GET /api/auth/me` using same-origin `HttpOnly` cookies. The `csrfToken` is kept in React memory only (never `localStorage` or `sessionStorage`), and `X-CSRF-Token` is sent on route mutations, vehicle location submissions, simulator controls, disruption mutations, external notice ingestion, and logout. Backend `401` clears session state and returns to `/login`; `403` navigates to `/unauthorized`.

## Files and checks

- `src/main.jsx`: React entry point.
- `src/App.jsx`: client route dispatch, router provider, and auth provider.
- `src/auth/`: `AuthProvider` and `useAuth` state/error handling.
- `src/api/`: `transit.js` (public reads including live vehicles, route shapes, simulator status, arrival estimates, active disruptions, affected services, service status, and route operational status), `auth.js`, and `admin.js`.
- `src/hooks/useResource.js`: keyed requests, cancellation, retries and refresh.
- `src/features/resident/` and `src/features/admin/`: resident workspace (timetable, departures, journey search under `src/features/journey/`, live Leaflet map under `src/features/live/`, and service alerts under `src/features/alerts/`) and administrator layout/guards.
- `src/pages/`: `LoginPage`, `UnauthorizedPage`, `AdminDashboardPage`, `AdminRoutesPage`, `AdminVehiclesPage`, `AdminDisruptionsPage`.
- `src/lib/`: `transit.js`, `router.jsx`, and `routerState.js` (including open-redirect sanitization).
- `src/App.css` and `src/index.css`: layout, components and shared design tokens.
- [DESIGN.md](DESIGN.md): visual system and frontend design scope.

This single-screen tool uses local React state and a shared request hook; it
does not need a routing library or global state store. Keep API transport,
formatting and display components separate as features grow. Self-hosted DM
Sans and Lucide icons are bundled locally; no external font service is needed.

```bash
npm run lint -- --deny-warnings
npm test
npm run build
```

The build writes static files to `dist/`. Vitest and React Testing Library
cover resident interactions, API filters, date/time boundaries, estimate
fallback, retries and stale-response protection. Root `npm test` runs both Go
and frontend suites. CI checks frontend tests and the production build.

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
