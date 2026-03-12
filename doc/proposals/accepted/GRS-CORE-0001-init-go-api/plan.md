
# Implementation plan: Go API scaffold (turquoise) MVP

This document is a detailed, implementation-oriented plan for the **initial** Go API scaffold that provides:
- Host-based routing to ARM + AAD-lite + Graph stub frontends
- Postgres-backed state (resources + operations + request log)
- AAD-lite token issuance compatible with Terraform’s AzureRM provider
- ARM-style CRUD + LIST for resource groups and generic resources
- Minimal long-running operation (LRO) support for selected resource types

It complements the proposal spec in:
- [README.md](README.md)
- [doc/design.md](../../../design.md)

## Guiding principles (implementation)

1) **Terraform-first**
- Implement the smallest compatible surface that unblocks `examples/basic`.
- Prefer behaviors that make Terraform converge (stable reads, deterministic lists, idempotent PUT).

2) **Azure-shaped, not Azure-complete**
- Match common response envelopes, headers, and LRO patterns.
- Be strict only where the client relies on it; otherwise accept and record.

3) **Debuggability over perfect fidelity**
- Persist sanitized request traces.
- Emit correlation IDs.
- Prefer explicit “not implemented” errors over silent nonsense.

## Outcomes / Definition of Done (plan-level)

When this plan is implemented:
- A single Go server can be run locally (direct HTTP) and under Docker Compose.
- Terraform in [examples/basic](../../../examples/basic) can `init/apply/apply/destroy` against hijacked endpoints.
- Repeated `apply` converges (no perpetual diffs caused by unstable IDs, list ordering, or missing read-after-write).
- Request traces are persisted without secrets.

## Non-goals for the initial implementation

- Blob data-plane behaviors (beyond placeholders if the provider probes endpoints).
- Real IAM authorization checks.
- Perfect provider registration realism.
- Full ARM type validation for every provider/type.

## Repository / package scaffold

The Go module already exists: `github.com/orbital-state/groundstack`.

Recommended layout (minimal but maintainable):

```
cmd/turquoise-api/
	main.go

internal/config/
	config.go

internal/httpserver/
	server.go           # net/http wiring + graceful shutdown
	middleware.go       # request id, recovery, request logging
	hostrouter.go       # dispatch by Host

internal/frontends/
	aad/
		token.go          # token endpoint handlers
	arm/
		routes.go         # ARM mux + route patterns
		resources.go      # CRUD/list
		errors.go         # Azure-ish error envelopes
		lro.go            # LRO response helpers + op polling endpoints
	graph/
		stub.go

internal/state/
	db.go               # pgxpool
	migrate.go          # minimal migration runner (go:embed SQL)
	resources.go        # resource store
	operations.go       # LRO operations store
	requestlog.go       # request trace store

internal/types/
	arm.go              # ARM resource shapes used in responses

internal/util/
	jsoncanon.go        # stable JSON encoding / hashing
	redaction.go        # sanitization helpers
	ids.go              # resource id parsing/canonicalization
```

Dependency policy (initial MVP):
- Prefer standard library (`net/http`, `encoding/json`, `crypto/*`).
- Existing dependencies already in `go.mod`: `github.com/golang-jwt/jwt/v5`, `github.com/jackc/pgx/v5`.
- Optional: add a small router (e.g. `chi`) **only if** route parsing becomes too error-prone with `ServeMux`.
	- If added, keep it to a single router dependency.

## Configuration & runtime modes

### Environment variables

Minimal set (no defaults except where safe):

- `GS_LISTEN_ADDR` (default `:8080`): Go API listen address
- `GS_DB_URL` (required): Postgres connection string
- `GS_AUTH_HS256_SECRET` (required for token issuance): shared secret used to sign AAD-lite tokens

Feature toggles:
- `GS_MODE` (default `permissive`): `permissive` | `strict`
- `GS_REQUEST_LOG_ENABLED` (default `true`)
- `GS_REQUEST_LOG_BODY_MAX_BYTES` (default `32768`)
- `GS_LRO_ENABLED` (default `true`)
- `GS_LRO_POLL_COUNT` (default `2`): number of polls to return `InProgress` before `Succeeded` (deterministic)

