# GRS-CORE-0002: Host-Run Terraform via Interceptor Proxy (Compose)

**Status**: drafted
**Created**: 2026-03-12
**Authors**: Groundstack Core Team
**Discussion**: TBD

## Abstract
Add a Docker Compose–managed HTTP(S) proxy service (e.g. Squid) that lives inside the same Groundstack “realistic” network environment (CoreDNS overrides + TLS edge proxy + turquoise API). This lets developers run `terraform plan/apply` directly from their host shell while Terraform’s outbound traffic is transparently routed into the Groundstack stack via standard proxy env vars, avoiding per-machine host/DNS rewiring and reducing iteration time versus running Terraform in a container.

## Motivation

Today, we validate Terraform workflows by running Terraform inside a container (e.g. `terraform-basic`). That is a good fit for automated CI/CD smoke tests:
- stable, hermetic network settings
- reproducible dependency/tooling versions
- minimal host configuration

However, it is a poor fit for manual development/debug loops:
- container start time + repeated `terraform init` can be slow
- stepping through local shell workflows (editing files, diffing state, running small commands) becomes awkward
- developers often want to run Terraform “natively” from `examples/basic` while still hitting the simulated Azure surface provided by Groundstack

We want:
- the same Azure endpoint interception behavior we get in-container (DNS overrides, TLS termination, host-based routing),
- without asking developers to run Terraform in Docker,
- and without requiring fragile machine-wide hacks (e.g. editing `/etc/hosts`, changing system DNS to point at the stack) just to test an example.

## Thesis
If Terraform is configured to use an HTTP(S) proxy, then for HTTPS requests Terraform will send `CONNECT management.azure.com:443` (etc.) to the proxy.

If the proxy runs inside the Compose network and uses Groundstack’s CoreDNS overrides, the proxy will resolve `management.azure.com`, `login.microsoftonline.com`, `graph.microsoft.com`, and Azure storage wildcard domains to the stack’s edge proxy IP. The edge proxy terminates TLS with Groundstack’s CA and forwards the request to `turquoise-api` using the original `Host` header.

This achieves “realistic interception” without requiring the developer machine to change its DNS: only Terraform’s proxy settings change.

## Specification

Implementation notes and common pitfalls are tracked in [impl-gotchas.md](impl-gotchas.md).

### 1) Add a Compose service: `tf-proxy`

Add a new service to [examples/docker-compose.yml](../../../examples/docker-compose.yml) under an opt-in profile (e.g. `proxy`) so it does not affect the default DB-only developer workflow.

Service requirements:
- Runs an HTTP proxy that supports HTTPS via `CONNECT` (Squid is a straightforward option).
- Attached to the existing `groundstack` network so it can reach:
  - `dns` at a stable IP (today: `172.28.0.53`)
  - `edge-proxy` at a stable IP (today: `172.28.0.2`)
- Uses the stack DNS service for name resolution (critical).
- Exposes a host port (e.g. `13128:3128`) so host-run Terraform can reach it.

Notes:
- This proposal does *not* require TLS MITM inside the proxy. The proxy can act as a tunnel and let TLS terminate at `edge-proxy`.
- The proxy must allow `CONNECT` to port 443 at minimum.

### 2) DNS and TLS: reuse existing Groundstack interception primitives

This proposal intentionally reuses the existing “realistic path” building blocks:

- CoreDNS config in [examples/dns/Corefile](../../../examples/dns/Corefile):
  - overrides `management.azure.com`, `login.microsoftonline.com`, `graph.microsoft.com` to `172.28.0.2`
  - overrides storage wildcard domains (e.g. `*.blob.core.windows.net`) to `172.28.0.2`

- Nginx edge proxy in [examples/edge/nginx.conf](../../../examples/edge/nginx.conf):
  - terminates TLS using the local CA/certs mounted from `examples/edge/certs`
  - forwards to `turquoise-api:8080`, preserving `Host`

The new proxy service must “consume” these by:
- using CoreDNS (`dns: 172.28.0.53`) so that Azure domains resolve inside the stack
- opening upstream TCP connections to those resolved names (which will land on `edge-proxy`)

