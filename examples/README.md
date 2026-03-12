# examples

All examples share a single Docker Compose stack located at:
- [examples/docker-compose.yml](docker-compose.yml)

## Why a shared Compose

- One Postgres instance for all examples
- One network plan (static IPs) when we add DNS/TLS interception
- Consistent env var and bootstrap flow

## Quick start (today)

Start the DB:
- `docker compose -f examples/docker-compose.yml up -d postgres`

Start the Go API (optional):
- `docker compose -f examples/docker-compose.yml --profile stack up -d --build`
- Verify: `curl -fsS localhost:18080/healthz`

Then for a specific example (e.g. `basic`):
- `python3 examples/basic/scripts/gen_ids.py`
- `GS_DB_URL=postgres://groundstack:groundstack@localhost:15432/groundstack?sslmode=disable \
	python3 examples/basic/scripts/register_ids.py`

Notes:
- If the `tenants` / `subscriptions` tables do not exist yet, `register_ids.py` will create them.
- `register_ids.py` uses local `psql` if available; otherwise it runs `psql` inside the running Compose `postgres` container.

## Profiles

The compose file uses profiles so it can evolve without breaking the simple DB-only use case.

- Default (no profile): `postgres`
- `stack`: the Go API service (added once the Dockerfile exists)
- `tlsdns`: DNS + edge proxy for hijacked Azure HTTPS endpoints (planned)
- `tf`: Terraform runner container(s) per example (planned)

### Terraform runners

Terraform runners are defined per example so that each example can choose its own working directory, env file, and smoke-test command.

- `terraform-basic`: runs `examples/basic`

Run it with:
- `docker compose -f examples/docker-compose.yml --profile tf run --rm terraform-basic`