### Local dev (direct HTTP)

Support a mode where the server is reachable without DNS/TLS interception:
- Use `curl`/tests with an explicit `Host` header (e.g. `Host: management.azure.com`).
- This mode should be the fastest for unit/integration tests.

### “Realistic” mode (DNS + TLS via edge proxy)

This plan assumes an edge proxy terminates TLS and forwards plain HTTP to `turquoise-api`.
The Go server should not need TLS support for MVP.

## HTTP server & routing

### Common middleware

Implement these as standard `net/http` middleware:

1) **Request ID + correlation IDs**
- If the request contains `x-ms-correlation-request-id`, propagate it; else generate a UUID.
- Also generate a stable `x-ms-request-id` per request.
- Include these in responses for easier debugging.

2) **Recovery**
- Catch panics and return a JSON Azure-ish error envelope.

3) **Request trace capture (sanitized)**
- Capture `host`, `method`, `path`, `query`, `status`, `duration_ms`.
- Store a body snapshot **only when**:
	- `GS_REQUEST_LOG_ENABLED=true`, and
	- body is <= `GS_REQUEST_LOG_BODY_MAX_BYTES` (truncate otherwise).

4) **Auth context extraction (permissive)**
- If `Authorization: Bearer ...` exists, parse the token best-effort (no enforcement).
- Extract `tid`, `sub`, `aud`, `iss` when decodable; store in request log metadata.

### Host router (frontend dispatch)

Dispatch by host (strip port; ignore case):

- `management.azure.com` => ARM frontend
- `login.microsoftonline.com` => AAD-lite frontend
- `graph.microsoft.com` => Graph stub frontend
- Anything else => 404 (Azure-ish JSON error with `code=HostNotSupported`)

Important: when running under an edge proxy, requests will arrive with the original `Host` preserved.

## AAD-lite (token issuance)

### Goals

- Provide OAuth2 token endpoint(s) compatible with common AzureRM provider flows.
- Be permissive in accepted parameters but deterministic in outputs.
- Do not implement full tenant/app registration.

### Endpoints to implement (initial)

Support both v1 and v2 token endpoints because tooling varies:

1) `POST /{tenantId}/oauth2/token`
2) `POST /{tenantId}/oauth2/v2.0/token`

Where `tenantId` is a UUID-ish string or `common`.

Content types to accept:
- `application/x-www-form-urlencoded` (primary)
- `application/json` (best-effort; some tooling uses it)

### Request parsing (permissive but sane)

Accept at minimum `grant_type=client_credentials`.

Supported parameter names (accept synonyms; ignore unknowns):
- `client_id` (required)
- `client_secret` (required but not validated; only used for redaction rules)
- `grant_type` (required)
- Audience:
	- v1 style: `resource=https://management.azure.com/`
	- v2 style: `scope=https://management.azure.com/.default`

If `aud` cannot be inferred, default to `https://management.azure.com/`.

### Response shape

Return `200` with a JSON document similar to Azure:

```json
{
	"token_type": "Bearer",
	"expires_in": 3600,
	"ext_expires_in": 3600,
	"access_token": "<jwt-like>"
}
```

Do not include refresh tokens.

### Token format (JWT-like, HS256)

Sign with `HS256` using `GS_AUTH_HS256_SECRET`.

Claims (MVP):
- `iss`: `https://login.microsoftonline.com/{tenantId}/v2.0`
- `aud`: `https://management.azure.com/`
- `tid`: tenantId
- `sub`: clientId
- `appid`: clientId (Azure-ish convenience)
- `iat`, `nbf`, `exp`
- Optional (debug value): `x_gs_mode`: `permissive|strict`

Implementation detail:
- The token’s role is primarily to make logs readable and allow future auth enforcement.
- ARM handlers should accept any bearer token in MVP.

### Error handling

