# Frontend to REST API Mapping

## Source status

`legacy/frontend.html` and `legacy/google-apps-script.js` are not present in the repository or visible Git history. The available frontend is `index.html`. It contains no `google.script.run` calls; it sends action names through `apiCall(action, payload)` to a hard-coded Apps Script URL.

The table maps every observed action and authentication branch. Permissions describe the implemented server policy. Backend business behavior that could not be verified without the Apps Script source remains documented as an assumption.

| Frontend function | Legacy action | REST replacement | Request | Response used by frontend | Permission | Implemented rules/status |
| --- | --- | --- | --- | --- | --- | --- |
| `loginDiscord` / page load | `verifyDiscord` (computed action) | `GET /api/v1/auth/discord/login`, then callback | Login accepts optional `return_to`; callback receives Discord `code`/`state` | JWT plus `{user:{id,discordId,name,avatar,role,isAuthorized}}`, or secure cookie redirect | Public initiation; pre-provisioned Discord user required | Implemented with server code exchange, random state cookie, configured redirect URI, JWT, and no client secret exposure. |
| `loginWithPoliceId` / page load | `verifyPoliceId` (computed action) | None | Old request was `{pid}` | Old user object | Removed | Intentionally unsupported: Police ID alone is not authentication. |
| `searchRoster` | `getPoliceRoster` | `GET /api/v1/members` | Query: `query`, `empFilter`, `discFilter`, `deptFilter`, `genRangeFilter`, `page`, `limit` | `data.items`, `data.groupedByGeneration`, `data.pagination` | Owner/Screener/Admin | Implemented. Deterministic generation/Police-ID ordering; active counts reconciled before filtering. |
| `openProfileModal` | `getMemberProfile` | `GET /api/v1/members/by-police-id/:policeId/summary` | Police ID path | `{info,penalties,seriousWarning}` | Owner/Screener/Admin | Implemented. `seriousWarning` is true at 3+ active penalties or an active permanent penalty. |
| missing `toggleEditProfile` / `saveProfileEdit` | Legacy action unknown | `PATCH /api/v1/members/:id` | Editable generation/rank/medical prefix/name/department/unit | Updated member | Owner/Screener/Admin | Implemented. Server reads old name and writes name history; ignores old name, sheet row, or updater identity. |
| missing `updateRetireStatus` | Legacy action unknown | `PATCH /api/v1/members/:id/status` | `{employmentStatus}` | Updated member | Owner/Screener/Admin | Implemented for confirmed values `รับราชการ` and `ปลดราชการ`. No automatic retirement from penalties. |
| `submitNewMembers` | `addNewMembers` | `POST /api/v1/members/bulk` | `{members:[{policeId,generation,rank,medicalPrefix,fullName,department,unit,employmentStatus}]}` | Created members | Owner/Screener/Admin | Implemented, maximum 100 and transaction-backed on replica-set deployments. The standalone local fallback uses ordered writes. Duplicate Police IDs return 409. |
| — | — | `POST /api/v1/members` | One member object | Created member | Owner/Screener/Admin | Implemented for non-bulk clients. |
| — | — | `DELETE /api/v1/members/:id` | Stable member ID | Success envelope | Owner/Screener/Admin | Implemented as audited soft deletion; Police ID remains reserved. |
| `loadPenaltyTypes` | `getPenaltyTypes` | `GET /api/v1/penalty-types` | None | `[{id,legacyId,name}]` | Owner/Screener/Admin | Implemented. |
| `addPenaltyType` | `savePenaltyType` | `POST /api/v1/penalty-types` | `{name,legacyId?}` | Created type | Owner/Admin | Implemented. Client `updaterId` is discarded. |
| `deletePenaltyType` | `deletePenaltyType` | `DELETE /api/v1/penalty-types/:id` | Stable type ID | Success envelope | Owner/Admin | Implemented; referenced types return 409 rather than corrupting history. |
| `submitPenalty` | `addPenalty` | `POST /api/v1/penalties` | Multipart fields plus `evidence` file; action identifiers may be IDs, legacy PT IDs, or names | `{penaltyId,penaltyCount,isPermanent}` | Owner/Screener/Admin | Implemented. Multiple action types, typed duration/fine, safe image storage, active count, and audit. No automatic retirement. |
| profile rendering | part of `getMemberProfile` | `GET /api/v1/members/:id/penalties` and summary endpoint | Stable member ID or Police ID summary | Complete penalty history with active/permanent/pardon fields | Owner/Screener/Admin | Implemented. Evidence URLs require authenticated retrieval. |
| no working control in current frontend | Backend unknown | `POST /api/v1/penalties/:id/pardon` | `{reason}` | `{penalty,penaltyCount}` | Owner/Screener/Admin | Implemented; record is retained and no longer contributes to active count. |
| `loadRulebook` | `getPenaltyCatalog` | `GET /api/v1/penalty-rules` | None | `[{id,category,name,description,fine,jail}]` | Owner/Screener/Admin | Implemented. |
| `submitRule` | `savePenaltyRule` | `POST /api/v1/penalty-rules` or `PATCH /api/v1/penalty-rules/:id` | Rule object | Created/updated rule | Owner/Admin | Implemented. Client `updaterId` and `updaterRole` are discarded. |
| `deleteRule` | `deletePenaltyRule` | `DELETE /api/v1/penalty-rules/:id` | Stable rule ID | Success envelope | Owner/Admin | Implemented with server authorization. |
| `loadRoles` | `getUsersList` | `GET /api/v1/users` | `page`, `limit` | `{items,pagination}` | Owner/Screener/Admin | Implemented. Frontend must retain stable `item.id`, not treat Discord ID as route ID. |
| `updateRole` | `updateUserRole` | `PATCH /api/v1/users/:id/role` | `{role}` | Updated user | Owner or Screener under hierarchy policy | Implemented. Owner cannot be assigned through API; self changes and Owner target changes are denied; Screener may only assign Admin/Guest. |
| `loadAuditLogs` | `getAuditLogs` | `GET /api/v1/audit-logs` | `page`, `limit` | Newest-first `{items,pagination}` | Owner/Screener/Admin | Implemented with immutable server-generated records. Frontend field names change from `date/admin/details` to `createdAt/actorId/description`. |
| no current frontend call | Backend unknown | `GET /api/v1/metadata/departments` | None | `{items:[...]}` | Owner/Screener/Admin | Implemented from distinct stored departments. No legacy default list was available to preserve. |
| profile evidence link | Direct legacy URL | `GET /api/v1/evidence/:id` | Evidence ID; Bearer token or secure auth cookie | Authorized image bytes | Owner/Screener/Admin | Implemented with private/no-store response and path isolation. |
| browser logout | local-storage deletion only | `POST /api/v1/auth/logout` | Bearer/cookie JWT | Success envelope | Any authenticated role | Implemented by incrementing token version and clearing the auth cookie. |

