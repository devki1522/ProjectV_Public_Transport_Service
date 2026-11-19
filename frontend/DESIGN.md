# Waterloo transit: commuter interface contract

## Scope and implementation boundary
Public resident flows: Live Map, Schedules & Departures, Journey Search, Service Alerts.
Retain React 19, Vite, plain CSS, Leaflet, Lucide, self-hosted DM Sans, native controls,
the custom router, and useResource cancellation/retry behavior. No UI framework migration.
Admin routes retain their workflows; shared tokens must keep them readable.

## Audit before component changes
| Area | Evidence in current code | Planned correction |
| --- | --- | --- |
| Theme | index.css forces light; App.css contains many hard-coded surface/status colours | Semantic light/dark tokens, persisted System/Light/Dark setting |
| App shell | ResidentPage header wraps tabs and long labels; large heading sits above map | Compact transport masthead, stable desktop tabs, safe-area bottom tabs on phones |
| Live map | LiveMapView toolbar/banner precedes map; right list can overwhelm map | Dominant map/list split, compact controls, explicit mobile map/list switch |
| Search | RouteSidebar hides searchable rail on phones; live route filter only select | Preserve mobile search; searchable live route filter without removing native select |
| Saved stops | No persistence or saved-stop entry flow | Local-only saved stops with pin/unpin and direct estimated-arrival board |
| Route identity | RouteMap assigns colours by current leg order, other badges use unrelated fills | Shared route-colour allocator and numbered badges; stable identity across views |
| Timing | Estimates already retain schedule, but arrival and route data have mixed weights | Tabular numerals, stronger time hierarchy, readable source/delay labels |
| Loading | DataState has one generic flex skeleton for every result | Geometry-specific timetable/departure/vehicle/journey skeleton variants |
| Connectivity | useResource retains data on error, but offline not surfaced and no age threshold | Shared offline signal; explicit stale/paused notices with update times |
| Accessibility | Native controls and tab keys exist; journey field errors aren't associated | Field errors linked and focused; named arrival regions, stable polite status regions |
| Mobile | route rail replaced by select; min-width controls and small captions | 44px controls, search on phone, wrapping toolbars, map/list toggle and 320px reflow |
| Feedback | Several statuses text + colour, but icons inconsistent | Icons + words for operational states and delays, without asserting on-time from missing data |
| Performance | Maps initialize per view; large bundle and polling | ResizeObserver invalidation only on size changes, stable map fits, no entrance animation |

Source audit, not a claim of rendered WCAG compliance. Browser verification follows implementation.

## Design direction
Swiss wayfinding clarity adapted to Waterloo: left-aligned station names, compact numbered
line lozenges, fixed-width time figures and a dominant regional map. Citymapper informs the
relationship of route choices to the map, not branding or proprietary layouts. The memorable
feature is the transit information itself: coloured line badges and a connected itinerary.
No hero, decorative gradient, card grid, floating ornaments or ornamental metadata.

References consulted:
- https://digital.sbb.ch/en/ (task-oriented, inclusive design)
- https://digital.sbb.ch/en/design-system/mobile/components/typography/
- https://citymapper.com/news/867/share-your-eta-as-you-move (map and arrival relationship)

## Token contract
| Role | Light | Dark |
| --- | --- | --- |
| Canvas | #f1f3f2 | #101715 |
| Surface | #ffffff | #18211d |
| Elevated/hover | #f5f7f6 | #222e28 |
| Primary text | #17231d | #f3f6f4 |
| Secondary text | #53625a | #b3c1b8 |
| Hairline | #d7dfda | #3b4b41 |
| Control boundary | #7b8a81 | #7f9387 |
| Action | #176444 | #92dbb1 |
| Action foreground | #ffffff | #10281b |
| Warning | #805000 | #f2c26d |
| Error | #a32b32 | #ffb1b7 |

Primary face: self-hosted DM Sans, deliberately retained as a legible grotesk rather than
adding a network font. 400 body, 550 labels, 650 stop names, 700 times. Tabular lining figures
on all times, route badges, IDs and countdowns; system monospace only for machine IDs.
Type roles: 12px caption, 14px compact body, 16px body/control, 20px section, 28px page,
32px principal arrival. Rem units and unitless leading. No tiny letterspaced overlines.
Space scale 4/8/12/16/24/32/48. Radius 4px controls/badges, 8px grouped surfaces.
No shadows on content. One-pixel boundaries distinguish rows, not every fragment of text.

