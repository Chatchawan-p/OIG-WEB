# Legacy Feature and Migration Mapping

## Evidence and limitation

The requested source of truth, `legacy/google-apps-script.js`, is absent from the working tree and all visible Git history. This prevents confirmation of Apps Script functions, sheet names/columns, server-side permission checks, date calculations, audit behavior, Drive storage behavior, and data cleanup rules.

The only available legacy implementation evidence is `index.html`. Statements below are labeled:

- **Observed**: directly present in the frontend.
- **Required**: specified by the implementation brief, not verified in legacy backend code.
- **Unknown**: requires the missing script and ideally a redacted Sheets export.

No unknown rule should be invented during later phases.

## Feature/action mapping

| Legacy frontend action or behavior | Observed input/output | Proposed REST destination | Migration note |
| --- | --- | --- | --- |
| `verifyDiscord` | Sends OAuth `code` and client-selected `redirectUri`; receives a user object | `GET /api/v1/auth/discord/callback` | Server must validate state and configured redirect URI, exchange code, fetch Discord identity, find internal user, and issue JWT. |
| `verifyPoliceId` | Sends only `pid` and treats a returned user as authenticated | None | Remove. Police ID is an identifier, not authentication evidence. |
| browser `localStorage.oig_user` | Stores user, role, avatar, authorization flag | JWT-backed `/auth/me`; logout revocation | Do not trust persisted client role/identity. Prefer secure cookie for refresh/session material if introduced. |
| `getPoliceRoster` | `query`, `empFilter`, `discFilter`; response grouped by generation | `GET /api/v1/members` | Return paginated flat items; frontend can group. Preserve search/filter semantics only after backend verification. |
| roster display | `gen`, `policeId`, `fullName`, `penaltyCount`, `empStatus` | Member DTO | Map to explicit field names and stable `member_id`. |
| `getMemberProfile` | Police ID; returns `info` plus penalty list | `/members/:id`, `/members/:id/summary`, `/members/:id/penalties` | URL ID is stable member ID, not a sheet row. |
| batch form / `addNewMembers` | `gen`, `policeId`, `fullName` array | `POST /api/v1/members/bulk` | Define all-or-nothing vs partial result before implementation; enforce Police-ID uniqueness. |
| profile edit form | Contains sheet `row`, Police ID, old name, generation, name; referenced JS functions are missing | `PATCH /api/v1/members/:id` | Never trust row or old name. Read current document and create name history server-side in one transaction. |
| employment status display/filter | Thai values `รับราชการ`, `ปลดราชการ` | `PATCH /api/v1/members/:id/status` | Required endpoint exists, but no working frontend mutation was found. Confirm full enum and transition rules. |
| discipline badge | 0 normal, 1 yellow, 2 orange, 3+ red; discharged takes visual precedence | computed member summary/filter | Treat as presentation derived from active penalty count unless legacy backend proves otherwise. |
| `getPenaltyTypes` | `{id,name}` list | `GET /api/v1/penalty-types` | Preserve historical PT identifier in `legacy_id`; generate stable new `type_id`. |
| `savePenaltyType` / `deletePenaltyType` | Name or ID plus client `updaterId` | POST/DELETE penalty types | Actor comes from JWT. Confirm deletion behavior when referenced by penalties. |
| `addPenalty` | Police ID, action name, reason, duration, dates/months, base64 image and client `punisherId` | `POST /api/v1/penalties` | Resolve member/type IDs server-side, validate evidence separately, and derive actor from JWT. |
| duration selector | `ถาวร`, `ระบุเดือน`, `ระบุวัน` | typed `permanent`, `months`, `date_range` | Confirm inclusivity, timezone, start defaults, month arithmetic, and expiry boundary from source/data. |
| profile penalty display | action, start/expiry dates, reason, image URL | penalty response DTO | Active/expired/pardoned filtering is unknown. Evidence should move to controlled object storage or signed URLs. |
| penalty count | shown as “yellow cards”; filter maps `3` to 3+ | stored reconciliation field plus calculated source | Required semantics say active only. Define permanent, expiry, and pardon calculation before migration. |
| pardon | No frontend action found | `POST /api/v1/penalties/:id/pardon` | Required new/legacy capability, but behavior is unconfirmed. Store actor, reason, and timestamp; never delete history. |
| `getPenaltyCatalog` | `{id,category,name,description,fine,jail}` | `GET /api/v1/penalty-rules` | `fine` and `jail` accept numeric or arbitrary text in UI; normalization needs source/data profiling. |
| `savePenaltyRule` / `deletePenaltyRule` | Rule plus client `updaterId` and `updaterRole` | POST/PATCH/DELETE penalty rules | Enforce Admin/Owner on server; discard claimed role and actor. |
| `getUsersList` | Discord/Police ID, name, role | `GET /api/v1/users` | Determine whether old identifier column mixes identity types before migration. |
| `updateUserRole` | Target Discord ID, new role, claimed requester role/ID | `PATCH /api/v1/users/:id/role` | Central policy must prevent privilege escalation, unsafe peer/superior edits, self-promotion, and last-Owner removal. |
| roles in UI | Owner, Screener, Admin, Guest | centralized RBAC policy | Owner alone can select Screener in UI; Admin/Owner can edit catalog. All other permissions are unknown. |
| `getAuditLogs` | date, admin, free-text details | `GET /api/v1/audit-logs` | Generate immutable structured audit records on the server; retain legacy text as migration description/provenance. |
| departments/unit | Not found in current frontend | metadata and member fields | Required by brief but default values/lookup source are unknown. |

