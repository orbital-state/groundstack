# Missing API surface / callbacks (Terraform basic UC)

## Context
The `examples/basic` Terraform UC is intended to run fully locally against groundstack (TLS/DNS interception + minimal Azure control-plane emulation).

At this point:
- The stack starts (`postgres`, `turquoise-api`, `dns`, `edge-proxy`).
- Terraform can authenticate (AAD stub), query Graph (service principal lookup), and call ARM endpoints.
- Terraform reaches the storage account create flow but can hang for minutes.

This doc captures the problem as “missing API / missing implementation / waiting for callback”, before deciding the concrete fix.

## Symptom
Terraform apply repeatedly shows:
- `azurerm_storage_account.sa: Still creating... [N elapsed]`

When inspected via logs, the provider is polling an endpoint and receiving a non-success response, so it never reaches the “ready” state.

## Repro
1. Start the stack:
   - `cd examples && docker compose --profile stack up -d --build`
2. Run the basic UC:
   - `./tests/run-basic-uc-test.sh`
3. Observe Terraform hanging during storage account creation.

## What we observed (important)
During the hang, the provider is repeatedly calling the ARM control-plane endpoint:

- `GET https://management.azure.com/subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.Storage/storageAccounts/{account}/fileServices/default?api-version=2023-01-01`

And our emulator returns `404` for this path.

This looks like a “readiness” loop: until `fileServices/default` returns a successful shape, the AzureRM provider treats the Storage File service as not available.

## Interpreting the failure (could be multiple things)
This hang can mean several different categories of missing behavior:

### A) Missing control-plane API implementation
The provider is polling ARM resources that we don’t implement yet, for example:
- `Microsoft.Storage/storageAccounts/fileServices` (and/or default child resources)
- possibly other `*Services/default` resources (blob, queue, table, etc.) depending on provider flow

In this category, the fix is to implement those ARM endpoints (return `200` with a minimal-but-acceptable JSON resource) so polling converges.

### B) Missing asynchronous provisioning behavior (“waiting for callback”)
In real Azure, some resources are asynchronous and move through provisioning states.

Our current behavior often returns `Succeeded` immediately for the storage account itself, but the provider may still expect subordinate services (like file service) to appear later.

In this category, the fix is to introduce a small state machine / delayed availability:
- storage account created => after some delay, fileServices/default exists

### C) Missing data-plane routing/emulation
Some storage checks happen against data-plane hosts:
- `https://{account}.file.core.windows.net/...`
- `https://{account}.blob.core.windows.net/...`

We have started wiring wildcard DNS/TLS and a permissive handler for these hosts, but it’s still possible the provider uses a different check (different path/query/headers) than we currently answer.

In this category, the fix is to implement the exact probe endpoints the provider uses (often service-properties endpoints), or to return a permissive OK for the probe paths.

### D) TLS/DNS interception gaps
If DNS or TLS SANs do not cover the hostname the provider probes, calls may go to real Azure or fail with TLS errors.

In this category, the fix is to ensure the runner resolves all relevant hostnames to `edge-proxy` and that the edge certificate includes required SANs.

### E) Response-shape mismatch
Even if we return `200`, the provider may require certain fields to exist (or provisioning state transitions).

In this category, the fix is to adjust the JSON/XML to match what the SDK/provider parses (at least the fields it inspects).

## Candidate missing endpoints (control-plane)
Based on observed polling, these ARM endpoints are likely required for convergence:

- `GET /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.Storage/storageAccounts/{account}/fileServices/default?api-version=...`
- (potentially)
  - `PUT .../fileServices/default?api-version=...` (if provider tries to create/patch)
  - `GET .../blobServices/default?api-version=...`
  - `GET .../queueServices/default?api-version=...`
  - `GET .../tableServices/default?api-version=...`

We should confirm by capturing requests during a hang.

## Candidate missing endpoints (data-plane)
These are typical probes (not yet confirmed for our specific run):

- `GET https://{account}.file.core.windows.net/?restype=service&comp=properties`
- `GET https://{account}.blob.core.windows.net/?comp=properties`

## Minimal acceptance criteria (for this problem)
We consider this issue resolved when:
- `./tests/run-basic-uc-test.sh` completes without manual interruption.
- `azurerm_storage_account.sa` reaches “Creation complete”.
- No terraform backend state lock remains after completion.

## Suggested next diagnostic step (before implementing)
To avoid guessing, capture the exact polling behavior and expected shapes:
- Tail `edge-proxy` + `turquoise-api` logs while Terraform is in the “Still creating…” loop.
- Identify:
  - which host is being polled (ARM vs data-plane)
  - which path/query
  - the current status code we return

Then implement only the endpoints that are actually probed.