When required parameters are missing, return `400` with an Azure-ish OAuth error:

```json
{ "error": "invalid_request", "error_description": "..." }
```

### Sanitization requirements

Never store `client_secret` or full access tokens in the request log.
- Store only a token fingerprint (e.g. first 8 chars of SHA-256 of token string).

## ARM frontend (management.azure.com)

### Goals

- Minimal ARM-like control plane sufficient for Terraform’s CRUD and state refresh.
- Stable IDs and deterministic list ordering.
- Azure-ish error envelopes.

### Minimum endpoints (MVP)

Implement these families first:

1) **Cloud metadata**
- `GET /metadata/endpoints?api-version=2020-06-01`
	- Return a minimal document describing ARM endpoint base URIs.
	- Rationale: many SDKs/providers probe this.

2) **Resource groups**
- `PUT /subscriptions/{subId}/resourceGroups/{rgName}?api-version=...`
- `GET /subscriptions/{subId}/resourceGroups/{rgName}?api-version=...`
- `DELETE /subscriptions/{subId}/resourceGroups/{rgName}?api-version=...`
- `GET /subscriptions/{subId}/resourceGroups?api-version=...`

3) **Generic resource CRUD**
- `PUT /subscriptions/{subId}/resourceGroups/{rg}/providers/{ns}/{typeAndName...}?api-version=...`
- `GET  /subscriptions/{subId}/resourceGroups/{rg}/providers/{ns}/{typeAndName...}?api-version=...`
- `DELETE ...`

4) **Generic list under a type**
- `GET /subscriptions/{subId}/resourceGroups/{rg}/providers/{ns}/{typePath}?api-version=...`
	- Where `typePath` may be single-segment (e.g. `storageAccounts`) or multi-segment for nested types.

Notes on routing ambiguity:
- ARM uses the same prefix for both collection and item paths; the differentiator is whether the path ends with a name segment.
- For MVP, treat the path as pairs after `{ns}`: `(type, name)` repeating.
	- If the number of remaining segments is odd: it is a collection list for the trailing type path.
	- If even: it is an instance resource.

### API version handling

For MVP:
- If `api-version` is missing:
	- permissive mode: accept and record `api_version=""`
	- strict mode: return `400` with code `MissingApiVersionParameter`
- Store `api_version` on the resource row for later debugging.

### Resource ID parsing & canonicalization

Azure resource IDs are case-insensitive. Terraform may produce different casing across calls.

Plan:
- Compute a **normalized ID** (`id_norm`) by lowercasing the full resource ID string.
- Use `id_norm` as the primary key for lookups and uniqueness.
- Store `id` (display form) as the ID returned in responses.
	- On first PUT, set `id` to a canonical-segment form:
		- Segment names cased consistently: `/subscriptions/.../resourceGroups/.../providers/...`
		- Preserve caller-provided casing for subscriptionId/resourceGroup/resourceName segments.
	- On later GET/LIST, always return the stored `id` for stability.

Also extract and store:
- `subscription_id`
- `resource_group`
- `provider_namespace`
- `resource_type` (top-level type segment after namespace)
- `name` (top-level name)

### CRUD semantics

#### PUT (create/update)

- Must be idempotent.
- Accept JSON body; unknown fields are stored.
- Required-ish ARM fields:
	- `location` often required by Terraform for many resource types.
	- In permissive mode, accept missing `location` and store null.
- Store:
	- `properties` (opaque JSON)
	- `tags` (object; default `{}`)
	- `location` (string)

Response:
- `200` or `201` (either is typically acceptable; prefer `200` on update, `201` on create)
- Include fields:
	- `id`, `name`, `type`, `location`, `tags`, `properties`, `etag`

ETag plan:
- Compute `etag` as a hash of a canonical JSON representation of the stored resource document.
- Return `etag` as both:
	- JSON field `etag`, and
	- response header `ETag`.

#### GET (read)

- Return `200` with the stored document.
- If not found:
	- permissive mode: return Azure-ish not found error with `404`
	- strict mode: same `404` but with stricter codes/messages

