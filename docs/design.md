# Public Transport Service Design

## Implementation status

The current application is a React frontend and a Go HTTP service with a health
endpoint. PostgreSQL persistence and container deployment are planned.

## Components

| Component | Responsibility | Implementation |
| --- | --- | --- |
| Frontend | Display service status | `frontend/src/App.jsx` |
| Development proxy | Forward `/api` requests to port 8080 | `frontend/vite.config.js` |
| Backend | Register HTTP routes and serve JSON | `backend/cmd/server/main.go` |
| Database (planned) | Persist transit data | PostgreSQL |

## Request flow

On mounting, the React component requests `/api/health`. During local development,
Vite forwards the request to the backend at `http://localhost:8080`. The backend
returns a JSON object with `status` set to `ok` and `service` set to
`public-transport`. The frontend displays the status, or `Connection failed` if
the request fails. No database is queried by this endpoint.

## Health API

`GET /api/health` returns HTTP 200 with `Content-Type: application/json`:

```json
{"status":"ok","service":"public-transport"}
```

## Documentation maintenance

Update these design pages when components, endpoints, or data flows change. Add
requirement and GitHub issue references when they are assigned; this initial page
does not claim traceability to requirements that have not yet been supplied.

The static website documentation for this repo can be accessed in build/docs. To run a dynamic website use Go pkgsite
