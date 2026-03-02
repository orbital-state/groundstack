
# GRS-CORE-0001: Turquoise Terraform MVP (Core API + State)

**Status**: drafted
**Created**: 2026-03-02
**Authors**: @ewiger
**Discussion**: TBD

## Abstract
Define the initial implementation plan for “turquoise”: a local, Docker Compose–based Azure API substitute focused on running Terraform (`azurerm`) against a stateful emulator. The MVP provides an ARM-like control plane, AAD-lite token issuance, Postgres-backed state, request tracing, and limited long-running operation (LRO) support.

## Motivation
We want to develop and test infrastructure and platform code without relying on a real Azure subscription:
- Faster feedback loops and offline development.
- Deterministic test environments.
- Ability to introspect/debug provider behavior via captured requests/state.

The first milestone must be useful quickly: Terraform should be able to create/update/read/list/delete core resources and converge across multiple applies.

## Specification

### Guiding principles
- Terraform-first: implement only what real client traffic requires, but keep shapes Azure-like.
- State-first: consistent read-after-write and stable list results are more important than perfect schema coverage.
- Incremental fidelity: permissive mock → convergent state → Azure-shaped workflows.

### Scope (MVP)

#### 1) Docker Compose stack
Required services:
- `turquoise-api` (Go)
- `postgres`

Optional (but assumed by design):
- `dns` (CoreDNS or dnsmasq) for domain overrides
- `edge-proxy` for TLS termination (mkcert)

MVP should run without `dns`/`edge-proxy` for local dev via direct access to `turquoise-api`, but the default “realistic” path assumes HTTPS interception.

#### 2) HTTP routing by Host
Turquoise routes requests based on `Host` (or SNI via edge proxy):
- `management.azure.com` → ARM frontend
- `login.microsoftonline.com` → AAD-lite frontend
- `graph.microsoft.com` → Graph stub frontend (minimal responses)

#### 3) AAD-lite (token endpoint)
Implement minimal OAuth2 token issuance compatible with Terraform’s common flows:
- Accept `client_credentials` at minimum.
- Return `access_token`, `expires_in`, `token_type`.

Token format:
- JWT-like string signed with HS256 (shared secret via env var).
- Claims include `tid`, `aud=https://management.azure.com/`, `iss`, `exp/iat`.

Authorization behavior:
- Permissive: accept any bearer token for now.
- Optional strict mode later (not in MVP).

#### 4) ARM (core)
Provide ARM-style endpoints sufficient for generic CRUD and Terraform convergence:

Resource identifiers:
- Canonical Azure resource IDs are the primary keys.

Minimum endpoint families:
- Resource groups CRUD:
	- `PUT /subscriptions/{subId}/resourceGroups/{rgName}?api-version=...`
	- `GET /subscriptions/{subId}/resourceGroups/{rgName}?api-version=...`
	- `DELETE /subscriptions/{subId}/resourceGroups/{rgName}?api-version=...`
	- `GET /subscriptions/{subId}/resourceGroups?api-version=...`

- Generic resource CRUD (provider/type resources):
	- `PUT /subscriptions/{subId}/resourceGroups/{rg}/providers/{ns}/{type}/{name}?api-version=...`
	- `GET ...`
	- `DELETE ...`
	- `GET /subscriptions/{subId}/resourceGroups/{rg}/providers/{ns}/{type}?api-version=...` (list)

Response shape:
- Include `id`, `name`, `type`, `location` (when applicable), `tags`, and `properties`.
- Error responses use Azure-like `CloudError`/`error` envelope where feasible.

Paging:
- Deterministic ordering (by resource id) and stable `nextLink` behavior.

Provider registration:
- MVP: always “registered” (no-op) unless Terraform requires explicit endpoints.

#### 5) Graph (stub-first)
Graph is included as a stub frontend to avoid hard failures when the provider hits Graph.
- MVP: implement only the minimum endpoints observed in our Terraform workflows.
- Behavior: return stable IDs and minimal JSON, or a controlled “not implemented” error that is easy to diagnose.

#### 6) State persistence (Postgres)
MVP storage model:
- `resources` table for canonical resource IDs and opaque `properties`.
- Index fields for subscription/resource group/provider/type/name.
- `operations` for LRO tracking.
- `request_log` for request trace capture (sanitized).

