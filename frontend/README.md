# Frontend

The React UI uses Vite and currently displays the backend's health status.
Use the [project setup](../README.md#setup) to run it with Docker Compose.

## Development

Run UI commands from `frontend/`:

```bash
npm run dev -- --port 3000
```

`vite.config.js` forwards `/api` requests to `http://backend:8080` for Compose.
When running Vite on your host instead, change that target to
`http://localhost:8080` and ensure the backend is running. Stop the Compose
frontend first to free port 3000; restore the proxy target before returning
to Compose.

## Files and checks

- `src/main.jsx`: React entry point.
- `src/App.jsx`: application UI and API requests.
- `src/App.css` and `src/index.css`: application and shared styles.

```bash
npm run lint -- --deny-warnings
npm run build
```

The build writes static files to `dist/`. There is currently no frontend unit
test runner.