#### DELETE

- Must be idempotent.
- If resource exists, delete it and return `200` or `204`.
- If missing, return `204` in permissive mode (to reduce terraform churn) and `404` in strict mode.

### LIST semantics & paging

Terraform expects stable list responses.

Plan:
- Deterministic ordering: sort by `id` (display id) or by `id_norm`.
- Page size: fixed (e.g. 100) unless `top` query param is provided.
- Implement `skiptoken` as an opaque cursor (base64-encoded last `id_norm`).
- `nextLink` should be an absolute URL under `https://management.azure.com/...` so clients can follow it.

Response shape:

```json
{
	"value": [ { /* resources */ } ],
	"nextLink": "https://management.azure.com/...&$skiptoken=..." 
}
```

If there is no next page, omit `nextLink`.

### ARM error envelope

Use a consistent envelope similar to Azure:

```json
{
	"error": {
		"code": "ResourceNotFound",
		"message": "...",
		"details": []
	}
}
```

Include `x-ms-request-id` and correlation headers on error responses too.

## Graph stub frontend (graph.microsoft.com)

Purpose: avoid hard failures when tooling probes Graph.

MVP behavior:
- Default: return `501` with a clear JSON error envelope:
	- `code=NotImplemented`
	- message includes method + path
- Allowlist any endpoint(s) observed in Terraform workflows to return minimal stable JSON (added incrementally).

Request traces are the primary tool here: once we observe a required endpoint, we implement it.

## Postgres state & migrations

### Connection handling

- Use `pgxpool.Pool`.
- All handlers receive a `Store` interface (allows unit tests with fakes).
- Use per-request timeouts (context deadlines) to avoid stuck requests.

### Migration strategy (minimal, no external tooling)

- Create a `schema_migrations` table.
- Store SQL migration files in `internal/state/migrations/*.sql`.
- Use `//go:embed` to include them in the binary.
- Apply migrations on startup:
	- Acquire a DB advisory lock.
	- Apply pending migrations in order.

### Tables (MVP)

This is the minimum viable schema; adjust only when Terraform forces it.

**Important**: even though MVP auth is permissive, we still want the emulator to have a notion of “known” tenants/subscriptions so that:
- request traces can be grouped by tenant/subscription
- strict-mode can be added later without a schema break
- examples can explicitly “register” the IDs they use

`tenants`:

- `id text primary key` (tenant ID; UUID string)
- `created_at timestamptz not null default now()`
- `meta jsonb not null default '{}'::jsonb`

`subscriptions`:

- `id text primary key` (subscription ID; UUID string)
- `tenant_id text not null references tenants(id)`
- `created_at timestamptz not null default now()`
- `meta jsonb not null default '{}'::jsonb`

`resources` (single source of truth for ARM objects):

- `id_norm text primary key` (lowercased resource ID)
- `id text not null` (display resource ID returned in responses)
- `subscription_id text not null`
- `resource_group text`
- `provider_namespace text`
- `resource_type text`
- `name text`
- `location text`
- `api_version text`
- `etag text`
- `provisioning_state text` (e.g. `Succeeded|Updating|Deleting`)
- `properties jsonb not null default '{}'::jsonb`
- `tags jsonb not null default '{}'::jsonb`
- `created_at timestamptz not null default now()`
- `updated_at timestamptz not null default now()`

Indexes:
- `(subscription_id, resource_group, provider_namespace, resource_type)` for list queries.
- `(subscription_id, provider_namespace, resource_type)` for future subscription-level listings.

`operations` (LRO state):

- `operation_id text primary key`
- `resource_id_norm text not null references resources(id_norm)` (nullable only if op is created before resource exists)
- `kind text not null` (`PUT|DELETE`)
- `status text not null` (`InProgress|Succeeded|Failed`)
- `poll_count int not null default 0`
- `error jsonb`
- `created_at timestamptz not null default now()`
- `updated_at timestamptz not null default now()`

`request_log` (sanitized trace):

