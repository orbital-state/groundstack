# examples/basic

This Terraform example is the integration smoke test for turquoise.

## What it creates
- One AzureRM resource group
- One AzureRM storage account

## How it is intended to be run
This example is meant to run against *hijacked* Azure endpoints (e.g. `management.azure.com`, `login.microsoftonline.com`) that resolve to the local groundstack services via CoreDNS, with TLS terminated by an edge proxy using a locally trusted mkcert root CA.

In practice, that usually means running Terraform **inside a container on the same Docker network** as the groundstack stack, configured with:
- DNS pointing at the CoreDNS container
- the mkcert root CA mounted and trusted by the Terraform container

### Sketch of a docker-based run
The exact wiring will depend on the Compose network/IP plan, but conceptually:

1) Ensure the shared examples stack is running (DB now; API/DNS/TLS later)

- Start Postgres:
	- `docker compose -f examples/docker-compose.yml up -d postgres`

2) Run Terraform in a container with:
- `--network` set to the stack network
- `--dns` set to the CoreDNS IP
- the mkcert root CA mounted into the container and added to trust

Example (placeholders):
- `COREDNS_IP=172.28.0.53`
- `MKCERT_CA=~/.local/share/mkcert/rootCA.pem`

Then:
- `docker run --rm \
	--network groundstack_default \
	--dns ${COREDNS_IP} \
	-v "$PWD/..:/work" \
	-v "${MKCERT_CA}:/usr/local/share/ca-certificates/mkcert-rootCA.crt:ro" \
	-w /work/basic \
	-e ARM_TENANT_ID=00000000-0000-0000-0000-000000000000 \
	-e ARM_SUBSCRIPTION_ID=00000000-0000-0000-0000-000000000000 \
	-e ARM_CLIENT_ID=00000000-0000-0000-0000-000000000000 \
	-e ARM_CLIENT_SECRET=dummy \
	hashicorp/terraform:1.8 \
	sh -lc "update-ca-certificates || true; terraform init; terraform apply -auto-approve; terraform apply -auto-approve; terraform destroy -auto-approve"`

Alternative (shared Compose runner):
- `docker compose -f examples/docker-compose.yml --profile tf run --rm terraform-basic`

## Environment variables
The AzureRM provider typically reads credentials from `ARM_*` env vars.
For turquoise MVP these can be dummy values as long as the token endpoint accepts them.

Minimum set:
- `ARM_TENANT_ID`
- `ARM_SUBSCRIPTION_ID`
- `ARM_CLIENT_ID`
- `ARM_CLIENT_SECRET`

## Bootstrap scripts (tenant/subscription)

This example includes scripts to generate and register the IDs it uses.

One-command bootstrap (starts Postgres + generates IDs + registers them):
- `sh examples/basic/bootstrap.sh`

1) Generate IDs (writes `examples/basic/.env.local`):
- `python3 examples/basic/scripts/gen_ids.py`

2) Register IDs in the emulator DB (idempotent):
- `GS_DB_URL=postgres://groundstack:groundstack@localhost:15432/groundstack?sslmode=disable \
	python3 examples/basic/scripts/register_ids.py`

Notes:
- Registration is currently DB-first (no API dependency). If the `tenants` / `subscriptions` tables do not exist yet, `register_ids.py` will create them.
- The generated `.env.local` can be sourced in a shell or used to pass env vars into `docker run`.
- `register_ids.py` will use local `psql` if present; otherwise it will run `psql` inside the running Compose `postgres` container.

## Notes
- `skip_provider_registration = true` is set to reduce calls to provider registration endpoints in early MVP.
