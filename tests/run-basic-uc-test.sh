#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"

COMPOSE_FILE="${COMPOSE_FILE:-${REPO_ROOT}/examples/docker-compose.yml}"

STAGE="init"

require_cmd() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "error: missing required command: $1" >&2
		exit 2
	fi
}

dump_diagnostics() {
	echo "--- diagnostics ---" >&2
	echo "compose file: ${COMPOSE_FILE}" >&2
	docker compose -f "$COMPOSE_FILE" ps -a || true
	echo "--- turquoise-api logs (tail) ---" >&2
	docker compose -f "$COMPOSE_FILE" logs --tail=200 turquoise-api || true
}

on_err() {
	# Terraform failures are currently expected until TLS/DNS interception is wired.
	if [[ "${STAGE}" == "terraform" ]]; then
		echo "---" >&2
		echo "terraform run failed." >&2
		echo "If you see AADSTS/tenant-not-found errors, Terraform is reaching real Azure instead of groundstack." >&2
		echo "The optional tls/dns interception stack isn't active yet (examples/edge/certs is currently empty)." >&2
		exit 1
	fi

	dump_diagnostics
	exit 1
}

trap on_err ERR

require_cmd docker
require_cmd curl

export GROUNDSTACK_UID
GROUNDSTACK_UID="$(id -u)"
export GROUNDSTACK_GID
GROUNDSTACK_GID="$(id -g)"

echo "==> Ensuring dev TLS certs exist (for edge-proxy)" >&2
"${REPO_ROOT}/tests/gen-dev-tls-certs.sh"

if [[ ! -f "$COMPOSE_FILE" ]]; then
	echo "error: compose file not found: ${COMPOSE_FILE}" >&2
	exit 2
fi

echo "==> Starting Postgres + turquoise-api + dns + edge-proxy (background)" >&2
STAGE="stack"
docker compose -f "$COMPOSE_FILE" --profile stack up -d --build postgres turquoise-api dns edge-proxy

echo "==> Waiting for API health endpoint" >&2
for _ in {1..60}; do
	if curl -fsS "http://localhost:18080/healthz" >/dev/null; then
		break
	fi
	sleep 1
done
curl -fsS "http://localhost:18080/healthz" | cat

echo "==> Bootstrapping example IDs into Postgres" >&2
STAGE="bootstrap"
if ! command -v python3 >/dev/null 2>&1; then
	echo "error: python3 is required for examples/basic bootstrap" >&2
	exit 2
fi

ENV_FILE="${REPO_ROOT}/examples/basic/.env.local"
if [[ ! -f "$ENV_FILE" ]]; then
	echo "error: missing env file: ${ENV_FILE}" >&2
	echo "hint: generate it with: python3 examples/basic/scripts/gen_ids.py" >&2
	exit 2
fi

GS_DB_URL_DEFAULT="postgres://groundstack:groundstack@localhost:15432/groundstack?sslmode=disable"
GS_DB_URL="${GS_DB_URL:-$GS_DB_URL_DEFAULT}" \
	python3 "${REPO_ROOT}/examples/basic/scripts/register_ids.py" \
		--env-file "$ENV_FILE" \
		--compose-file "$COMPOSE_FILE"

echo "==> Running terraform smoke test (streams output)" >&2
STAGE="terraform"
docker compose -f "$COMPOSE_FILE" --profile tf run --rm terraform-basic

echo "==> Done. Stack is still running (postgres + turquoise-api)." >&2