## Contract mismatches requiring frontend changes

1. The old `apiCall` expects `{status:"success",data}`. REST responses use numeric `status` and HTTP status codes.
2. The roster previously received an object keyed directly by generation. The REST response exposes the equivalent at `data.groupedByGeneration` and also includes paginated `items`.
3. `getMemberProfile` used `{info:{policeId,gen,...}, penalties}`. The summary keeps `info`/`penalties`, but fields are `generation`, `employmentStatus`, and typed ISO timestamps.
4. Penalty upload must send `multipart/form-data`; base64 fields, MIME claims, filenames, and `punisherId` are no longer accepted as authority.
5. Penalty duration values should be `permanent`, `months`, or `date_range`; the service temporarily accepts the three observed Thai values for migration compatibility.
6. Role updates use stable internal user ID in the URL. Actor ID/role fields must be removed.
7. Audit rows use structured fields and pagination.
8. Errors must be handled from non-2xx status plus `{error:{code,details}}`, rather than assuming a 200 transport response.

## Current frontend defects independent of the backend

- `toggleEditProfile`, `saveProfileEdit`, and `updateRetireStatus` are referenced by HTML controls but have no JavaScript definitions.
- Profile rendering never populates edit form fields, retirement visibility/buttons, rank, medical prefix, department, unit, or employment status.
- The login path still offers insecure Police-ID-only login.
- The frontend stores its authorization object in editable local storage and uses it to show privileged controls.
- `innerHTML` interpolates names, rules, audit text, and evidence URLs without escaping, allowing stored XSS.
- Penalty type selection is a single `<select>`, while the required backend contract supports multiple selections.
- The evidence anchor relies on direct URLs; Bearer-only retrieval would not work in a normal anchor. The backend also supports a secure HTTP-only cookie so same-site/allowed-origin navigation works.
- The role input is labeled “Discord/Police ID”; the REST route requires the stable user `id` returned by `GET /users`.
- The current frontend has no pardon UI, department filter, generation-range filter, fine input, or rendering for the permanent/3+ serious warning response.

## Unconfirmed behavior

Exact legacy role checks, department defaults, penalty month arithmetic, date-range inclusivity, historical fine units, and sheet audit semantics cannot be confirmed until `legacy/google-apps-script.js` and a redacted schema/export are provided. Current documented choices are secure, explicit defaults rather than claims about missing legacy behavior.
