# Database

PostgreSQL stores routes, stops, timetables and service disruptions. Complete the
[project setup](../README.md#setup) first. Run the commands below from the root.

## Configuration

The root `.env` defines `POSTGRES_USER`, `POSTGRES_PASSWORD` and `POSTGRES_DB`.
The username, password and database in `backend/.env`'s `DATABASE_URL` must match.
Existing volumes keep the credentials used when first initialized: changing
environment variables does not rename roles or change stored passwords.

If you see `role "transport" does not exist`, check the stored role against your
configuration. This project's original role and current examples use `projectv`.

## Seed the database

Start the database and backend, and wait for the backend's startup AutoMigrate
to complete before loading data:

```bash
npm run db:seed
```

The loader loads [`grt.sql`](seeds/grt.sql) into the existing tables in a
transaction. No GRT
downloads are needed. Repeating it preserves IDs and team edits. A first import
into populated location or schedule tables is refused; errors roll back the load.

For a separate PostgreSQL server, configure the standard `PGHOST`, `PGPORT`,
`PGDATABASE`, `PGUSER` and `PGPASSWORD` variables and use
`npm run db:seed -- --local` with `psql` on PATH. Run the backend against that
database first so AutoMigrate creates the tables.

The [GORM models](../backend/internal/models) define the schema. The backend
applies model changes through AutoMigrate on startup.
See the [database design](../docs/database/design.md) for fields and relationships.

## Inspect the data

Open the PostgreSQL prompt using the example credentials (substitute your own
username and database if different):

```bash
docker compose exec database psql -U projectv -d public_transport
```

```sql
\dt
\d routes
SELECT * FROM routes LIMIT 10;
\q
```

These list tables, describe `routes`, show sample records and exit the prompt.

## Seed contents and limits

The fixed November 5, 2026 GRT snapshot covers Waterloo Region, including
Kitchener, Cambridge and township services:

| Data | Rows |
| --- | ---: |
| Routes (53 public numbers, separated by direction) | 104 |
| Stops and stations | 2,297 |
| Map shapes | 183 |
| Route patterns, including branches and short turns | 186 |
| Recurring trips | 6,666 |
| Stop times | 190,327 |

- All location patterns are retained. `route_stops` contains one representative
  traversal per direction; use the pattern tables for complete branch coverage.
- Only departures matching that representative stop list are scheduled.
- November 5's weekday timetable repeats Monday–Friday; November 7's Saturday
  timetable repeats on both weekend days. Holidays and seasonal changes are ignored.
- Times after midnight remain above 24 hours as PostgreSQL `INTERVAL` values.
- Vehicles, live positions, disruptions and fares are outside the seed.

## Source and licence

Contains information provided by the Regional Municipality of Waterloo under
licence. The data comes from [GRT's public GTFS feeds](https://www.grt.ca/about-grt/open-data/)
under the [Region of Waterloo Open Data Licence](https://regionofwaterloo-018-ca.govstack.com/government-and-council/transparency-and-accountability/open-data/).
Source checksums are recorded in the SQL header. This is a class project and is
not endorsed by GRT or the Region.
