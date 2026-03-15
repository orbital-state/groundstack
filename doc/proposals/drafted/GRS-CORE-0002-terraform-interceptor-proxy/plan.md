# Implementation plan: Host-run Terraform via interceptor proxy

This plan complements the spec in [README.md](README.md).

## Outcomes / Definition of Done
- A new Compose service provides an HTTP(S) proxy suitable for Terraform.
- Host-run Terraform can use `HTTPS_PROXY` to reach Groundstack’s intercepted Azure endpoints.
- No system-wide DNS changes are required on the developer machine.
- Proxy is opt-in and safe-by-default (local-only exposure, minimal logging).

## Non-goals
- Replacing the containerized Terraform runner used for CI.
- Full transparent proxying (no iptables/TUN setup on the host).
- TLS MITM inside the proxy (TLS terminates at `edge-proxy`).

## Step 1 — Choose proxy implementation and config
- Default: Squid (`squid:alpine` or equivalent).
- Config requirements:
  - allow `CONNECT` to `443`
  - set `dns_nameservers 172.28.0.53`
  - bind to `0.0.0.0:3128` in-container
  - keep access logs small and avoid sensitive header logging
  - optionally allowlist destination domains

## Step 2 — Wire into Compose
- Update [examples/docker-compose.yml](../../../examples/docker-compose.yml):
  - add service `tf-proxy` under profile `proxy`
  - attach to network `groundstack`
  - publish `13128:3128`
  - set `dns: [172.28.0.53]`
  - depend on `dns` and `edge-proxy` being started

## Step 3 — Document developer workflow
- Update [examples/README.md](../../../examples/README.md):
  - how to start stack with `--profile proxy`
  - required env vars (`HTTPS_PROXY`, `HTTP_PROXY`, `NO_PROXY`)
  - CA trust requirement for `edge-proxy` certs

## Step 4 — Add a lightweight smoke test
- Add a script under `tests/` or `examples/basic/scripts/` that:
  - starts the stack
  - runs a `curl` through the proxy to a known HTTPS endpoint
  - optionally runs a short Terraform command if available

## Step 5 — Iterate with real Terraform traffic
- Run `terraform plan` and inspect proxy destinations.
- Extend DNS allowlist and/or edge proxy routing only as needed.
