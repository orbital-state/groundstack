
# GRS-STATE-0001: Migration Control (golang-migrate) + Control Plane Bootstrap API

**Status**: drafted
**Created**: 2026-03-03
**Authors**: TBD
**Discussion**: TBD

## Abstract
Centralize Postgres schema management using a dedicated migration tool (e.g. `golang-migrate`) and stop having example scripts mutate the database directly. Instead, introduce a small Groundstack “control plane” HTTP API for bootstrap operations (tenants, subscriptions, and other emulator-internal objects).

## Motivation

Today, [examples/basic/bootstrap.sh](../../../examples/basic/bootstrap.sh) generates IDs and then registers tenant/subscription rows by running Python that talks directly to Postgres.

That approach has a few downsides:
- Schema becomes implicit and scattered across ad-hoc scripts.
- The DB becomes part of the example contract, which makes future refactors (tables, constraints, migrations) risky.
- It bypasses the API surface we ultimately want Terraform and other tooling to interact with.

We want:
1) One central, repeatable migration mechanism that works in local dev and in Docker Compose.
2) Examples to depend on the API (Groundstack’s control plane), not on direct DB access.

## Specification

### 1) Central DB migrations via `golang-migrate`

#### Goals
- A single, canonical migration history and schema definition.
- Migrations runnable:
	- from a developer machine (optional), and
	- as a Compose step/service (primary).
- No migration logic hidden in example scripts.

#### Migration location and format
Add a repository-owned migrations directory:

```
db/migrations/
	000001_init.up.sql
	000001_init.down.sql
	...
```

Notes:
- Use numeric, monotonic versions (supported well by `migrate`).
- Keep migrations pure SQL.

#### How migrations are applied
Preferred: a dedicated Compose service that runs migrations and exits.

Conceptually:
- `postgres` starts and becomes healthy
- `migrate` runs `migrate -path /migrations -database "$GS_DB_URL" up`
- the rest of the stack may depend on `migrate` having completed successfully

This keeps DB bootstrapping out of application code and makes schema evolution explicit and reviewable.

### 2) Groundstack control plane API for bootstrap objects

#### Goals
- Examples (and later, other tooling) create/ensure tenants and subscriptions via HTTP.
- No direct database manipulation from `examples/basic/scripts/*.py`.
- Keep it intentionally small: only what’s needed for bootstrap.

#### Design constraints
- This is not an Azure API. It is a Groundstack-owned API.
- It must be safe to run locally. For MVP, it can be unauthenticated but should be:
	- bound to localhost by default, or
	- only exposed in a Docker profile intended for dev.
- Operations must be idempotent to support repeatable bootstrap.

#### Proposed endpoints (MVP)
Prefix all endpoints with `/_groundstack` to avoid collisions with Azure-shaped routes.

- `PUT /_groundstack/tenants/{tenantId}`
	- Upsert tenant.
	- Body may include `{ "displayName": "...", "meta": { ... } }`.

- `PUT /_groundstack/subscriptions/{subscriptionId}`
	- Upsert subscription.
	- Body includes `{ "tenantId": "...", "displayName": "...", "meta": { ... } }`.

- `GET /_groundstack/tenants/{tenantId}` (debug/read-back)
- `GET /_groundstack/subscriptions/{subscriptionId}` (debug/read-back)

Semantics:
- `PUT` is idempotent.
- Missing referenced tenant on subscription upsert should either:
	- auto-create (permissive mode), or
	- return a clear `400` (strict mode).

#### Storage model
Tenants and subscriptions are first-class rows in Postgres (as already implied by the design in [doc/design.md](../../../doc/design.md)).

The control plane API is the only supported way (besides migrations) to create and mutate these rows.

### 3) Changes to examples/basic bootstrap

#### Current
[examples/basic/bootstrap.sh](../../../examples/basic/bootstrap.sh) does:
- `gen_ids.py`
- `register_ids.py` (direct DB writes)

#### Proposed
Bootstrap becomes:
1) Bring up `postgres` (and optionally the full stack)
2) Run DB migrations via `migrate`
3) Generate IDs (or request IDs from the control plane API)
4) Call Groundstack control plane API to upsert:
	 - tenant
	 - subscription

This makes `examples/basic` depend on an API contract, not a DB contract.

### Backwards Compatibility
- Direct DB bootstrap scripts are deprecated.
- During transition we may keep the Python scripts but change their implementation to call the control plane API instead of Postgres.

### Security Considerations
- Avoid exposing the control plane API publicly.
	- Bind to `127.0.0.1` by default in non-container dev.
	- In Compose, expose it only on the bridge network unless explicitly published.
- Avoid logging secrets. If a future auth layer is added, ensure request logging redacts tokens.

### Deployment / Activation
Primary activation path: Docker Compose.

Expected Compose sequence (conceptual):
- `postgres` (healthy)
- `migrate` (run-once)
- `turquoise-api` (serves Azure frontends + `/_groundstack` control plane endpoints)

For local non-Compose dev, a developer may run:
- `migrate` CLI against `GS_DB_URL`
- then start `turquoise-api`

### Reference Implementation (planned)
- Add `db/migrations` and wire a `migrate` step into [examples/docker-compose.yml](../../../examples/docker-compose.yml).
- Implement `/_groundstack/*` handlers in the Go API.
- Update `examples/basic/scripts/*.py` to call HTTP endpoints rather than using `psycopg`/`psql`.

### Test Plan
- Migration correctness:
	- Start a fresh Postgres container and run `migrate up`.
	- Validate expected tables/constraints exist.

- Bootstrap idempotency:
	- Run `examples/basic/bootstrap.sh` twice; the second run should succeed and not duplicate rows.

- API tests (Go):
	- Unit/integration tests for tenant/subscription upsert handlers.

## Changelog
- 2026-03-03: drafted

