# Backend

The Go API connects to PostgreSQL before starting its HTTP server. Use the
[project setup](../README.md#setup) to run it with Docker Compose.

## API

`GET http://localhost:8080/api/health` returns HTTP 200 with:

```json
{"status":"ok"}
```

This is currently the only endpoint. It confirms that the server is running;
the handler does not query the database.

## Develop outside Docker

After project setup, leave the database running and stop the containerized API
from the repository root so port 8080 is available:

```bash
docker compose stop backend
```

In `backend/.env`, change the `DATABASE_URL` hostname from `database` to
`localhost`, then run:

```bash
cd backend
go run ./cmd/api
```

Run from `backend/` so the API loads its `.env`. Restore the hostname to
`database` before returning to Compose. Available settings and pool defaults
are listed in [`.env.example`](.env.example).

## Code and tests

- `cmd/api/`: startup, dependency wiring and HTTP routes.
- `internal/config/`: environment parsing and validation.
- `internal/database/`: database connection and pool setup.
- `internal/models/`: GORM mappings for every table in the SQL migration.

Keep startup small. As endpoints grow, separate HTTP handling, application
logic and database access under `internal/`, passing dependencies explicitly.

Run backend tests from this directory:

```bash
go test -count=1 ./...
```

Tests live beside their implementation as `*_test.go` files and need no database
or credentials. Add tests for behavior changes and format Go files with `gofmt`.

From the repository root, `npm run test:db` also compares the GORM-generated
tables with `database/migrations/001_initial_schema.sql` using PGlite. This needs
Go and the root npm dependencies, but no running PostgreSQL server. It checks
column order, types, defaults, nullability, identity generation, foreign keys,
checks, and index rules; constraint and index names may differ. Keep the models
and SQL migration synchronized when changing the schema. API startup does not
run GORM AutoMigrate or modify existing tables.
