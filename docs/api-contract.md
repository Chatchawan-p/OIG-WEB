# REST API Contract

These routes are implemented. All business endpoints require a JWT Bearer token or the secure HTTP-only JWT cookie issued by the OAuth callback. Authorization is enforced by route middleware and service policy.

## Response envelope

Success:

```json
{"status":200,"message":"get members successfully","data":{"items":[],"pagination":{"page":1,"limit":20,"total":0}}}
```

Error:

```json
{"status":400,"message":"invalid request","error":{"code":"VALIDATION_ERROR","details":[]}}
```

Use 400 for malformed/invalid input, 401 for missing or invalid authentication, 403 for denied actions, 404 for absent resources, 409 for unique/state conflicts, 422 only when syntactically valid content cannot be processed, 429 for throttling, and 500/503 for internal/dependency failures. Database errors and secrets are never exposed.

## Routes

| Module | Method | Path | Phase |
| --- | --- | --- | --- |
| System | GET | `/health` | Implemented |
| System | GET | `/ready` | Implemented |
| Auth | GET | `/api/v1/auth/discord/login` | Implemented |
| Auth | GET | `/api/v1/auth/discord/callback` | Implemented |
| Auth | GET | `/api/v1/auth/me` | Implemented |
| Auth | POST | `/api/v1/auth/logout` | Implemented |
| Members | GET | `/api/v1/members` | Implemented |
| Members | GET | `/api/v1/members/:id` | Implemented |
| Members | GET | `/api/v1/members/by-police-id/:policeId/summary` | Implemented |
| Members | POST | `/api/v1/members` | Implemented |
| Members | POST | `/api/v1/members/bulk` | Implemented |
| Members | PATCH | `/api/v1/members/:id` | Implemented |
| Members | PATCH | `/api/v1/members/:id/status` | Implemented |
| Members | DELETE | `/api/v1/members/:id` | Implemented |
| Penalties | POST | `/api/v1/penalties` | Implemented |
| Penalties | GET | `/api/v1/penalties/:id` | Implemented |
| Penalties | GET | `/api/v1/members/:id/penalties` | Implemented |
| Penalties | POST | `/api/v1/penalties/:id/pardon` | Implemented |
| Evidence | GET | `/api/v1/evidence/:id` | Implemented |
| Penalty types | GET | `/api/v1/penalty-types` | Implemented |
| Penalty types | POST | `/api/v1/penalty-types` | Implemented |
| Penalty types | DELETE | `/api/v1/penalty-types/:id` | Implemented |
| Catalog | GET | `/api/v1/penalty-rules` | Implemented |
| Catalog | POST | `/api/v1/penalty-rules` | Implemented |
| Catalog | PATCH | `/api/v1/penalty-rules/:id` | Implemented |
| Catalog | DELETE | `/api/v1/penalty-rules/:id` | Implemented |
| Users | GET | `/api/v1/users` | Implemented |
| Users | PATCH | `/api/v1/users/:id/role` | Implemented |
| Audit | GET | `/api/v1/audit-logs` | Implemented |
| Metadata | GET | `/api/v1/metadata/departments` | Implemented |

## Query conventions

`GET /api/v1/members` accepts the frontend-compatible names `query`, `empFilter`, `discFilter`, `deptFilter`, `genRangeFilter`, `page`, and `limit`. `discFilter=3` means three or more active penalties. `genRangeFilter` accepts one generation or an inclusive `min-max`. Defaults are page 1 and limit 100; maximum limit is 500. Results sort deterministically by generation, Police ID, and stable member ID.

Collection endpoints return `data.items` and `data.pagination`. IDs in URL paths are stable public resource IDs, never MongoDB ObjectIDs, Police IDs, or sheet row positions unless a route explicitly documents otherwise.

Penalty creation is `multipart/form-data`. Evidence accepts JPEG, PNG, or WebP up to `EVIDENCE_MAX_BYTES`. Date-range end dates are interpreted as inclusive Bangkok dates and stored as the exclusive next-midnight UTC boundary. This boundary remains an explicit assumption until the missing Apps Script is supplied.
