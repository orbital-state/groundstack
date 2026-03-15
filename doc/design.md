
# groundstack design (turquoise)

## Abstract
groundstack (“turquoise”) is a local, Docker Compose–based Azure API substitute intended for Terraform-driven prototyping and integration tests. It works by redirecting Azure control-plane/service endpoints (e.g. `management.azure.com`, `login.microsoftonline.com`, `*.blob.core.windows.net`) to a local Go HTTP server that implements a growing subset of Azure behaviors. A Postgres database stores a consistent emulated state so that repeated Terraform runs converge and drift detection behaves predictably.

The guiding principle is: start as a permissive mock (mostly `200`) and evolve toward a consistent, debuggable, stateful emulator where the “shape” of Azure (resource IDs, async operations, error codes, paging, etags) is more important than perfectly matching every internal detail.

## Goals
- Run Terraform with the official `azurerm` provider against a local stack.
- Provide a consistent state model (CRUD + listing + read-after-write) across resources.
- Be “good enough Azure” for typical app/platform prototyping: resource groups, storage, compute-ish objects, databases, identities (IAM), etc.
- Support incremental fidelity: permissive by default, strict mode later.
- Keep the stack modest: Go web server + Postgres + optional edge proxy/DNS.

## Non-goals (initially)
- Full, realistic networking (VNets, routing, NSGs, private endpoints). These can be represented as resources but won’t actually change network connectivity.
- Running real hypervisors/VMs/containers. “VM” resources are control-plane objects only.
- Complete parity with Azure’s undocumented quirks. The target is stable Terraform workflows.

## High-level architecture

### Components (Docker Compose)
- `turquoise-api` (Go): single entrypoint HTTP server.
- `postgres`: persistence for resources, operations, and request traces.
- (Optional) `edge-proxy` (nginx/traefik/caddy/envoy): terminates TLS and forwards to `turquoise-api`.
- (Optional) `dns` (CoreDNS/dnsmasq): wildcard-ish domain overrides to local.

### Current decisions (2026-03-02)
- DNS overrides: **CoreDNS/dnsmasq container + system DNS change**.
- TLS: **edge proxy terminates TLS using a locally-trusted CA (mkcert)**.
- Auth: **JWT-like access token with useful claims** (still permissive in enforcement).
	- Signing: **HS256 shared secret via env var**.
- Observability: **persist request traces in Postgres** (sanitized).
- Initial scope: **ARM + AAD + Graph stubs** (plus service endpoints as needed by Terraform).
- LRO: **storage accounts + VMs**.
- State typing: **typed schemas for a few core resources**, opaque JSON fallback for the rest.

### Request flow
1. Terraform (or SDK/app) makes HTTPS requests to Azure endpoints.
2. Local DNS/hosts/proxy routes those domains to the local stack.
3. `edge-proxy` (if present) handles TLS and forwards plain HTTP to `turquoise-api`.
4. `turquoise-api` routes by `Host` + path into “service frontends”:
	- ARM (Azure Resource Manager): `management.azure.com` and the `/subscriptions/...` resource hierarchy.
	- AAD/OAuth token endpoint: `login.microsoftonline.com` (minimal token issuance).
	- Data plane endpoints: e.g. Blob `*.blob.core.windows.net`.
5. Handlers read/write Postgres state and return Azure-shaped responses.

### Why an edge proxy matters
Terraform and most Azure SDKs talk HTTPS and expect correct SNI + certificates for the real domains. Redirecting `management.azure.com` to `127.0.0.1` requires solving:
- **Wildcard domains**: `/etc/hosts` cannot express `*.azure.com`.
- **TLS**: a local server needs a cert that the client trusts for `management.azure.com`, `login.microsoftonline.com`, etc.

Pragmatic approach:
- Run a local DNS server that answers selected Azure domains with `127.0.0.1`.
- Terminate TLS using a locally-trusted CA (e.g. `mkcert`) in `edge-proxy`.
- Keep `turquoise-api` simple by speaking HTTP internally.

Implementation note:
- The edge proxy should route based on SNI/`Host` so `management.azure.com` and `login.microsoftonline.com` can share the same listener.
- Cert strategy should cover both exact names and wildcard where needed (e.g. `*.blob.core.windows.net`).

## API surface and compatibility strategy

### Tiered fidelity
Implement in tiers so the system is immediately usable but steadily becomes more realistic.

**Tier 0: permissive mock**
- Accept requests, return `200`/`201` with minimal schemas.
- Create placeholder resources in state so that follow-up GETs succeed.

**Tier 1: Terraform-convergent state**
- Correct resource IDs, names, locations, tags.
- Deterministic PUT/PATCH semantics.
- List operations return stable paging.
- Common Azure error shapes (e.g. `CloudError`) for obvious issues.

