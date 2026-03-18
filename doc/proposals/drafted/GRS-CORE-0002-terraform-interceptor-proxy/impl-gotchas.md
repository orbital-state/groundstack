# Implementation gotchas: Terraform interceptor proxy

This is a scratchpad of practical issues that can make the proxy approach fail or behave inconsistently.

## TLS + certificates
- The edge proxy certificate must cover *every* intercepted hostname via SANs/wildcards.
  - Control plane: `management.azure.com`, `login.microsoftonline.com`, `graph.microsoft.com`.
  - Storage-style wildcards (as DNS already overrides them): `*.blob.core.windows.net`, `*.dfs.core.windows.net`, `*.file.core.windows.net`, `*.queue.core.windows.net`, `*.table.core.windows.net`, `*.web.core.windows.net`.
- Host trust is mandatory.
  - Terraform + provider plugins validate TLS using the host trust store.
  - If the Groundstack root CA isn’t trusted, you’ll see x509 failures even if routing is correct.

## Terraform init and non-Azure traffic
- `terraform init` typically talks to non-Azure domains (provider/module downloads).
  - Common: `registry.terraform.io`, `releases.hashicorp.com`, `checkpoint.hashicorp.com`, and sometimes `github.com`.
- If `HTTPS_PROXY` is set and the proxy is configured “Azure-only allowlist”, `terraform init` can fail.

### Workable patterns
- **Recommended**: keep proxy enabled, but allow general egress through the proxy.
  - Use CoreDNS overrides to steer only Azure-shaped domains into the stack; everything else goes out normally.
- **Simple**: don’t proxy `terraform init`.
  - Run `terraform init` with no proxy env vars; set proxy only for `plan/apply/destroy`.
- **Possible but brittle**: use `NO_PROXY` for Terraform registry endpoints.
  - Expect the list to grow over time due to redirects and new sources.

### Speed/ergonomics
- Set `TF_PLUGIN_CACHE_DIR` on the host to avoid repeated provider downloads during manual loops.

## DNS + proxy resolution
- The proxy must resolve names using Groundstack CoreDNS, not default resolver behavior.
  - Docker `dns: [172.28.0.53]` helps, but some proxies also need explicit config (e.g. Squid `dns_nameservers`).
- If the proxy resolves `management.azure.com` to the real internet IPs, traffic bypasses Groundstack.

## Endpoint allowlist drift
- Terraform/provider auth mode can expand the hostname set.
  - Legacy/alternate AAD endpoints (example): `login.windows.net`, `sts.windows.net`.
- Keep the DNS allowlist (CoreDNS) and certificate SAN set in sync with observed traffic.

## Proxy safety
- Treat the proxy as sensitive:
  - bind/publish to localhost-only on the host side
  - avoid logging Authorization headers, full tokens, or request bodies
  - prefer minimal access logs (`CONNECT host:port`, status)

## Debugging checklist
- Confirm Terraform is actually using the proxy (proxy access log shows `CONNECT ...:443`).
- Confirm proxy DNS resolution returns `172.28.0.2` for Azure domains.
- Confirm host trusts the CA and the edge proxy cert matches the SNI hostname.
