# Azure deployment: switching to production configuration

**For:** whoever manages the `rg-public-transport` App Service and the
`acrtransportprojectv` container registry.

**Why:** The deployed app was running with `APP_ENV=development`, insecure
cookies, and the database password committed to git. The repository's
[`docker-compose.azure.yml`](docker-compose.azure.yml) now runs in production
mode and contains no secrets. The secrets have to be added in the Azure portal
before deploying it.

> [!IMPORTANT]
> Do step 1 before step 3. If the new compose file is deployed without these
> settings, the backend stops at startup with a configuration error. That is
> deliberate: it fails closed instead of running insecurely.

## What production mode changes

- Session cookies are `Secure`, so the site has to be reached over **HTTPS**. The
  default `https://rg-public-transport.azurewebsites.net` address already works.
- Only HTTPS origins are trusted for sign-in.
- Mock login is rejected. Sign-in goes through the Registration Service, which
  authenticates credentials over HTTPS. Our backend generates its own session
  signing key and grants administrator access through `AUTH_ADMIN_WHITELIST`.

## 1. Add the application settings

Go to **App Service → Settings → Environment variables → App settings** (older
portals: **Configuration → Application settings**) and add the following. App
Service passes these into every container as environment variables.

| Name | Value | Notes |
|---|---|---|
| `POSTGRES_USER` | `projectv` | Read by the database container |
| `POSTGRES_PASSWORD` | *new strong password* | Generate one. Don't reuse `local_dev_password` |
| `POSTGRES_DB` | `public_transport` | |
| `AUTH_ADMIN_WHITELIST` | *Registration usernames, comma-separated* | Required; exact, case-sensitive usernames allowed to administer transit |
| `DATABASE_URL` | `postgres://projectv:<POSTGRES_PASSWORD>@database:5432/public_transport?sslmode=disable` | Same password as above. URL-encode any special characters in it |

You can mark the secret values as Key Vault references if the subscription has a
Key Vault. Plain app settings are also encrypted at rest.

Equivalent Azure CLI command (fill in the placeholders):

```bash
az webapp config appsettings set \
  --resource-group <resource-group> \
  --name rg-public-transport \
  --settings \
    POSTGRES_USER=projectv \
    POSTGRES_PASSWORD='<new-password>' \
    POSTGRES_DB=public_transport \
    DATABASE_URL='postgres://projectv:<new-password>@database:5432/public_transport?sslmode=disable' \
    AUTH_ADMIN_WHITELIST='<admin-username>'
```

### Database password note

The current Postgres container has no persistent volume, so its data and
password are recreated on every restart, and the new password takes effect
immediately. If a persistent volume is added later, note that Postgres only
reads `POSTGRES_PASSWORD` when it first creates the data directory. After that,
changing the password needs an `ALTER ROLE`.

## 2. Confirm the non-secret values

These are committed in `docker-compose.azure.yml`. Check that they match the
real environment, and tell the Public Transport team if any differ:

| Setting | Current value |
|---|---|
| `AUTH_TRUSTED_ORIGINS` | `https://rg-public-transport.azurewebsites.net` (add any custom domain, comma-separated) |
| `REGISTRATION_SERVICE_URL` | `https://cm-rs-api-gbgrffa8h5e5fdcy.canadaeast-01.azurewebsites.net` |
| `REGISTRATION_JWT_ISSUER` / `_AUDIENCE` | `CmRegistrationService` / `CmRegistrationClients` (must match the Registration service's token config) |
| `STAFF_HR_SERVICE_URL` | `https://cmes-staffhr-api-gtofz7g3n2z56.azurewebsites.net` |
| `BUDGET_SERVICE_URL` | `https://financeapi20261125000202-g9ctbdc4axdscbet.westus-01.azurewebsites.net` |

> [!NOTE]
> **Service-to-service integrations (`SH-S07`):** No new secrets are needed in Azure. Calls to `/api/transport/v1/integrations/*` with `Authorization: Bearer <token>` are verified over HTTPS against `{REGISTRATION_SERVICE_URL}/api/Login/validate` without cookies or CSRF. Optionally set `AUTH_INTEGRATION_ROLES` in App settings to override the default allowed roles (`TransitAdministrator,Administrator,Security Administrator,Dispatcher,Manager,ServiceAccount,Service`).

## 3. Deploy the new compose file

1. Pull the latest `main` (or ask the Public Transport team for the updated
   `docker-compose.azure.yml`).
2. Go to **App Service → Deployment Center**. Under the Docker Compose
   configuration, replace the existing YAML with the new
   `docker-compose.azure.yml`, then **Save**.
3. **Restart** the App Service.

The GitHub Action `Build and Push Containers to ACR` keeps publishing the
`backend:latest` and `frontend:latest` images on every push to `main`. That
doesn't change.

## 4. Verify

1. Open **Log stream**. The backend should log
   `Backend running on port 8080 (env=production auth_provider=registration)`.
   - `AUTH_ADMIN_WHITELIST must contain at least one username`: add the administrator usernames.
   - `DATABASE_URL is required`, or database connection errors: check `DATABASE_URL` and `POSTGRES_*`.
2. Visit `https://rg-public-transport.azurewebsites.net/login` and sign in with a
   Registration Service account listed in `AUTH_ADMIN_WHITELIST`.
3. Visiting the site over plain `http://` should no longer keep you signed in. This is expected.

## Rollback

Redeploy the previous compose YAML in the Deployment Center and restart. The app
settings added in step 1 can stay. The old configuration ignores them.

## Planning note

Microsoft retires Docker Compose support in App Service on **March 31, 2027**.
The replacement is App Service **sidecar containers** or Azure Container Apps.
The settings above carry over unchanged, but the compose file itself will need
to be converted before that date.
