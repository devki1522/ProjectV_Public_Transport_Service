# Resident transit UI

This is an Operate surface: residents find transit times and complete a small
set of tasks. A quiet civic transit identity supports the information rather
than turning it into a marketing page. Product scope is in [PRODUCT.md](../PRODUCT.md).

## Visual system

The source of truth for tokens is `src/index.css`.

| Role | Token / value |
| --- | --- |
| Page canvas | `--canvas`, `#f5f6f3` |
| Work surface | `--surface`, `#ffffff` |
| Primary text | `--ink`, `#243a31` |
| Secondary text | `--muted`, `#647169` |
| Separators | `--line`, `#e2e7df` |
| Functional green | `--accent`, `#246447` |
| Selected background | `--accent-soft`, `#edf3e9` |
| Warning/error text | `--error`, `#93472d` |
| Panel radius | `--radius`, `12px` |
| Control radius | `--control-radius`, `6px` |
| Spacing unit | `--space`, `8px` |

DM Sans Variable is self-hosted, with 400 and 600 weights. Headings range from
17px section labels to a 32px page title (27px on phones). Time data and route
numbers use tabular figures. Green identifies selection, actions and actual
estimates. It never claims an operational status that the API did not supply.

## Structure and behavior

- A route-number rail and work area share aligned top edges. The mobile route
  selector replaces the rail below 768px.
- Direction selection remains separate from public route grouping. Direction
  changes preserve focus and reset route-specific filters.
- Timetable and Upcoming departures are keyboard-accessible tabs. Date,
  select, table and disclosure controls retain familiar native behavior.
- Trip disclosures expose both scheduled arrival and departure times, retaining
  repeated stop occurrences. Times beyond midnight carry a day offset.
- Departures retain the published schedule beside any estimate, with textual
  source and delay labels. Missing or failed estimates stay scheduled-only.
- Loading uses static skeleton rows; empty and failed states explain recovery.
  A failed background refresh retains the previous result with an explicit
  stale notice and its successful update time.
- Hover and press feedback is brief and functional. There are no animated
  entrances or decorative loops. Reduced-motion preferences are respected.

## Verification

Behavior tests cover the three resident stories and request races. Browser
review covers the timetable, expanded stop times and departure board at desktop,
tablet and phone widths. Automated axe WCAG A/AA and horizontal-overflow checks
passed at 1440, 1024, 820, 768, 390 and 320 CSS-pixel viewport widths. These checks
do not replace testing on physical phones or with assistive technology.
