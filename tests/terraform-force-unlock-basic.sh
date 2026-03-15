#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
	echo "usage: $0 <LOCK_ID>" >&2
	echo "example: $0 bd539342-01c1-ead9-f9ad-0f43359ace82" >&2
	exit 2
fi

LOCK_ID="$1"

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
COMPOSE_FILE="${COMPOSE_FILE:-${REPO_ROOT}/examples/docker-compose.yml}"

export GROUNDSTACK_UID
GROUNDSTACK_UID="$(id -u)"
export GROUNDSTACK_GID
GROUNDSTACK_GID="$(id -g)"

# Safety: stop any stuck terraform run containers for this project.
docker ps -aq --filter "name=groundstack-examples-terraform-basic-run-" | xargs -r docker rm -f >/dev/null 2>&1 || true

# Local backend lock files can linger if the process was killed.
rm -f "${REPO_ROOT}/examples/basic/.terraform.tfstate.lock.info" || true

# Run terraform force-unlock using the same image, but override the service entrypoint.
docker compose -f "$COMPOSE_FILE" --profile tf run --rm --entrypoint terraform terraform-basic force-unlock -force "$LOCK_ID"