- `id bigserial primary key`
- `ts timestamptz not null default now()`
- `host text not null`
- `method text not null`
- `path text not null`
- `query text not null default ''`
- `status int not null`
- `duration_ms int not null`
- `correlation_id text`
- `request_id text`
- `token_fp text` (token fingerprint; optional)
- `tenant_id text` (optional)
- `subject text` (optional)
- `body jsonb` (optional; redacted)

### Store operations

Implement these store functions first:

- `UpsertTenant(ctx, tenantID) error`
- `UpsertSubscription(ctx, subscriptionID, tenantID) error`

- `UpsertResource(ctx, ResourceRecord) (created bool, err error)`
- `GetResource(ctx, idNorm) (ResourceRecord, found bool, err error)`
- `DeleteResource(ctx, idNorm) (deleted bool, err error)`
- `ListResourcesByType(ctx, subId, rg, ns, typePath, pageSize, cursor) ([]ResourceRecord, nextCursor, err)`
- `ListResourceGroups(ctx, subId, pageSize, cursor) (...)`

- `CreateOperation(ctx, OperationRecord) error`
- `GetOperation(ctx, operationID) (OperationRecord, found bool, err error)`
- `BumpOperationPoll(ctx, operationID) (OperationRecord, err error)`
- `MarkOperationSucceeded/Failed(ctx, operationID, ...)`

- `InsertRequestLog(ctx, RequestLogRecord) error`

## LRO (long-running operations)

### Scope

Enable LRO for:
- Storage accounts: `Microsoft.Storage/storageAccounts`
- Virtual machines: `Microsoft.Compute/virtualMachines`

Everything else is synchronous initially.

### Behavior contract

On LRO-eligible PUT/DELETE:
- Create/update the resource row immediately with:
	- `provisioning_state=Updating` (PUT) or `Deleting` (DELETE)
- Create an `operations` row with `status=InProgress` and deterministic `poll_count=0`.
- Respond `202` and include at least:
	- `Azure-AsyncOperation: https://management.azure.com/subscriptions/{subId}/providers/Microsoft.Resources/operationStatuses/{opId}?api-version=2020-06-01`
	- `Location` (optional; can mirror async-op URL)

Operation status endpoint:
- `GET /subscriptions/{subId}/providers/Microsoft.Resources/operationStatuses/{opId}?api-version=...`
- Response `200` JSON:

```json
{
	"status": "InProgress" | "Succeeded" | "Failed",
	"error": { "code": "...", "message": "..." }
}
```

Deterministic completion:
- Each poll increments `poll_count`.
- After `GS_LRO_POLL_COUNT` polls, transition to `Succeeded` and set resource `provisioning_state=Succeeded`.

Rationale: deterministic tests and fewer timing flakes.

### Failure injection (not in MVP)

Do not implement random failures. If failures are needed later, gate behind a config flag.

## Request logging & sanitization

### What to store

Store enough to debug Terraform/provider behavior:
- `host`, `method`, `path`, `query`, `status`
- correlation IDs
- redacted request body (if JSON/form and small)

### What to never store

- `Authorization` header values
- OAuth `client_secret`
- full access tokens
- any headers that look like secrets (e.g. `x-ms-*key*`, `x-ms-sas*`)

### Redaction rules

1) For `application/x-www-form-urlencoded`:
- Parse into key/value pairs.
- Replace values for keys matching (case-insensitive):
	- `client_secret`, `password`, `assertion`, `refresh_token`, `access_token`
	- any key containing `secret` or `token`

2) For JSON bodies:
- Best-effort parse to `map[string]any`.
- Recursively redact fields with key names matching:
	- `password`, `secret`, `token`, `key`, `signature`, `sas`

3) Truncation:
- If body exceeds limit, store `{ "truncated": true }` plus the first N bytes hashed.

## Terraform smoke test plan

Use [examples/basic](../../../examples/basic) as the canonical MVP acceptance test.

### Expected Terraform behavior

