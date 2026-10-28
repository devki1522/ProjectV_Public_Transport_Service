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

To run the frontend, backend, and database run:
`docker compose up --build --d`
from the project root

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

## Automated quality checks

The repository uses [Lefthook](https://lefthook.dev/), a Git hook manager written
in Go, to run the existing
frontend Oxlint rules, check Go formatting with `gofmt`, and run `go vet`.
Warnings from Oxlint fail the check. Ensure Node.js 24 or newer, npm, Git, and Go
(including `gofmt`) are on your PATH before setting up hooks. On Windows, Go's
default bin directory is `C:\Program Files\Go\bin`; Git for Windows supplies the
shell used by Lefthook. Python is not required.

From the repository root:

```bash
npm ci
npm ci --prefix frontend
npm run lint
```

The root `npm ci` installs the pinned Lefthook binary and installs the Git hook
through the `prepare` script. Each developer needs to do this per clone. To
reinstall the hook manually, run `npm run hooks:install`. On commit, checks run
when relevant frontend or backend files change, and inspect that entire component.
Checks inspect the working tree, including unstaged edits in that component.
`npm run lint` checks all tracked frontend and backend files regardless of staging.
The checks report issues without rewriting files. Fix Go formatting with
`gofmt -w <file>` and fix frontend findings before committing.

GitHub Actions runs the same checks on pushes and pull requests, so checks also
run for contributors who have not installed the local hook. To enforce them before
merging, configure the repository's branch protection to require the `lint` job.

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

## Project Structure

```text
├── backend/       # Go REST API
├── frontend/      # React application
├── database/      # SQL migrations and seed data (planned)
├── compose.yaml   # Container configuration (planned)
└── README.md
```
