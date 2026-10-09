# Architecture

## Runtime flow

```text
HTTP request
  -> Gin router
  -> request ID / security headers / CORS / logging / recovery
  -> module handler (parse, validate, response mapping)
  -> module service (business rules and authorization decisions)
  -> repository interface and MongoDB implementation
  -> MongoDB
```

Dependencies point inward: repositories do not import HTTP code, services do not depend on Gin, and handlers do not query MongoDB. `context.Context` is passed from the request through service and repository calls.

## Package layout

```text
cmd/api                  process entry point and graceful shutdown
docs                     Swagger output and design/migration documents
internal/config          environment parsing and startup validation
internal/database        MongoDB connection, health check, and indexes
internal/http/handler    transport-level handlers
internal/http/middleware cross-cutting HTTP behavior
internal/http/response   public response envelope
internal/http/router     dependency wiring and routes
internal/model           persistence documents (not public DTOs)
internal/dto             explicit public request/response contracts and mappers
internal/service         business rules and authorization-aware operations
internal/repository      interfaces plus MongoDB and isolated-memory implementations
internal/authz           centralized action and role-hierarchy policy
internal/evidence        validated private evidence storage
internal/platform        logging and other process infrastructure
```

New business modules should use feature-specific packages under `internal/modules` while preserving handler/service/repository separation. Constructor injection keeps authorization policy and business services testable.

## Startup and shutdown

1. Load and validate environment variables before opening network listeners.
2. Create a JSON `slog` logger; secrets are never logged.
3. Connect to MongoDB, ping the primary, and create idempotent indexes.
4. Construct Gin with trusted proxies disabled unless explicitly designed later.
5. Start an `http.Server` with bounded read/write/idle timeouts.
6. On SIGINT or SIGTERM, stop accepting requests and disconnect MongoDB using bounded contexts.

Failure to validate configuration, reach MongoDB, or create required indexes is a startup failure. `/health` is a process liveness probe; `/ready` performs a bounded MongoDB ping.

## Identity and authorization

The server exchanges Discord authorization codes, validates a random short-lived state cookie, looks up a pre-provisioned internal user by immutable Discord ID, and issues an HS256 JWT with issuer, audience, stable user ID, role, expiry, JTI, and token version. Authentication reloads the user and compares current role/token version, so role changes and logout invalidate old tokens. The callback can set a secure HTTP-only SameSite cookie and redirect only to an allowlisted origin; Authorization Bearer remains supported.

Logout increments the user's token version, revoking all existing access tokens. Refresh tokens are not implemented; the client repeats Discord login after access-token expiry.

RBAC uses a centralized action policy. Guest has no operational access. Catalog and penalty-type writes are Owner/Admin only. Owner and Screener can manage roles under a stricter hierarchy: no self-change, Owner targets are immutable through the API, Owner cannot be assigned through the API, and Screener can only assign Admin/Guest. UI visibility is never authorization.

## Persistence decisions

- MongoDB `_id` remains internal. Public resources use stable string IDs (`member_id`, `penalty_id`, and similar).
- `police_id` and `discord_id` are unique domain identifiers but are not used as proof of identity.
- Times are stored as BSON UTC datetimes. Asia/Bangkok conversion belongs at input/output boundaries only where confirmed legacy semantics require it.
- Penalty duration, expiry, pardon, and fine fields are typed. Original legacy values can be retained under migration provenance rather than mixed into operational fields.
- Required unique and query indexes are created at startup. Additional indexes must be justified with query evidence.
- Multi-document member/penalty/name-history/audit mutations use MongoDB transactions on replica-set deployments. Standalone local MongoDB falls back to ordered writes, and active penalty counts have explicit reconciliation operations. Production must use a replica set for atomic audit/business mutations.

## Observability and security baseline

- JSON logs contain request ID, method, path, status, latency, client IP, and user agent.
- Panic details and stack traces are logged server-side but never returned to clients.
- CORS is an exact allowlist and rejects wildcard configuration.
- Swagger routes are absent unless `SWAGGER_ENABLED=true`; production should normally set it to false.
- MongoDB is not published to the host in Compose.
- Evidence uploads are multipart-only, bounded by configuration, magic-byte checked for JPEG/PNG/WebP, renamed to opaque IDs, stored outside MongoDB, and retrieved through an authenticated ID lookup. Local storage is suitable for one-instance deployments; clustered production should replace the storage interface with private object storage and signed/authorized retrieval.