The run should exercise:
- Token acquisition (`login.microsoftonline.com/.../token`)
- RG create/read/list
- Storage account create/read/list (likely LRO-enabled)
- Second apply performs mostly reads and converges

### Execution sequence

Run (containerized Terraform recommended):

1) `terraform -chdir=examples/basic init`
2) `terraform -chdir=examples/basic apply -auto-approve`
3) `terraform -chdir=examples/basic apply -auto-approve` (must converge)
4) `terraform -chdir=examples/basic destroy -auto-approve`

Credentials:
- Provide dummy values for:
	- `ARM_TENANT_ID`
	- `ARM_SUBSCRIPTION_ID`
	- `ARM_CLIENT_ID`
	- `ARM_CLIENT_SECRET`

Groundstack env:
- `GS_AUTH_HS256_SECRET` set to a stable value.

### Debug loop (how we iterate)

If Terraform fails:
- Inspect `request_log` rows for the failing request.
- Add/adjust the minimal endpoint, shape, or header.
- Re-run until the acceptance criteria are satisfied.

## Docker Compose wiring (reference)

This section describes the **intended** Docker wiring for the “realistic” HTTPS interception mode.

### Services

Minimum required:
- `postgres`
- `turquoise-api`

Recommended for realistic Terraform runs:
- `edge-proxy` (TLS termination via mkcert)
- `dns` (CoreDNS overrides for Azure domains)

Optional (for deterministic integration test runs):
- per-example `terraform-*` runners (containers that run Terraform against the stack)

### Network & addressing

Goal: make DNS deterministic (especially for CI).

Plan:
- Create a dedicated Compose network (e.g. `groundstack`) with IPAM.
- Assign **static** IPs to:
	- `dns` (e.g. `172.28.0.53`)
	- `edge-proxy` (e.g. `172.28.0.2`)

Rationale:
- Terraform container can use `--dns 172.28.0.53`.
- CoreDNS can return `172.28.0.2` for intercepted domains.

### mkcert material

Assumption: mkcert is used to generate a locally-trusted CA and leaf certs.

Plan:
- Developer machine:
	- install mkcert root CA into host trust store
	- generate certs for the intercepted names
- `edge-proxy` container mounts:
	- leaf cert/key (PEM)
	- (optionally) full chain if required by proxy

Per-example terraform runner containers mount:
	- mkcert root CA PEM and installs it into container trust

Minimum certificate names to cover:
- `management.azure.com`
- `login.microsoftonline.com`
- `graph.microsoft.com`

If/when storage data plane is added:
- `*.blob.core.windows.net`

### Edge proxy routing

Requirements:
- Single listener on `:443`.
- Route by SNI/Host header and forward to `turquoise-api:8080` over plain HTTP.

Routing rules:
- `management.azure.com` => `http://turquoise-api:8080`
- `login.microsoftonline.com` => `http://turquoise-api:8080`
- `graph.microsoft.com` => `http://turquoise-api:8080`

Implementation note:
- A single upstream is fine because the Go server already dispatches by `Host`.

### CoreDNS rules (allowlist-only)

Goal: override only the minimum set of Azure domains needed for Terraform.

Plan:
- Start with exact A records (no wildcards) for control-plane names:
	- `management.azure.com` -> `edge-proxy` IP
	- `login.microsoftonline.com` -> `edge-proxy` IP
	- `graph.microsoft.com` -> `edge-proxy` IP

Forward all other queries to an upstream resolver.

If/when storage data plane becomes necessary:
- Add a carefully scoped wildcard for `*.blob.core.windows.net` only.

### Terraform runner container (recommended for MVP verification)

Purpose: run Terraform in a way that naturally uses CoreDNS + trusts mkcert.

Implementation note:
- Define one runner per example (e.g. `terraform-basic`) so each can set its own working directory, env file, and smoke command.

Plan:
- Container joins the same Compose network.
- Container DNS is set to the `dns` service IP.
- Root CA is mounted into `/usr/local/share/ca-certificates/`.

