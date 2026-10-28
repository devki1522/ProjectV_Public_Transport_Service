# Public Transport Service

## Architecture

The application uses a React frontend with Vite, a Go REST API, and PostgreSQL.
Docker Compose runs all three services. The frontend requests `/api/health` through
Vite's `/api` proxy, which forwards to `http://backend:8080` inside Docker.
The backend initializes its database connection before starting the HTTP server.

```text
backend/
  cmd/api/             Application entry point and HTTP router
  internal/
    config/            Environment configuration and validation
    database/          PostgreSQL connection and pool setup
    handlers/          HTTP handlers as endpoints are added
    models/            Domain and persistence types as needed
    routes/            Route registration as the API grows
    services/          Application logic as features are added
    utils/             Shared helpers, only when needed
frontend/              React application
database/             SQL migrations and seed data (planned)
scripts/               Cross-platform quality and documentation commands
compose.yaml           Container configuration
lefthook.yml           Local quality checks
.github/workflows/     CI quality checks and documentation build
```

### Code and architecture rules

- Keep `cmd/api` focused on startup, dependency wiring, and server configuration.
  Put reusable backend implementation in focused packages under `internal`.
- Keep HTTP parsing, response formatting, and status codes in handlers; put
  application rules in services and database setup/access in the database layer.
  Add these layers when a feature needs them rather than creating empty abstractions.
- Pass dependencies explicitly. Services should not depend on HTTP handlers, and
  shared helpers should have a clear purpose rather than becoming a catch-all.
- Read configuration through `internal/config`. Validate required values at startup,
  return errors from library packages, and handle fatal startup errors in `main`.
- Return JSON from API endpoints and set `Content-Type: application/json` before
  writing the response. Keep frontend expectations and API documentation aligned.
- Format Go with `gofmt`, pass `go vet`, and follow the existing frontend Oxlint
  rules. Source files use LF as enforced by `.gitattributes`; keep Windows batch
  scripts on CRLF. Do not override this with editor-specific line-ending changes.
- Keep actual `.env` files out of Git. Commit `.env.example` files with safe local
  examples and update them whenever configuration requirements change.
- Add or update tests with behavior changes. Keep unit tests independent of Docker,
  PostgreSQL, real credentials, and network services.

## Prerequisites

Install Go matching `backend/go.mod`, Node.js 24 or newer with npm, Git, and
Docker Desktop. Go and `gofmt` must be on PATH. On Windows, Go's default binary
folder is `C:\Program Files\Go\bin`; Git for Windows supplies Lefthook's shell.
Python is not required for the project checks.

## Installation

```bash
git clone https://github.com/devki1522/ProjectV_Public_Transport_Service.git
cd ProjectV_Public_Transport_Service
npm ci
npm ci --prefix frontend
```

The root `npm ci` installs Lefthook and installs the Git hooks through the
`prepare` script. Run this once per clone. To reinstall hooks, run
`npm run hooks:install`. Go dependencies are managed by `backend/go.mod` and
`backend/go.sum` and downloaded by Go when needed.

## Running locally

Copy the two environment examples before starting Compose. From PowerShell at
the repository root:

```powershell
Copy-Item .env.example .env
Copy-Item backend/.env.example backend/.env
docker compose up --build -d
```

In bash, use `cp` instead of `Copy-Item`. Copy examples only when creating your
local configuration; preserve existing values when updating an existing clone.

The root `.env` supplies `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB`
to Compose. `backend/.env` supplies the API configuration, including
`DATABASE_URL`. Its credentials and database name must match the root `.env`.
Inside Docker the database hostname is `database`; when running the backend
directly on your computer, use `localhost` instead. Existing PostgreSQL volumes
retain their stored credentials when environment values change.

Open the frontend at `http://localhost:3000`. To check the API directly, open
`http://localhost:8080/api/health`. The expected HTTP 200 JSON response is:

```json
{"status":"ok"}
```

## Testing

Run all backend unit tests from the repository root:

```bash
npm test
```

Or run Go directly from `backend/`:

```bash
go test -count=1 ./...
go test -count=1 -cover ./...
```

`-count=1` ensures tests execute rather than reuse cached successes. The suite
currently covers configuration defaults, environment overrides, invalid inputs,
valid boundaries, and the health endpoint's status, JSON body, and content type.
Tests do not require either `.env` file or a running database.

### Test organization

Put Go unit tests beside the code they test with a `_test.go` suffix, for example
`internal/config/config_test.go`. Go excludes these files from production builds.
Use the same package when a test needs unexported helpers; use a `_test` package
when testing only the public API. Use table-driven tests for related cases,
`t.Setenv` for isolated environment changes, and `httptest` for HTTP behavior.
Do not run environment-mutating tests in parallel.

Use a dedicated `tests/integration/` directory for future tests spanning services
or requiring PostgreSQL. Keep those separately invoked so `go test ./...` remains
self-contained; document their setup when introducing them. There is currently
no frontend unit test runner or database integration suite.

## Automated quality checks

From the repository root:

```bash
npm run lint
```

This runs the same Lefthook pre-commit checks across all tracked files: frontend
Oxlint with warnings treated as failures, Go formatting, `go vet`, and all Go unit
tests. On every commit, the full unit suite runs; frontend lint and Go format/vet
checks run when relevant files are staged and inspect their entire component.
Checks inspect the working tree, including unstaged edits, and do not rewrite
files. Fix formatting with `gofmt -w <file>` and stage the resulting changes.

GitHub Actions runs these same checks on every push and pull request through the
`lint` job, including the full unit suite. A test failure fails the job and blocks
a local commit when hooks are installed. Configure branch protection to require
`lint` to enforce these checks before merging.

## Doxygen design documentation

The sprint input asks for a ZIP containing Doxygen-generated HTML design
information. Doxygen does not have built-in Go or JavaScript/JSX parsers, but it
can generate HTML from Markdown. This repository uses `docs/design.md` as its
initial design page, configured in `Doxyfile`. Add further design pages to the
`INPUT` setting as needed. This build does not extract API symbols from source;
confirm the required documentation scope with the instructor. Use standard Go
documentation comments and `go doc` for Go API documentation.

Install [Doxygen](https://www.doxygen.nl/download.html), then run from the root:

```bash
npm run docs:build
```

Open `build/docs/html/index.html` to review the result. The script packages the
whole HTML directory (including assets) as `build/docs/design-information.zip`.

The `documentation` GitHub Actions job also builds and packages the HTML. Download
the `doxygen-design-information` artifact from a successful workflow run and
extract it to obtain `design-information.zip` for submission. Generated files are
ignored by Git. Documentation generation runs separately from commit hooks.
