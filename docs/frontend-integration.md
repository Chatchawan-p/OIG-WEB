# Frontend REST Integration Guide

The current `index.html` was not modified. It must replace the Apps Script action client with HTTP requests to `/api/v1`.

## Shared client

```js
const API_BASE = "http://localhost:8080/api/v1";
let accessToken = null;

async function api(path, options = {}) {
  const headers = new Headers(options.headers || {});
  if (accessToken) headers.set("Authorization", `Bearer ${accessToken}`);
  if (options.body && !(options.body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
    options.body = JSON.stringify(options.body);
  }

  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers,
    credentials: "include"
  });
  const payload = await response.json().catch(() => null);
  if (!response.ok) {
    const error = new Error(payload?.message || `HTTP ${response.status}`);
    error.code = payload?.error?.code;
    error.details = payload?.error?.details || [];
    throw error;
  }
  return payload.data;
}
```

Prefer keeping the access token in memory. The OAuth callback also sets an HTTP-only JWT cookie for browser navigation and return redirects; do not copy roles or tokens into local storage.

## Discord login

Redirect through the backend so it can generate and validate OAuth state:

```js
const returnTo = `${location.origin}${location.pathname}`;
location.assign(`${API_BASE}/auth/discord/login?return_to=${encodeURIComponent(returnTo)}`);
```

After redirect, call:

```js
const currentUser = await api("/auth/me");
```

The user must already exist in MongoDB with a Discord ID and one of `Owner`, `Screener`, `Admin`, or `Guest`. Guest can authenticate but cannot use operational APIs. Remove `loginWithPoliceId` entirely.

Logout must call the server before clearing frontend state:

```js
await api("/auth/logout", { method: "POST" });
accessToken = null;
```

## Roster and profile

```js
const roster = await api(
  `/members?query=${encodeURIComponent(query)}` +
  `&empFilter=${encodeURIComponent(empFilter)}` +
  `&discFilter=${encodeURIComponent(discFilter)}` +
  `&deptFilter=${encodeURIComponent(deptFilter)}` +
  `&genRangeFilter=${encodeURIComponent(genRangeFilter)}` +
  `&page=1&limit=500`
);

renderRoster(roster.groupedByGeneration);
```

Profile/disciplinary summary:

```js
const summary = await api(`/members/by-police-id/${encodeURIComponent(policeId)}/summary`);
// summary.info, summary.penalties, summary.seriousWarning
```

Create members:

```js
await api("/members/bulk", {
  method: "POST",
  body: { members: membersData.map(m => ({
    policeId: m.policeId,
    generation: Number(m.gen),
    fullName: m.fullName,
    rank: m.rank || "",
    medicalPrefix: m.medicalPrefix || "",
    department: m.department || "",
    unit: m.unit || ""
  })) }
});
```

Profile edits use stable `summary.info.id`; never send `rowIndex`, `oldName`, or updater identity:

```js
await api(`/members/${summary.info.id}`, {
  method: "PATCH",
  body: { generation: Number(generation), fullName }
});

await api(`/members/${summary.info.id}/status`, {
  method: "PATCH",
  body: { employmentStatus: "ปลดราชการ" }
});
```

## Penalty creation

Populate type controls from `GET /penalty-types` and retain each item’s `id`. Use a multiple-select control if multiple actions are required.

```js
const form = new FormData();
form.set("policeId", policeId);
for (const id of selectedPenaltyTypeIds) form.append("actionTypeIds", id);
form.set("reason", reason);
form.set("durationType", durationType); // permanent | months | date_range
if (months) form.set("months", String(months));
if (startDate) form.set("startDate", startDate); // YYYY-MM-DD
if (endDate) form.set("endDate", endDate);
if (fineAmount !== "") form.set("fineAmount", String(fineAmount));
form.set("evidence", file);

const result = await api("/penalties", { method: "POST", body: form });
if (result.penaltyCount >= 3 || result.isPermanent) {
  // Show the serious disciplinary warning. Do not auto-retire the member.
}
```

Only JPEG, PNG, and WebP files up to `EVIDENCE_MAX_BYTES` are accepted. The server derives the actor and MIME type. Evidence is retrieved through the returned `/api/v1/evidence/:id` URL while authenticated.

Pardon:

```js
const result = await api(`/penalties/${penaltyId}/pardon`, {
  method: "POST",
  body: { reason: pardonReason }
});
```

## Catalog, users, audit, and metadata

```js
const rules = await api("/penalty-rules");
await api("/penalty-rules", { method: "POST", body: rule });
await api(`/penalty-rules/${rule.id}`, { method: "PATCH", body: rule });
await api(`/penalty-rules/${rule.id}`, { method: "DELETE" });

const users = await api("/users?page=1&limit=100");
await api(`/users/${user.id}/role`, { method: "PATCH", body: { role: "Admin" } });

const audit = await api("/audit-logs?page=1&limit=50");
const departments = await api("/metadata/departments");
```

Catalog writes and penalty-type writes are restricted to Owner/Admin. Role changes follow a stricter Owner/Screener hierarchy. The frontend should use 403 responses as the authority rather than locally cached roles.

## Error handling

Validation responses contain field details:

```json
{
  "status": 400,
  "message": "invalid request",
  "error": {
    "code": "VALIDATION_ERROR",
    "details": [{"field":"policeId","message":"is required"}]
  }
}
```

Handle at least `UNAUTHORIZED`, `FORBIDDEN`, `NOT_FOUND`, `CONFLICT`, `VALIDATION_ERROR`, `PAYLOAD_TOO_LARGE`, and `INTERNAL_ERROR`. Never display raw strings using `innerHTML`; assign user-controlled text through `textContent` or an escaping template layer.