Expected run command inside container:
- `update-ca-certificates || true`
- `terraform -chdir=examples/basic init`
- `terraform -chdir=examples/basic apply -auto-approve`
- `terraform -chdir=examples/basic apply -auto-approve`
- `terraform -chdir=examples/basic destroy -auto-approve`

Acceptance check:
- Second `apply` produces no changes.
- The API returns stable `id`/`name`/`type`/`etag` fields.

## Milestones / work breakdown

## Testable implementation steps (16)

Each step below is structured so it can be validated either by **unit tests** (`go test ./...`) or by running **examples** (primarily [examples/basic](../../../examples/basic)).

### 1) Bootstrap IDs + register tenant/subscription (examples-first)

**Goal**: Make it easy and deterministic to produce the IDs the whole system will use, and ensure the emulator can “know about” those IDs.

**Deliverables**
- Add `examples/basic/scripts/` with:
	- `gen_ids.py`: generates and writes a dotenv file containing `ARM_TENANT_ID`, `ARM_SUBSCRIPTION_ID`, `ARM_CLIENT_ID`, `ARM_CLIENT_SECRET`.
	- `register_ids.py`: idempotently inserts tenant/subscription into Postgres (`tenants`, `subscriptions`).
- Update [examples/basic/README.md](../../../examples/basic/README.md) to document the scripts.

**Implementation notes**
- The registration script should:
	- require `GS_DB_URL`
	- read IDs from the dotenv file (default `examples/basic/.env.local`)
	- use `psql` and `INSERT ... ON CONFLICT DO NOTHING`

**How to test**
- Example-level:
	- Run `python3 examples/basic/scripts/gen_ids.py` and confirm it writes the file.
	- Run `GS_DB_URL=... python3 examples/basic/scripts/register_ids.py` against a Postgres instance that already has the schema (once step 4 is done).

### 2) Create `cmd/turquoise-api` skeleton + health endpoint

**Goal**: Boot a server that can be hit in tests and in Compose.

**Deliverables**
- `cmd/turquoise-api/main.go` starts HTTP server, loads config, connects DB.
- `GET /healthz` returns `200` (non-Azure endpoint; for ops/testing only).

**How to test**
- Unit/integration: `go test` can start the server with `httptest` and call `/healthz`.

### 3) Host router + error envelope baseline

**Goal**: Route by `Host` and always return JSON errors with request IDs.

**Deliverables**
- `Host` dispatch: ARM/AAD/Graph.
- Unknown host => `404` JSON with `code=HostNotSupported`.

**How to test**
- Unit: table test for host routing (strip port, ignore case).

### 4) Postgres migrations runner + initial schema (tenants/subscriptions/resources/operations/request_log)

**Goal**: One command starts the server and the DB schema is ensured.

**Deliverables**
- Embedded SQL migrations.
- `schema_migrations` tracking.
- Tables: `tenants`, `subscriptions`, `resources`, `operations`, `request_log`.

**How to test**
- Integration (DB): run migrations against a real Postgres (docker) and assert tables exist.

### 5) Request ID + correlation middleware

**Goal**: Every response (success or error) has request IDs.

**Deliverables**
- Generate `x-ms-request-id`.
- Propagate or generate `x-ms-correlation-request-id`.

**How to test**
- Unit: middleware wraps a handler and asserts headers present.

### 6) Sanitized request logging to Postgres

**Goal**: Persist traces without secrets.

**Deliverables**
- Insert one row per request with redacted body.
- Never store `Authorization` or `client_secret`.

**How to test**
- Unit: redaction functions.
- Integration: hit a handler with a secret in the body; assert DB contains redacted value.

### 7) AAD-lite token endpoints (v1 + v2)

**Goal**: Terraform can obtain an access token.

**Deliverables**
- `POST /{tenant}/oauth2/token` and `/oauth2/v2.0/token`.
- HS256 JWT-like token with required claims.
- On token issuance, `UpsertTenant` for the tenant.

**How to test**
- Unit: form parsing + claim validation.
- Example: curl with form body returns token.

### 8) ARM metadata endpoints

