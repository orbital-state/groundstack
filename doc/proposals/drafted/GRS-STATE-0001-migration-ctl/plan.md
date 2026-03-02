# Implementation plan: Migration control + control plane bootstrap API

This plan complements the spec in [README.md](README.md).

## Outcomes / Definition of Done

When this plan is implemented:
- The database schema is defined only by repo-owned migrations.
- `examples/basic/bootstrap.sh` never connects to Postgres directly.
- Tenants and subscriptions are created/ensured via a Groundstack HTTP API (`/_groundstack/...`).
- Running bootstrap repeatedly is idempotent.

## Non-goals (for this plan)

- Full Azure fidelity for tenant/subscription creation (this is intentionally not an Azure API).
- AuthN/AuthZ for the control plane API (can be added later; keep it local-only for MVP).
- Reworking Terraform flows beyond what’s needed to bootstrap consistent IDs.

## Step 1 — Add `golang-migrate` as the migration mechanism

### 1.1 Choose how `migrate` runs

Preferred (Compose-first): use a container image that provides the `migrate` CLI.

Two viable options:
- Use the official `migrate/migrate` image.
- Build a tiny image in-repo that installs `migrate`.

Decision guideline:
- If we want fastest iteration and minimal repo changes, use `migrate/migrate`.
- If we want full reproducibility behind firewalls and pinned tooling, build in-repo.

### 1.2 Create migrations directory

Add:

```
db/migrations/
```

Initial migrations should include (at minimum):
- `schema_migrations` (managed by `migrate`)
- `tenants`
- `subscriptions`

Recommended tables (minimal):

- `tenants(id text primary key, created_at timestamptz not null default now(), meta jsonb not null default '{}'::jsonb)`
- `subscriptions(id text primary key, tenant_id text not null references tenants(id), created_at timestamptz not null default now(), meta jsonb not null default '{}'::jsonb)`

### 1.3 Wire migrations into Docker Compose

Update [examples/docker-compose.yml](../../../examples/docker-compose.yml):

- Add a `migrate` service that:
  - depends on `postgres` health
  - mounts `db/migrations` into the container
  - runs `migrate up`

Example shape (exact commands may vary):

- `migrate -path /migrations -database "$GS_DB_URL" up`

Then:
- Make `turquoise-api` depend on `migrate` having succeeded.

Note: Compose’s “depends_on: condition: service_completed_successfully” is supported in newer Compose versions; if not available, keep `turquoise-api` depending only on Postgres and have bootstrap run `docker compose run --rm migrate` before starting the stack.

### 1.4 Local dev path (optional)

Document the CLI usage for local runs:

- `migrate -path db/migrations -database "$GS_DB_URL" up`
- `migrate ... down 1`

## Step 2 — Implement `/_groundstack` control plane endpoints

### 2.1 Routing

Add a small router group for `/_groundstack/*` in the Go API (served by the same HTTP server as the Azure-shaped endpoints).

These endpoints should not depend on `Host` matching Azure domains. They should work with:
- direct dev usage (`curl http://localhost:18080/_groundstack/...`), and
- within the Compose network (`http://turquoise-api:8080/_groundstack/...`).

### 2.2 Endpoints (MVP)

Implement:
- `PUT /_groundstack/tenants/{tenantId}`
- `GET /_groundstack/tenants/{tenantId}`
- `PUT /_groundstack/subscriptions/{subscriptionId}`
- `GET /_groundstack/subscriptions/{subscriptionId}`

Behavior:
- `PUT` is idempotent (upsert).
- `GET` returns `404` if not found.
- Return JSON with stable shapes; keep it simple.

### 2.3 DB operations

Implement store functions in `internal/state`:
- `UpsertTenant(ctx, tenantId, meta)`
- `UpsertSubscription(ctx, subscriptionId, tenantId, meta)`
- `GetTenant(ctx, tenantId)`
- `GetSubscription(ctx, subscriptionId)`

Use `INSERT ... ON CONFLICT (id) DO UPDATE`.

### 2.4 Strict/permissive behavior (optional toggle)

If `GS_MODE=strict`:
- Reject subscription upsert when tenant is missing.

If `GS_MODE=permissive`:
- Auto-create missing tenant on subscription upsert.

## Step 3 — Convert `examples/basic/scripts` to use the API

### 3.1 Replace direct DB registration

Update [examples/basic/bootstrap.sh](../../../examples/basic/bootstrap.sh):

Replace:
- `register_ids.py` (DB)

With:
- an HTTP call step to `turquoise-api` control plane endpoints.

A minimal approach is to keep Python but switch it to HTTP:
- `register_ids.py` becomes “register_ids_via_api.py” (or rename in-place) and:
  - reads `.env.local` IDs
  - `PUT`s tenant and subscription via `/_groundstack/...`

### 3.2 Decide where API is reachable from bootstrap

Two supported modes:

1) Host machine bootstrap (default today)
- `bootstrap.sh` runs on the host.
- It talks to `http://localhost:18080` (or whatever port we publish for `turquoise-api`).

2) Containerized bootstrap
- Add a `bootstrap` service in Compose that runs the bootstrap steps inside the network.
- It talks to `http://turquoise-api:8080`.

Start with (1) to minimize moving parts.

## Step 4 — Update docs and remove DB coupling

- Update [examples/basic/README.md](../../../examples/basic/README.md) to describe:
  - migrations as a standard step
  - control plane API bootstrap
- Remove any mention of connecting to Postgres directly for schema creation.

## Step 5 — Tests

Minimum tests to add:
- Migration smoke test (can be a simple script or Go integration test) that runs `migrate up` on a fresh DB.
- Go handler tests for `/_groundstack` endpoints using `httptest`.

## Rollout / sequencing

Recommended PR sequence:
1) Add migrations + `migrate` compose service (no API changes yet).
2) Add `/_groundstack` handlers + store functions.
3) Convert example scripts to use HTTP.
4) Delete the old direct-DB registration code.

