# OIG Backend Agent Guidance

## Scope

This repository is migrating a legacy Google Apps Script application to a Go REST API. Follow the phased plan in `README.md`; do not implement later phases implicitly.

## Required boundaries

- Keep the dependency flow `router -> middleware/handler -> service -> repository -> MongoDB`.
- Handlers only parse, validate, and map HTTP requests and responses.
- Services own business rules and authorization-aware decisions.
- Repositories own database access and accept `context.Context`.
- Do not serialize MongoDB persistence models as public API contracts; use DTOs.
- Derive actor identity and role from authenticated server context, never request bodies.
- Use stable string resource IDs and UTC timestamps. Never use sheet row numbers as IDs.
- Significant mutations must write server-generated audit records.
- Do not store secrets, access tokens, production data, or credentials in the repository.

## Verification

Run `make fmt`, `make swagger`, and `make verify` for Go changes. Add focused standard-library tests for changed behavior. Integration tests that require MongoDB must be clearly labeled and must not target production databases.

## Legacy behavior

The expected source file is `legacy/google-apps-script.js`. If it is absent, do not infer backend rules from UI behavior alone. Record assumptions and unresolved questions in `docs/legacy-migration.md`.