### 3) Developer UX (MVP)

Host-run Terraform workflow:
1) Start the stack services (DB + API + realistic interceptors + proxy):
   - `docker compose -f examples/docker-compose.yml --profile stack --profile proxy up -d`

2) Ensure the host trusts the Groundstack root CA used by `edge-proxy`.
   - (One-time per developer machine.)

3) In a host shell, set proxy env vars for Terraform:
   - `export HTTPS_PROXY=http://127.0.0.1:13128`
   - `export HTTP_PROXY=http://127.0.0.1:13128`
   - `export NO_PROXY=127.0.0.1,localhost`

4) Run Terraform directly:
   - `cd examples/basic`
   - `terraform init`
   - `terraform plan`
   - `terraform apply`

Non-goals for MVP UX:
- automatic proxy injection into every shell (keep it explicit and opt-in)
- modifying system-wide DNS or certificate stores automatically

### 4) Behavior / correctness requirements

- **No host DNS changes required**: the developer machine does not need to resolve Azure domains to the stack.
- **Deterministic routing**: all Azure-shaped endpoints used by Terraform must resolve (from the proxy) to `edge-proxy` using the allowlisted CoreDNS overrides.
- **TLS validation succeeds**: once the host trusts Groundstack’s CA, Terraform must not need `-insecure` flags.
- **No new Azure endpoint configuration**: avoid relying on Terraform-provider-specific “custom endpoints” knobs for basic usage.

### 5) Observability / debug

The proxy should provide enough logging to answer:
- “Did Terraform use the proxy for this request?”
- “Which `CONNECT host:port` targets were requested?”
- “Did upstream connect succeed?”

Keep logs local and do not log secrets.

### Backwards Compatibility
- No changes required for CI: containerized `terraform-basic` remains supported and is still the recommended CI smoke-test path.
- The proxy path is opt-in and only impacts developers who set proxy env vars.

### Security Considerations
- Proxies can unintentionally capture credentials and bearer tokens.
  - The proxy must bind to localhost-only on the host-published port (or be otherwise restricted).
  - Avoid logging Authorization headers or full URLs that contain secrets.
- Prefer an allowlist-style configuration:
  - allow `CONNECT` only to `:443`
  - optionally restrict destinations to Azure domains relevant to Groundstack (management, login, graph, storage) to reduce accidental exfiltration.

### Deployment / Activation
- Activated via Docker Compose profile (e.g. `proxy`).
- Requires `edge-proxy` certificates to be present and a developer-installed CA for host TLS trust.

### Reference Implementation (planned)
- Compose:
  - Add service `tf-proxy` using `squid` (or alternative) in [examples/docker-compose.yml](../../../examples/docker-compose.yml).
  - Configure:
    - `dns: [172.28.0.53]`
    - `ports: ["13128:3128"]`
    - allow `CONNECT` to `443`
    - small access logs
- Docs:
  - Add a short “Host-run Terraform via proxy” section to [examples/README.md](../../../examples/README.md).
  - Document CA trust steps for major OSes (or reference existing CA generation docs once present).

### Test Plan

#### 1) Smoke test: host → proxy → edge → API
- With stack running and the host trusting the Groundstack root CA, run:
  - `curl -fsS --proxy http://127.0.0.1:13128 https://management.azure.com/healthz`
  - Expect: `200` and a JSON body like `{"status":"ok",...}`.

#### 2) Terraform basic example (manual loop)
- In `examples/basic` on the host:
  - `terraform init`
  - `terraform apply -auto-approve`
  - `terraform apply -auto-approve` (must converge)
  - `terraform destroy -auto-approve`

Validation:
- proxy logs show `CONNECT management.azure.com:443` and any other expected destinations
- Groundstack request logs (if enabled) show the corresponding ARM/AAD/Graph activity

#### 3) Negative test: without proxy
- With no proxy env vars set, Terraform should behave as it does today (likely failing to reach Azure, which is acceptable).

## Changelog
- 2026-03-12: drafted
