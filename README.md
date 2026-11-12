# Public Transport Service

A class project using React, a Go REST API, and PostgreSQL. Docker Compose runs
the three services. Start here for setup; use the component guides for details:

- [Backend](backend/README.md): API, configuration and Go development.
- [Database](database/README.md): GRT seed, data assumptions and database access.
- [Frontend](frontend/README.md): UI development, API proxy and builds.

## Requirements

Install Git, Docker Desktop, Node.js 24+ with npm, and Go matching
[`backend/go.mod`](backend/go.mod). Go and `gofmt` must be on PATH. Windows
also needs Git for Windows' shell for the commit hooks.

## Setup

Clone the repository and open its folder:

```bash
git clone https://github.com/devki1522/ProjectV_Public_Transport_Service.git
cd ProjectV_Public_Transport_Service
npm ci
npm ci --prefix frontend
```

For a new checkout, copy the environment examples in PowerShell without overwriting existing files:

```powershell
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
if (-not (Test-Path backend/.env)) { Copy-Item backend/.env.example backend/.env }
```

Use `cp -n` in bash (`cp -n .env.example .env && cp -n backend/.env.example backend/.env`). Keep existing `.env` files when updating a checkout, and never commit real secrets.

Before starting the backend in mock development mode (`APP_ENV=development`, `AUTH_PROVIDER=mock`), generate a 32+ byte `MOCK_JWT_SECRET` and choose local passwords for `MOCK_ADMIN_PASSWORD` and `MOCK_USER_PASSWORD` in `backend/.env`:

```powershell
# PowerShell: print a random 32-byte base64url secret (paste into MOCK_JWT_SECRET in backend/.env)
[Convert]::ToBase64String((1..32 | ForEach-Object { Get-Random -Minimum 0 -Maximum 256 })) -replace '\+','-' -replace '/','_' -replace '=',''
```

```bash
# bash / WSL: print a random 32-byte hex secret
openssl rand -hex 32
```

Development mock usernames (passwords come from `backend/.env`):
- `transit.admin` (`TransitAdministrator` role — can view `/admin` and manage routes)
- `transit.user` (`User` role — can sign in, receives `403` from administrator APIs and `/unauthorized` in the UI)

Start the app:

```bash
docker compose up --build -d
```

Open the resident UI at http://localhost:3000 and the administrator sign-in at http://localhost:3000/login. Follow the
[database seed instructions](database/README.md#seed-the-database) to load GRT data.
To stop the app while keeping the database volume, run `docker compose down`.

See [Backend Authentication](backend/README.md#authentication-and-administrator-portal) and [Design](docs/design.md) for session architecture, CSRF rules, and switching to the Registration Service.

## Shared checks

Run from the repository root before committing:

```bash
npm run lint
npm test
```

Lint checks frontend code, Go formatting and `go vet`. Tests cover the backend
and resident frontend behavior without a running server. Source files use LF line endings;
Windows `.bat` and `.cmd` files use CRLF, as defined in `.gitattributes`.

The pre-commit hook runs both categories as separate jobs: `frontend-lint`,
`go-format`, `go-vet`, and `go-test`. On every commit, the full unit suite runs;
frontend lint and Go format/vet checks run when relevant files are staged and
inspect their entire component. Checks inspect the working tree, including
unstaged edits, and do not rewrite files. Fix formatting with `gofmt -w <file>`
and stage the resulting changes.

GitHub Actions runs independent `lint` and `test` jobs on every push and pull
request. The `lint` job runs `npm run lint`; the `test` job runs
`go test -count=1 ./...` from `backend/`, matching `npm test`. The jobs run
independently so a lint failure does not prevent test results from being reported.
A test failure also blocks a local commit when hooks are installed. Configure
branch protection to require both `lint` and `test` before merging.

## Static backend documentation (pkgsite)

Write backend documentation as standard Go comments next to packages, exported
types, fields and functions. The build uses **official pkgsite v0.6.0** to render
those comments, then exports its HTML and CSS into an offline snapshot. New Go
packages are discovered automatically using `go list ./...`, including `internal`
packages and commands. Empty placeholder directories do not appear.

From WSL Ubuntu:

```bash
npm run docs:build
```

Go matching `backend/go.mod` and Node.js 24+ are required. The first build needs
internet access to download the pinned pkgsite tool and its dependencies. Later
builds can reuse Go's caches. No application server, PostgreSQL, Docker or Doxygen
is needed to build backend documentation.

Outputs:

- `build/docs/backend/html/index.html`: open directly in a browser, including
  after extracting the ZIP. It also works under a subdirectory on a static host.
- `build/docs/backend/backend-documentation.zip`: complete shareable site.
- `build/docs/backend/html/build-info.json`: tool version and included packages.

Each build regenerates the snapshot from the current working tree, removes stale
generated HTML, checks local links and source line anchors, then creates the ZIP.
Generated output and the local pkgsite binary under `build/tools` are ignored by
Git. To upgrade pkgsite, change `pkgsiteVersion` in
`scripts/build-backend-docs.mjs`, rebuild, and review the resulting pages.

The snapshot retains pkgsite's documentation layout, package/type/function links,
and offline source pages with line anchors. The package index replaces server
search; browser Find works within each page. Dynamic version/import tabs,
playground controls and other server-dependent controls are omitted. Standard
library, third-party package and Go website links remain online links; their
documentation is not bundled. Only declarations pkgsite normally documents are
included, not private implementation symbols.

GitHub Actions runs the same build and uploads the
`backend-pkgsite-documentation` artifact containing the ZIP.
