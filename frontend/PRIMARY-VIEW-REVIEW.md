# Primary view: screenshot-led redesign

## Scope
2026-11-19. Only the primary Live Map/stop-departure view and its resident-shell
composition were rebuilt. Global font/token additions were explicitly requested.
Timetable, journey, alerts and admin page components were not redesigned in this pass.

## Diagnosis before implementation
Playwright captured the actual running UI at 1440x900 and 390x844. Both images
were opened directly before editing the page components.

- Desktop map began at y=462. Heading, saved-stop button, duplicate route controls,
  update label and a full-width simulation notice dominated the upper half.
- The vehicle list prioritized speed, vehicle and trip IDs over the commute decision.
  Arrival estimates were below that list and outside the initial desktop viewport.
- Mobile's entire 844px viewport was consumed by chrome and controls. The map and
  departure content were below the fold, behind a second map/list control row.
- Flat emphasis between names, secondary metadata and times prevented fast scanning.
  No large minute countdown was present.
- Saved stops were hidden in a drawer; a large rounded work surface added redundant
  padding. These were structural findings, not a claim of failed measured contrast.

## Token contract (implemented before component rebuild)
`src/transit-tokens.css` is authoritative. `tailwind.config.js` mirrors it for Tailwind
consumers. This app continues using plain CSS; no Tailwind runtime was installed.

| Role | Light | Dark |
| --- | --- | --- |
| --bg-canvas | #EDF0ED | #101A16 |
| --bg-surface | #FFFFFF | #17241D |
| --bg-elevated | #F5F7F4 | #213127 |
| --text-primary | #14251D | #F1F6F1 |
| --text-secondary | #526258 | #B2C2B5 |
| --border-hairline | #D6DDD6 | #3C4E41 |
| --action | #0B6446 | #92DBB1 |

Self-hosted Plus Jakarta Sans for UI/headings; JetBrains Mono with tabular figures
for route numbers, stop IDs, timestamps and countdowns. 30px ETA; 4px control radius;
6px panel radius; one-pixel boundaries; no decorative gradients or raised cards.
Map strokes brighten in dark mode while badges retain their high-contrast solid fills.

## Implementation
- Map and stop departure board share the initial desktop viewport. Search, pinned
  stops, stop heading and live arrivals are grouped in the board. Secondary fleet
  metadata and filters stay below it.
- Primary departure rows use solid line badge, destination headsign from route shape,
  route/direction metadata, large numeric ETA and explicit Estimated/Scheduled source.
- Pinned stops are real device-local preferences; stop search and arrivals use backend
  endpoints. No sample arrivals or fake favorites were added to production.
- Keyboard or pointer selection draws the full route. The map focuses its vehicles
  while the full fleet list remains available. Show all routes restores the network.
- Mobile has a short geographic preview and a Departures switch that removes the
  preview for more board space. Navigation remains thumb-reachable in its original order.
- Countdown rounding, old-estimate handling, invalid-time handling, pin persistence,
  keyboard map selection and existing resident/admin behaviors have test coverage.

## Visual verification and corrections
A first recapture exposed an obscured first row at 320px and excessive marker clutter.
The pin strip was compacted, primary navigation labels shortened, and focused-route
vehicles isolated on the map. Dark route strokes were brightened after direct inspection.
Images were recaptured and opened again after these corrections.

Final screenshots use real backend arrivals at Queen / Woodside (stop 718), pinned
only in an isolated Playwright browser context. The backend simulator is explicitly
labelled in the UI. Separate final-default captures verify the honest empty state at
Central Station, where there were no approaching vehicles during review.

| Capture | Result |
| --- | --- |
| Desktop 1440x900 | Map starts at y=206 (previously 462); visible first departure y=459 |
| Mobile 390x844 | Map starts at y=212; first departure ends at y=733; navigation starts at y=767 |
| Narrow 320x844 | First departure ends at y=764; navigation starts at y=767 |
| Light and dark | Inspected directly at desktop and phone sizes |
| Overflow | Document width equals viewport at 1440, 390 and 320 |
| Runtime | No JavaScript page errors in captured flows |
| Fonts | Computed UI and route-number families match the requested fonts |
| Keyboard | Enter on a departure creates the route path on the map |

Artifacts are under `../build/ui-review/`:
- before-desktop.png, before-mobile.png
- final-desktop.png, final-mobile.png, final-narrow.png
- final-desktop-dark.png, final-mobile-dark.png
- final-mobile-board.png (board-focused mobile mode)
- final-default-desktop.png, final-default-mobile.png
- verification.json, capture.cjs, verify.cjs

Playwright tooling was installed in the OS temporary directory, outside package.json.
To reproduce, set TRANSIT_PLAYWRIGHT to that package path, run the dev server on 5173
with the API proxy targeting the local backend on 8080, then run the capture scripts.

## Checks and boundaries
- Strict lint: passed with --deny-warnings.
- Vitest: 65 tests passed across 7 files.
- Production build: passed; existing large-bundle warning remains outside this scope.
- Whitespace check: passed.
- This is rendered verification of the primary view, not a whole-app visual or
  screen-reader certification. Pixel-perfect comparisons across browsers and dedicated
  screen-reader testing were not performed.

The local Docker frontend was rebuilt and its primary stylesheet verified with HTTP 200 on http://localhost:3000/. The background development server remains available on http://127.0.0.1:5173/.
