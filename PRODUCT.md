# Public Transport Service

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users and purpose

Residents of Waterloo Region need to select a route and service date, read its
scheduled trips and stop times, and find upcoming departures by route, stop or
trip. Estimated departure times supplement scheduled times when supplied.

## Capabilities and constraints

This class project uses React/Vite, a Go REST API and PostgreSQL. The current
frontend scope is work items 164, 166 and 167 and the relevant SRS requirements
PT-FR-004, PT-FR-006 and PT-FR-007. Existing backend contracts are documented in
backend/README.md. Resident reads are public. Fare payments, dispatch, accounts,
trip planning, maps and administrative editing are outside this frontend task.

The fixed GRT snapshot uses recurring weekday/weekend schedules with no holiday
exceptions. Transit time uses America/Toronto. Direction codes are traversal
identifiers, not compass directions. No live estimate provider is configured by
default; missing estimates must preserve the scheduled time.

## Evidence

User-supplied stories, Downloads/Public_Transport_SRS_Requirements.pdf,
backend/README.md, database/README.md and the implemented REST API.

## Product principles

- Readable route numbers, stop names and times take precedence over decoration.
- Render API data; never fabricate departures, estimates or service status.
- Make loading, empty, failure and scheduled-only states understandable.
- Support keyboard users, small screens and agency-local date boundaries.
