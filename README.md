# Public Transport Service

## Architecture

The application uses three main components:

- **Frontend:** React (JavaScript) with Vite for the user interface.
- **Backend:** Go REST API for processing requests and communicating with other services.
- **Database:** PostgreSQL for storing transit-related data (planned).

The frontend communicates with the backend through HTTP requests. The backend will communicate with PostgreSQL and other City Management services through their respective interfaces.

The application is intended to run in containers using Docker Compose.

## Prerequisites

Install the following:

- [Go](https://go.dev/dl/)
- [Node.js](https://nodejs.org/) (includes npm)
- [Git](https://git-scm.com/downloads)
- [Docker Desktop](https://www.docker.com/products/docker-desktop/) (for future containerized deployment)

## Installation

Clone the repository:

```bash
git clone https://github.com/devki1522/ProjectV_Public_Transport_Service.git
cd ProjectV_Public_Transport_Service
```

Install frontend dependencies:

```bash
cd frontend
npm install
```

The Go backend uses Go modules. Dependencies are managed through `go.mod`.

## Running Locally

Start the backend from the `backend` directory:

```bash
go run ./cmd/server
```

The backend runs at `http://localhost:8080`.

In a separate terminal, start the frontend from the `frontend` directory:

```bash
npm run dev
```

The frontend is typically available at `http://localhost:5173`.

The frontend uses Vite's development proxy to forward `/api` requests to the Go backend.

### Verify the Connection

Open the frontend in your browser. It should display the backend connection status.

You can also check the backend directly:

```text
http://localhost:8080/api/health
```

Expected response:

```json
{
  "status": "ok",
  "service": "public-transport"
}
```

## Project Structure

```text
├── backend/       # Go REST API
├── frontend/      # React application
├── database/      # SQL migrations and seed data (planned)
├── compose.yaml   # Container configuration (planned)
└── README.md
```
