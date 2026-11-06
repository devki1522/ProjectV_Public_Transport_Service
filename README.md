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

For a new checkout, copy the environment examples in PowerShell:

```powershell
Copy-Item .env.example .env
Copy-Item backend/.env.example backend/.env
```

Use `cp` instead of `Copy-Item` in bash. Keep existing `.env` files when updating
a checkout, and never commit them. The examples contain matching local settings;
see the [database guide](database/README.md#configuration) if you change credentials.

Start the app:

```bash
docker compose up --build -d
```

Open the UI at http://localhost:3000. Follow the
[database seed instructions](database/README.md#seed-the-database) to load GRT data.
To stop the app while keeping the database volume, run `docker compose down`.

## Shared checks

Run from the repository root before committing:

```bash
npm run lint
npm test
```

Lint checks frontend code, Go formatting and `go vet`. Tests cover the backend
without a running server. Source files use LF line endings;
Windows `.bat` and `.cmd` files use CRLF, as defined in `.gitattributes`.

`npm ci` installs the Git hooks. Reinstall them with `npm run hooks:install`
if needed. The hooks run relevant checks before commits; GitHub Actions also
runs lint and backend tests on pushes and pull requests.

## Design documentation

Install Doxygen, then run `npm run docs:build` from the root. It builds the
Markdown configured in `Doxyfile` into `build/docs/html/index.html` and packages
`build/docs/design-information.zip`. CI publishes the same ZIP in the
`doxygen-design-information` artifact. Generated output is not committed.
