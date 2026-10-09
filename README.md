# OIG Police Management API

Production-oriented Go REST backend for migrating the OIG Police Management System away from Google Apps Script and Google Sheets. It implements Discord OAuth2, JWT authorization, members, disciplinary penalties, catalog administration, users/RBAC, audit logs, metadata, and protected evidence storage.

## Requirements

- Go 1.22+
- MongoDB 7 (local compose configuration provided)
- Docker with Compose (optional)

## Local configuration

```sh
cp .env.example .env
# Replace all placeholder secrets and Discord values.
set -a
source .env
set +a
```

The process validates all required configuration at startup. `JWT_SECRET` must be at least 32 characters, example placeholders are rejected, CORS accepts exact origins only, and evidence defaults to a private `data/evidence` directory. Do not commit `.env`.

Users are deliberately not self-registered. At least one Owner with a Discord ID must be provisioned through the migration process or an audited administrative database procedure before login can succeed.

## Run

With a local MongoDB:

```sh
make run
```

With Docker Compose:

```sh
cp .env.example .env
# Edit .env first.
make docker-up
```

The API listens on `http://localhost:8080` by default.

## Implemented endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/health` | Process liveness; does not depend on MongoDB |
| GET | `/ready` | Readiness; checks the MongoDB primary |
| GET | `/swagger/index.html` | Swagger UI when `SWAGGER_ENABLED=true` |
| GET/POST | `/api/v1/auth/...` | Discord login, current user, logout/revocation |
| GET/POST/PATCH/DELETE | `/api/v1/members...` | Member search, profile, bulk CRUD, status, disciplinary summary |
| GET/POST/DELETE | `/api/v1/penalty-types...` | Penalty type management |
| GET/POST | `/api/v1/penalties...` | Multipart penalty creation, details, pardon |
| GET | `/api/v1/evidence/:id` | Authorized evidence image retrieval |
| GET/POST/PATCH/DELETE | `/api/v1/penalty-rules...` | Penalty catalog |
| GET/PATCH | `/api/v1/users...` | User list and hierarchical role changes |
| GET | `/api/v1/audit-logs` | Paginated immutable audit records |
| GET | `/api/v1/metadata/departments` | Stored department options |

Every JSON response uses the documented `{status,message,data}` or `{status,message,error}` envelope. Evidence retrieval returns image bytes. See [docs/api-contract.md](docs/api-contract.md) and generated Swagger for complete route schemas.

## Development commands

```sh
make fmt       # format Go source
make swagger   # regenerate docs/docs.go and OpenAPI files
make test      # go test ./...
TEST_MONGO_URI=mongodb://localhost:27017 make test-integration
make vet       # go vet ./...
make build     # build bin/oig-api
make verify    # test, vet, and build
```

## Documentation

- [Architecture](docs/architecture.md)
- [Proposed API contract](docs/api-contract.md)
- [Legacy feature and migration mapping](docs/legacy-migration.md)
- [Frontend/API mapping](docs/frontend-api-mapping.md)
- [Frontend integration guide](docs/frontend-integration.md)

## Current limitation

The required `legacy/frontend.html` and `legacy/google-apps-script.js` sources were not present in the repository or its Git history. Confirmed compatibility observations therefore come from `index.html`; unverified legacy rules are explicitly marked in the mapping documents. The current frontend still uses the old Apps Script action transport and must apply the documented integration changes before it can call this REST API.