**Tier 2: Azure-shaped workflows**
- Long-running operations (LRO): `202` + `Azure-AsyncOperation` / `Location` polling.
- ETags (`If-Match`), optimistic concurrency where it matters.
- Provider registration and API versions.

**Tier 3+: service depth**
- Blob API details (SAS, containers, block blobs).
- Compute model details (VM extensions, managed disks).
- Databases (SQL/Postgres) as control-plane resources; optional real Postgres/MySQL instances later.

### Contract-first vs implementation-first
Turquoise is driven by real client traffic (Terraform first). The contract is:
- Azure-like endpoints and response shapes sufficient for the client.
- A stable internal resource model.

We should avoid overfitting to any single Terraform provider version by:
- Recording unknown/extra fields received.
- Responding with superset fields where safe.
- Versioning handlers by `api-version`.

## Domain routing and endpoint mapping

### Options
1. **Local DNS (recommended)**
	- Run `coredns`/`dnsmasq` and configure the machine to use it.
	- Pros: supports many domains cleanly; can wildcard per-zone.
	- Cons: requires system DNS changes.

Concrete approach:
- Start with a small, explicit allowlist of zones we override (e.g. `azure.com`, `windows.net`, `microsoftonline.com`) and only answer for the specific records we need.
- Keep the DNS server authoritative only for those domains; forward everything else to upstream resolvers.
- Prefer CoreDNS “template” or dnsmasq “address=/<domain>/<ip>” style rules, with care to not over-capture unrelated Microsoft properties.

2. **Hosts file (limited)**
	- Manually map a handful of domains (`management.azure.com`, `login.microsoftonline.com`).
	- Pros: simple.
	- Cons: no wildcards; data-plane endpoints explode in count.

3. **Explicit proxy (good for dev)**
	- Configure Terraform/SDK HTTP(S) proxy env vars.
	- Pros: no DNS changes.
	- Cons: not all tooling respects proxies equally; still need TLS MITM.

### Minimum viable domain set (Terraform-centric)
Expect at least:
- `management.azure.com` (ARM)
- `login.microsoftonline.com` (token)

Additionally (chosen scope):
- `graph.microsoft.com` (Graph): start as **stub-first** and only implement the specific endpoints that Terraform/providers actually call in our workflows.

For storage data plane you’ll need:
- `*.blob.core.windows.net` (+ optionally `queue`, `table`, `file`)

## Authentication model (AAD-lite)

Terraform’s `azurerm` provider uses OAuth2 to obtain an access token. For local emulation we can provide a simplified AAD endpoint that:
- Accepts common token requests (`client_credentials`, optionally `password` or `device_code` later).
- Returns JSON with `access_token`, `expires_in`, `token_type`.
- The token will be a JWT-like string (JWS). For early MVP it can be signed by a local keypair and validated by turquoise only.

Authorization (permissions) should start permissive:
- Accept any bearer token.
- Optionally enforce “tenant/subscription exists” in strict mode.

JWT-like token contents (suggested):
- `iss`: `https://login.microsoftonline.com/{tenantId}/v2.0`
- `aud`: `https://management.azure.com/`
- `tid`: tenantId
- `sub`: a stable subject (clientId)
- `exp`/`nbf`/`iat`
- Optional: `x_gs_subscriptions`: array of allowed subscriptionIds

This keeps traces readable (“which tenant/sub did Terraform think it was using?”) without immediately building a full AAD permission model.

## Resource model and state

### Canonical identifiers
Use Azure’s canonical resource ID format as the primary key for everything:

`/subscriptions/{subId}/resourceGroups/{rg}/providers/{namespace}/{type}/{name}`

Nested resources append `/childType/childName` segments.

### Postgres schema (initial)
Keep it normalized enough for consistent behavior, but flexible for unknown properties.

- `tenants(id, created_at, meta jsonb)`
- `subscriptions(id, tenant_id, created_at, meta jsonb)`
- `resources(
	 id text primary key,             -- canonical resourceId
	 subscription_id text not null,
	 resource_group text,
	 provider_namespace text,
	 resource_type text,              -- top-level type
	 name text,
	 location text,
	 api_version text,
	 etag text,
	 provisioning_state text,
	 properties jsonb,
	 tags jsonb,
	 created_at timestamptz,
	 updated_at timestamptz
  )`
- `operations(
	 operation_id text primary key,
	 resource_id text,
	 kind text,                       -- PUT/PATCH/DELETE
	 status text,                     -- InProgress/Succeeded/Failed
	 error jsonb,
	 created_at timestamptz,
	 updated_at timestamptz
  )`