## Legacy data to MongoDB mapping

Exact sheet headers are unknown. This is a target mapping, not proof of legacy columns.

| Target collection | Target identity and key fields | Expected legacy source | Transformation/validation |
| --- | --- | --- | --- |
| `users` | `user_id`, unique `discord_id`, display name, role, timestamps | user/role sheet (unknown) | Reject mixed Police-ID/Discord-ID ambiguity; normalize only the four confirmed roles; manually resolve duplicates. |
| `police_members` | `member_id`, unique `police_id`, generation, full name, active penalty count, employment status, department, unit | roster sheet (unknown) | Generate stable IDs; trim but do not silently change Police IDs; validate generation/status; compute count from migrated penalties after load. |
| `name_history` | `history_id`, member/Police IDs, old/new name, generation, actor, time | name-history sheet if present | Link through Police ID, parse Bangkok legacy times explicitly, flag orphan and no-op rows. |
| `penalties` | `penalty_id`, member/type IDs, reason, typed duration, dates, evidence, pardon, actor, time | penalty sheet and possibly Drive links | Preserve original row ID in migration provenance; parse each duration variant; never infer invalid end dates; flag orphan members/types. |
| `penalty_types` | `type_id`, optional `legacy_id`, unique name | type/config sheet | Preserve PT identifiers; normalize whitespace; resolve duplicate names without losing references. |
| `penalty_catalog` | unique `rule_id`, name, category, fine, jail, description | rule/catalog sheet | Preserve confirmed legacy rule IDs; retain raw fine/jail text until normalization policy is approved. |
| `audit_logs` | `audit_id`, actor, action, resource type/ID, description, UTC time | audit sheet | Legacy free text may not be reliably parseable. Import it as immutable historical description with provenance rather than fabricating structured actions. |

## Migration process (Phase 6)

1. Obtain and checksum the Apps Script source and read-only exports for every sheet/config source.
2. Build a header/version inventory; fail closed on unknown schema variants.
3. Dry-run parses every row without writes and emits counts, duplicates, invalid dates/enums, ambiguous identities, and orphan references.
4. Resolve blocking issues with an explicit mapping file; do not mutate the source export.
5. Import reference data, users, members, types/catalog, penalties/history, then audit history using idempotent upserts keyed by migration source IDs.
6. Recalculate every member's active penalty count from normalized penalties and report discrepancies against the legacy value.
7. Validate collection counts, referential links, sampled profiles, role assignments, and date boundaries.
8. Cut over only after signed reconciliation; retain encrypted, access-controlled source snapshots per retention policy.

Migration tooling must support `--dry-run`, an explicit target database, resumable batches, deterministic output, and a machine-readable reconciliation report. It must refuse production writes without a separate explicit confirmation mechanism.

## Security gaps observed in the frontend

1. Police-ID-only login provides no authentication.
2. OAuth has no visible `state` validation and sends a client-controlled redirect URI to the backend; PKCE is also absent.
3. User identity, authorization flag, and role are stored in editable local storage without a signed server session.
4. Role changes and catalog mutations submit `requesterRole`, `requesterId`, `updaterRole`, `updaterId`, or `punisherId` from the browser.
5. The UI hides controls by role, which is not an authorization boundary.
6. The Apps Script endpoint is hard-coded and the error message directs operators to expose it to “everyone.”
7. API values are inserted with `innerHTML`, including names, audit text, rule text, and URLs, creating stored/reflected XSS risk if data is not perfectly sanitized.
8. Evidence images are read fully into base64 with no visible size, content, dimension, or malware controls.
9. OAuth codes and Police IDs enter the browser URL. The code is removed from history after load, but may still reach server/access logs and referrers before replacement.
10. Logout only removes local storage; it does not revoke a server session.
11. The generic RPC transport always uses POST and has no visible CSRF, rate-limit, request-size, or replay defenses.
12. Frontend errors can surface backend-provided messages directly, risking internal-detail disclosure.

Whether the missing Apps Script mitigates any of these problems is unknown. The new backend must not depend on frontend behavior for security.

## Unresolved business questions

1. Provide `legacy/google-apps-script.js` and a redacted sheet schema/export. What are all function names, sheet names, columns, magic values, and deployed script settings?
2. What is the complete RBAC action matrix? Can Admin manage Admin or Guest, can Owner change self, who can manage Screener, and how is the first/last Owner protected?
3. Are Police IDs mutable? Can a Police ID be reused after deletion, and should member deletion be soft delete for audit retention?
4. What are the complete employment status values and allowed transitions? What defaults apply to department and unit?
5. Is bulk member creation atomic, or should valid rows succeed with row-level errors?
6. For month penalties, what is the start instant and calendar-month rule? For date ranges, is the end date inclusive, and which Bangkok boundary makes a penalty expire?
7. Does an expired or pardoned penalty cease contributing immediately? Can a pardon be reversed? Is a reason required?
8. How were penalty counts maintained, and are there known mismatches? Are multiple penalties ever equivalent to more than one “card”?
9. What do free-text/non-numeric fine and jail values mean? What currency and numeric precision are required?
10. What happens when a referenced penalty type or catalog rule is deleted—restrict, archive, or snapshot display values?
11. Where are evidence images stored, what retention/access rules apply, and may legacy Drive URLs be migrated?
12. What are the required audit retention, visibility, and redaction policies?
13. What frontend origin(s), token transport (Authorization header versus secure cookie), session duration, and logout-all requirements apply in production?