## Layout and behavior
Desktop: masthead -> navigation -> compact heading -> toolbar -> map (roughly 2/3) +
vehicle/arrival list (roughly 1/3). Timetables keep a searchable route rail. Journey form
leads into a full-route preview and leg timeline; route colours match cards and map.
Mobile: compact masthead, heading, controls, map/list switch, bottom four-tab navigation.
DOM order matches visual order. Lists remain keyboard alternatives to map gestures.
Saved stops appear in a collapsible commuter drawer and retain only id/name locally.
An unavailable or expired saved stop has retry/remove recovery; never fabricate arrival data.

## Verification contract
Run existing lint, Vitest, production build. Add behavior tests for theme persistence,
pinning and saved arrivals, offline notices, map/list keyboard controls and route identity.
Measure token contrast numerically. Inspect real rendered light/dark views at desktop and
phone widths, then check 320/768/1024/1440, keyboard, reduced motion, and overflow. If a
backend or screen reader isn't available, report that exact gap rather than claiming a pass.

## Implementation and verification (2026-11-19)

Installed 18 project-local skills; all installed SKILL.md files read. Applied the design,
engineering and seven better-* domain rules. No dependency or UI framework added.

Implemented:
- Shared semantic light/dark tokens, persisted appearance with system preference tracking,
  first-paint theme initialization and suppressed colour transitions during theme changes.
- Resident shell, numbered route badges, quieter surface hierarchy, map-dominant desktop
  split, phone map/list switch and safe-area bottom navigation. Existing route search now
  stays available on phones; live route options can be searched by number/name.
- Saved stops stored on-device, searchable catalog, save/remove, persistent pins and real
  GET /api/stops/{id}/arrivals boards with scheduled/estimated sources and simulation labels.
- Stable route-colour mapping with collision resolution within a journey, separate route
  paths, full-network bounds, matching badges/legend and per-leg times.
- Static geometry-specific skeletons, offline banner, age-based stale notices, retained
  stale data, manual retry, pause controls and active-feed reconnect/visibility recovery.
- Associated/focused journey validation, labelled arrival regions, stable polite result
  announcements, native controls and resize-driven Leaflet size invalidation.

Checks completed:
- npm run lint -- --deny-warnings: passed.
- npm test -- --reporter=dot --testTimeout=15000: 61 tests passed in 6 files.
- npm run build: passed. Existing >500 kB bundle warning remains (Leaflet and admin code
  are still in the initial bundle); code splitting remains a performance follow-up.
- git diff --check: passed.
- Local backend public routes endpoint responded. Development preview is served on
  http://127.0.0.1:5173 with API_PROXY_TARGET=http://localhost:8080.
- WCAG luminance calculation from declared tokens: light primary/secondary text on surface
  16.22:1 / 6.44:1; dark 15.15:1 / 8.82:1. Status foreground/background combinations minimum
  6.32:1 light and 7.63:1 dark. White route-label text 5.78:1 to 7.10:1 in both themes.
  Control boundaries 3.62:1 light / 5.04:1 dark on surface. These are token-level checks,
  not an assertion that every rendered pair or the filtered map tile labels passes.

Not verified:
- Browser snapshots, actual 320/768/1024/1440 layout, 200% zoom, screen-reader interaction,
  actual rendered contrast, tile filtering, touch navigation and runtime map drawing.
- Browser host discovery returned unavailable. Native Brave access was then rejected by
  automatic review: "Computer Use was not approved to use Brave". No screenshots were
  captured and no alternate automation path was used to bypass this decision.

The source implementation and automated behavior checks are complete. Visual sign-off and
screen-reader checks remain required before describing this as production-verified.
`docker compose up -d --build --no-deps frontend` rebuilt the local service. HTTP 200 and redesigned resident sources verified on http://localhost:3000/. This is source delivery verification, not a rendered browser check.