Schema can be refined, but must support:
- Idempotent PUT.
- Read-after-write.
- Deterministic LIST.

#### 7) Long-running operations (LRO)
Implement async semantics for selected resource types:
- Storage accounts (ARM resource type)
- VM create/delete flows (control-plane objects)

Mechanism:
- On PUT/DELETE for those types, return `202` with `Azure-AsyncOperation` (and/or `Location`) header.
- Provide operation status endpoint returning `InProgress` → `Succeeded` after a small delay.

Everything else may be synchronous initially.

#### 8) Request tracing and sanitization
Persist request traces in Postgres:
- Store: timestamp, host, method, path, status, correlation id, truncated body (JSON when possible).
- Do not store secrets or auth headers.
- Store token fingerprints (optional) if needed for correlation.

### Out of scope (MVP)
- Real networking enforcement (VNets/NSGs) affecting connectivity.
- Blob data-plane implementation (may be milestone 2).
- Strict permission checks and tenant/subscription authorization.
- Full provider registration workflows.

### Milestones

**M0: Repo skeleton (optional if already present)**
- Minimal Go module layout and docker-compose wiring.

**M1: AAD-lite + ARM generic CRUD + Postgres**
- Token endpoint works for Terraform flows.
- Resource groups + generic resources CRUD/list work.
- DB migrations created and applied.
- Deterministic list + paging.
- Request log capture.

**M2: LRO support for storage accounts + VMs**
- Async operation endpoints and polling behavior.

**M3: Graph stubs for observed endpoints**
- Implement only what is required to unblock realistic runs.

### Acceptance criteria (definition of done)
- Terraform can `plan` and `apply` for a minimal config including at least:
	- one resource group
	- one ARM resource via generic handler (placeholder if needed)
- Repeated `terraform apply` converges (no perpetual diffs caused by turquoise).
- `terraform destroy` removes resources (or behaves deterministically in permissive mode).
- LRO resources complete successfully under provider polling.
- Request traces show enough context to diagnose failures without secrets leakage.

## Backwards Compatibility
Not applicable (initial implementation).

## Security Considerations
- Token signing secret must not be logged.
- Request logs must be sanitized (no Authorization headers, secrets, or full tokens).
- DNS overrides must be scoped/allowlisted to reduce risk of unintended interception.

## Deployment / Activation
- Primary runtime is Docker Compose on a developer machine.
- Default “realistic” mode assumes:
	- local DNS points to the stack’s DNS container
	- TLS interception via edge proxy with a locally-trusted CA

## Reference Implementation
Planned repo artifacts (names may change):
- Go server with host routing: ARM/AAD/Graph frontends.
- Postgres migrations for `resources`, `operations`, `request_log`.
- Minimal LRO worker.

## Test Plan
- Unit tests for:
	- resource id parsing and canonicalization
	- CRUD semantics and deterministic list ordering
	- LRO lifecycle state machine

- Integration smoke test (Terraform):
	- Use [examples/basic](../../../examples/basic) as the canonical MVP test configuration.
	- Run Terraform in an environment that naturally resolves hijacked Azure endpoints to groundstack:
		- Terraform runs *inside a Docker container* attached to the same Compose network as the stack.
		- The container’s DNS is configured to use the stack’s CoreDNS service.
		- The container trusts the mkcert root CA used by the edge proxy for `management.azure.com`, `login.microsoftonline.com`, and any other intercepted domains.

	- Expected execution sequence:
		1. `terraform -chdir=examples/basic init`
		2. `terraform -chdir=examples/basic apply -auto-approve`
		3. `terraform -chdir=examples/basic apply -auto-approve` (must converge; no perpetual diffs)
		4. `terraform -chdir=examples/basic destroy -auto-approve`

	- Environment variables:
		- Provide dummy-but-present `ARM_TENANT_ID`, `ARM_SUBSCRIPTION_ID`, `ARM_CLIENT_ID`, `ARM_CLIENT_SECRET`.
		- Provide a stable HS256 signing secret for AAD-lite tokens.

	- Practical note (to make DNS deterministic in CI):
		- Assign the DNS container a stable IP via Compose IPAM and point the terraform-runner container at it (or run Terraform within the same network namespace).

## Changelog
- 2026-03-02: drafted

