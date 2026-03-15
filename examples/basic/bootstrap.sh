#!/usr/bin/env sh
set -eu

# Bootstrap helper for examples/basic.
# - starts Postgres via the shared examples/docker-compose.yml
# - generates tenant/subscription/client IDs
# - registers tenant/subscription IDs into Postgres

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "${SCRIPT_DIR}/../.." && pwd)

PYTHON=${PYTHON:-python3}
COMPOSE_FILE=${COMPOSE_FILE:-"${REPO_ROOT}/examples/docker-compose.yml"}

# Default DB URL matches examples/docker-compose.yml (postgres port 15432 on localhost)
GS_DB_URL_DEFAULT="postgres://groundstack:groundstack@localhost:15432/groundstack?sslmode=disable"
GS_DB_URL=${GS_DB_URL:-"$GS_DB_URL_DEFAULT"}

if [ "${NO_DOCKER_UP:-}" != "1" ]; then
	if ! command -v docker >/dev/null 2>&1; then
		echo "error: docker is required (or set NO_DOCKER_UP=1 to skip bringing services up)" >&2
		exit 2
	fi

	echo "starting postgres via docker compose..." >&2
	docker compose -f "$COMPOSE_FILE" up -d postgres
fi

echo "generating IDs (.env.local)..." >&2
"$PYTHON" "${REPO_ROOT}/examples/basic/scripts/gen_ids.py"

echo "registering tenant/subscription into DB..." >&2
GS_DB_URL="$GS_DB_URL" \
	"$PYTHON" "${REPO_ROOT}/examples/basic/scripts/register_ids.py"

echo "bootstrap complete" >&2