- `request_log(id bigserial, ts timestamptz, host text, method text, path text, status int, correlation_id text, body jsonb)` (optional, but extremely helpful)

Notes:
- `properties` stores the provider-specific payload, mostly opaque.
- We still extract common indexing fields (`subscription_id`, `resource_group`, `provider_namespace`, `resource_type`, `name`) for list queries.

### Consistency rules
- PUT is idempotent: same body ⇒ same stored representation.
- GET after successful PUT returns the stored resource including `id`, `name`, `type`, `location`, `tags`, and `properties`.
- LIST is stable and repeatable: ordering is deterministic (e.g. by `id`).
- DELETE is idempotent: deleting a missing resource returns `204` or `404` depending on strictness.

## Azure Resource Manager (ARM) emulation

### Routing
Requests to `management.azure.com` are routed by path:
- `GET /subscriptions/{subId}/resourcegroups` …
- `PUT /subscriptions/{subId}/resourceGroups/{rgName}` …
- `PUT /{resourceId}` with `api-version=...` …

### API versions
Azure requires `api-version` for most ARM calls. Turquoise should:
- Parse `api-version` and store it with the resource.
- Maintain a registry of supported versions per provider/type.
- In permissive mode, accept unknown versions but keep them recorded.

### Provider registration
Terraform often expects providers to be registered.
Options:
- Always behave as “registered” (simplest).
- Implement `Microsoft.Resources/providers` endpoints and track registration state in DB (more realistic).

### Long-running operations (LRO)
Many Azure PUT/DELETE operations are async.

Minimal implementation:
- For selected resource types, respond with `202` and set `Azure-AsyncOperation: /.../operationStatuses/{opId}`.
- Create an `operations` row as `InProgress`.
- A background worker (or in-process goroutine) transitions to `Succeeded` after a small delay.
- Poll endpoint returns `{ "status": "InProgress" | "Succeeded" | "Failed" }`.

This is enough for Terraform’s polling behavior without simulating real provisioning.

## Storage emulation

### Control plane vs data plane
Terraform creates storage accounts via ARM, but many workflows then talk to the data plane (Blob).
We should treat these as separate frontends sharing the same backing state.

### Blob API (incremental)
Start with minimal endpoints needed by common SDKs:
- Container create/list
- Blob put/get
- Basic metadata and content-type

Authentication for data plane:
- Start permissive: accept any `Authorization` header.
- Later: implement Shared Key and SAS in a compatibility-focused way.

## Service implementation structure (Go)

Suggested package boundaries:
- `internal/httpserver`: server bootstrap, middleware, host routing.
- `internal/auth`: token endpoint + auth middleware.
- `internal/arm`: ARM router + generic resource CRUD.
- `internal/services/storage`: blob frontend.
- `internal/state`: Postgres store, migrations, transactions.
- `internal/ops`: LRO operations + worker.

Cross-cutting middleware:
- Correlation IDs (`x-ms-correlation-request-id`, `x-ms-request-id`).
- Structured logging with request/response summary.
- Strictness toggles via env vars.

## Observability and debugging
- Request log table (plus structured logs) is a first-class feature; the ability to answer “why did Terraform do X?” is critical.
- Add a “replay” mode later: capture HTTP interactions and re-run them for regression tests.

Sanitization guidelines (for request_log):
- Never store client secrets, authorization headers, or full access tokens.
- Store a token fingerprint (e.g. first 8 chars of SHA-256) if correlation is needed.
- Store bodies only for selected content-types and size-limit them.

## Open questions (to decide next)
These choices affect early implementation effort and long-term ergonomics.

1. **Domain allowlist**: which exact hostnames/zones do we override first (ARM only vs ARM+Blob)?
2. **JWT signing**: are we OK with a local self-contained signing key in the stack, or should tokens be unsigned (less correct) to reduce setup?
3. **Resource schema**: should `properties` be fully opaque JSON (fast) or do we want typed structs for key resources early (more correctness)?
4. **Async operations**: which resource types should be LRO from the start (storage accounts? VMs? everything optional)?

## Proposed first milestone (Terraform MVP)
- AAD-lite token endpoint: return a fake token.
- ARM:
  - subscriptions/resourceGroups CRUD
	- generic provider/type resource CRUD backed by `resources` table
  - deterministic LIST + paging

- Graph:
	- minimal stubs for required endpoints (return shaped `200`/`201` with stable IDs), expand only when real clients demand it

- LRO:
	- implement async behavior for storage accounts and VM create/delete flows

- Storage:
	- storage accounts via ARM (typed schema)
	- blob data plane is optional in the very first cut; add once ARM state is stable