**Goal**: Avoid client failures on metadata probes.

**Deliverables**
- `GET /metadata/endpoints?api-version=2020-06-01` minimal response.

**How to test**
- Unit: request returns 200 and JSON.

### 9) Resource ID parser + canonicalization utilities

**Goal**: Normalize IDs and extract subscription/rg/ns/type/name.

**Deliverables**
- `id_norm` lowercasing.
- Extract fields and validate basic segment structure.

**How to test**
- Unit: many ID shapes and casing variants.

### 10) ARM resource group CRUD + list

**Goal**: `azurerm_resource_group` can converge.

**Deliverables**
- PUT/GET/DELETE/LIST with stable shapes and deterministic ordering.
- On PUT, `UpsertSubscription` (from path `subId`) with a tenant mapping strategy:
	- permissive: associate subscription to a “last seen tenant” from token claim `tid` when present; else a sentinel tenant.

**How to test**
- Integration: create RG, read it, list it, delete it.
- Example: terraform apply/destroy (once step 14 is done).

### 11) Generic resource CRUD (PUT/GET/DELETE)

**Goal**: arbitrary ARM resources can be stored and read back.

**Deliverables**
- Generic handler using parsed resource ID.
- Stable response fields and ETag.

**How to test**
- Unit: ETag determinism.
- Integration: PUT then GET returns the same stored representation.

### 12) Generic LIST + paging (`nextLink` + `$skiptoken`)

**Goal**: Terraform refresh/list calls don’t flake.

**Deliverables**
- Deterministic ordering.
- Cursor-based paging.

**How to test**
- Unit: paging returns all items exactly once.

### 13) Graph stub frontend baseline

**Goal**: Graph calls don’t crash the run; they produce actionable errors and traces.

**Deliverables**
- Default `501 NotImplemented` with JSON error.

**How to test**
- Unit: host route to Graph + response code.

### 14) LRO operations table + async operation status endpoint

**Goal**: Provide the polling endpoint Terraform expects.

**Deliverables**
- Create operation rows.
- `GET /subscriptions/{subId}/providers/Microsoft.Resources/operationStatuses/{opId}`.

**How to test**
- Integration: create op, poll twice, see status transition.

### 15) Enable LRO for storage accounts + VMs

**Goal**: unblock storage account creation under Terraform if it requires async semantics.

**Deliverables**
- On eligible PUT/DELETE return `202` with `Azure-AsyncOperation`.
- Deterministic poll-count completion.

**How to test**
- Integration: PUT storage account triggers `202` and operation completes.

### 16) End-to-end terraform runner flow (examples/basic)

**Goal**: prove MVP acceptance criteria.

**Deliverables**
- Containerized Terraform run (DNS + mkcert trust).
- `init/apply/apply/destroy` passes.

**How to test**
- Example: run the documented container command and verify second apply is a no-op.

### M0: Scaffold

- Create `cmd/turquoise-api` and `internal/*` packages.
- Implement config loading and server startup.
- Implement host router and basic JSON error response.
- Implement DB connection and migration runner.

### M1: AAD-lite + ARM CRUD + request log

- AAD-lite token endpoints (v1 + v2).
- ARM resource group CRUD/list.
- ARM generic resource CRUD/list.
- Deterministic paging.
- Request logging with sanitization.

### M2: LRO

- Operations table + async operation endpoint.
- LRO behavior for storage accounts and VMs.
- Ensure provider polling completes deterministically.

### M3: Graph stubs (traffic-driven)

- Implement allowlisted Graph endpoints observed during Terraform runs.
- Everything else returns a clear `501 NotImplemented` with request logging.

## Test plan (code-level)

Unit tests (fast, no Docker):
- Resource ID parsing & normalization (`id_norm`, segment handling)
- ETag stability (same input => same etag)
- Paging cursor behavior
- Redaction rules (form + JSON)

Integration tests (optional early, required before calling MVP done):
- Run Postgres in Docker and execute handler flows against a real DB.
- Terraform smoke run as described above.

