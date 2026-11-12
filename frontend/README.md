# Frontend

The React/Vite UI provides resident route timetables and upcoming departures
for work items 164, 166 and 167 (SRS PT-FR-004, PT-FR-006 and PT-FR-007).
Use the [project setup](../README.md#setup) to run it with Docker Compose.

## Resident workflows

- Search routes by public number or name, then select a travel direction.
- In **Timetable**, select a date and optional time-of-day filter. Expand a trip
  to see its ordered arrival and departure times at every stop. Longer lists
  have pages of eight trips.
- In **Upcoming departures**, filter the selected route by stop, trip and a
  2-, 6- or 24-hour window. Refresh manually or leave the tab open for updates
  every minute while the browser page is visible.
- An available estimate is labeled **Estimated** and shown beside the published
  scheduled time. Missing estimates stay **Scheduled**; the UI does not infer
  live data. The default backend has no live provider configured.

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

- `/`: public resident timetable and departure workspace (no login required).
- `/login`: administrator sign-in page (`rememberMe=false`, CSRF-protected same-origin login).
- `/admin`: protected administrator dashboard showing live route inventory counts.
- `/admin/transit`: protected route management (list including inactive routes, create, edit, activate/deactivate with confirmation).
- `/unauthorized`: access-denied page for authenticated users lacking `TransitAdministrator`.

Authentication restores via `GET /api/auth/me` using same-origin `HttpOnly` cookies. The `csrfToken` is kept in React memory only (never `localStorage` or `sessionStorage`), and `X-CSRF-Token` is sent on route mutations and logout. Backend `401` clears session state and returns to `/login`; `403` navigates to `/unauthorized`.

## Files and checks

- `src/main.jsx`: React entry point.
- `src/App.jsx`: client route dispatch, router provider, and auth provider.
- `src/auth/`: `AuthProvider` and `useAuth` state/error handling.
- `src/api/`: `transit.js` (public reads), `auth.js`, and `admin.js`.
- `src/hooks/useResource.js`: keyed requests, cancellation, retries and refresh.
- `src/features/resident/` and `src/features/admin/`: resident workspace and administrator layout/guards.
- `src/pages/`: `LoginPage`, `UnauthorizedPage`, `AdminDashboardPage`, `AdminRoutesPage`.
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
