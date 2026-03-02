#!/usr/bin/env python3
import argparse
import os
import shutil
import subprocess
import uuid
from pathlib import Path
from urllib.parse import urlparse


def parse_dotenv(path: Path) -> dict:
    env: dict[str, str] = {}
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue
        if "=" not in line:
            continue
        key, value = line.split("=", 1)
        env[key.strip()] = value.strip()
    return env


def require_uuid(value: str, name: str) -> str:
    try:
        parsed = uuid.UUID(value)
    except Exception as exc:  # noqa: BLE001
        raise SystemExit(f"error: {name} must be a UUID, got: {value!r} ({exc})")
    return str(parsed)


def parse_db_url(db_url: str) -> tuple[str, str, str]:
    """Return (user, password, dbname) from a postgres URL.

    Only the parts needed for psql are extracted.
    """
    parsed = urlparse(db_url)
    if parsed.scheme not in {"postgres", "postgresql"}:
        return ("groundstack", "groundstack", "groundstack")

    user = parsed.username or "groundstack"
    password = parsed.password or "groundstack"
    dbname = (parsed.path or "/groundstack").lstrip("/") or "groundstack"
    return (user, password, dbname)


def run_psql(args: list[str], sql: str, extra_env: dict[str, str] | None = None) -> int:
    env = os.environ.copy()
    if extra_env:
        env.update(extra_env)

    proc = subprocess.run(
        args,
        input=sql,
        text=True,
        check=False,
        env=env,
    )
    return proc.returncode


def main() -> int:
    parser = argparse.ArgumentParser(
        description=(
            "Register the tenant/subscription IDs used by examples/basic into the groundstack DB. "
            "This is DB-first and idempotent."
        )
    )
    parser.add_argument(
        "--env-file",
        default=str(Path(__file__).resolve().parent.parent / ".env.local"),
        help="Dotenv file containing ARM_TENANT_ID/ARM_SUBSCRIPTION_ID",
    )
    parser.add_argument(
        "--compose-file",
        default=str(Path(__file__).resolve().parents[2] / "docker-compose.yml"),
        help="Path to the shared examples docker-compose.yml (used when local psql is missing)",
    )
    parser.add_argument(
        "--compose-service",
        default="postgres",
        help="Docker Compose service name for Postgres (used when local psql is missing)",
    )
    args = parser.parse_args()

    db_url = os.environ.get("GS_DB_URL")
    psql_path = shutil.which("psql")

    env_file = Path(args.env_file)
    if not env_file.exists():
        print(f"error: env file not found: {env_file}", file=os.sys.stderr)
        print("hint: run examples/basic/scripts/gen_ids.py first", file=os.sys.stderr)
        return 2

    env = parse_dotenv(env_file)
    tenant_id_raw = env.get("ARM_TENANT_ID")
    subscription_id_raw = env.get("ARM_SUBSCRIPTION_ID")
    if not tenant_id_raw or not subscription_id_raw:
        print(
            f"error: ARM_TENANT_ID and ARM_SUBSCRIPTION_ID must be set in {env_file}",
            file=os.sys.stderr,
        )
        return 2

    tenant_id = require_uuid(tenant_id_raw, "ARM_TENANT_ID")
    subscription_id = require_uuid(subscription_id_raw, "ARM_SUBSCRIPTION_ID")

    ensure_schema_sql = """
CREATE TABLE IF NOT EXISTS tenants (
    id text PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now(),
    meta jsonb NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS subscriptions (
    id text PRIMARY KEY,
    tenant_id text NOT NULL REFERENCES tenants(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    meta jsonb NOT NULL DEFAULT '{}'::jsonb
);
""".lstrip()

    sql = """
BEGIN;

INSERT INTO tenants (id, meta)
VALUES (:'tenant_id', '{}'::jsonb)
ON CONFLICT (id) DO NOTHING;

INSERT INTO subscriptions (id, tenant_id, meta)
VALUES (:'subscription_id', :'tenant_id', '{}'::jsonb)
ON CONFLICT (id) DO UPDATE SET tenant_id = EXCLUDED.tenant_id;

COMMIT;
""".lstrip()

    print("ensuring schema (tenants/subscriptions)...", file=os.sys.stderr)

    if psql_path is not None:
        if not db_url:
            print(
                "error: GS_DB_URL is required when using local psql (e.g. postgres://user:pass@localhost:15432/groundstack?sslmode=disable)",
                file=os.sys.stderr,
            )
            return 2

        user, password, _dbname = parse_db_url(db_url)
        code = run_psql(
            [
                psql_path,
                db_url,
                "-v",
                "ON_ERROR_STOP=1",
                "-q",
                "-f",
                "-",
            ],
            ensure_schema_sql,
            extra_env={
                # Helps when pg_hba requires a password.
                "PGUSER": user,
                "PGPASSWORD": password,
            },
        )
        if code != 0:
            return code

        print("registering tenant/subscription into DB...", file=os.sys.stderr)

        code = run_psql(
            [
                psql_path,
                db_url,
                "-v",
                "ON_ERROR_STOP=1",
                "-q",
                "-v",
                f"tenant_id={tenant_id}",
                "-v",
                f"subscription_id={subscription_id}",
                "-f",
                "-",
            ],
            sql,
            extra_env={
                # Helps when pg_hba requires a password.
                "PGUSER": user,
                "PGPASSWORD": password,
            },
        )
        if code != 0:
            return code
    else:
        if shutil.which("docker") is None:
            print(
                "error: neither psql nor docker is available; install a Postgres client (psql) or use Docker Compose",
                file=os.sys.stderr,
            )
            return 2

        # Fall back to running psql inside the running postgres container.
        # This avoids requiring psql on the host.
        if db_url:
            user, password, dbname = parse_db_url(db_url)
        else:
            user, password, dbname = ("groundstack", "groundstack", "groundstack")

        code = run_psql(
            [
                "docker",
                "compose",
                "-f",
                args.compose_file,
                "exec",
                "-T",
                "-e",
                f"PGPASSWORD={password}",
                args.compose_service,
                "psql",
                "-U",
                user,
                "-d",
                dbname,
                "-v",
                "ON_ERROR_STOP=1",
                "-q",
                "-f",
                "-",
            ],
            ensure_schema_sql,
        )
        if code != 0:
            print(
                "hint: ensure the postgres service is running: docker compose -f examples/docker-compose.yml up -d postgres",
                file=os.sys.stderr,
            )
            return code

        print("registering tenant/subscription into DB...", file=os.sys.stderr)

        code = run_psql(
            [
                "docker",
                "compose",
                "-f",
                args.compose_file,
                "exec",
                "-T",
                "-e",
                f"PGPASSWORD={password}",
                args.compose_service,
                "psql",
                "-U",
                user,
                "-d",
                dbname,
                "-v",
                "ON_ERROR_STOP=1",
                "-q",
                "-v",
                f"tenant_id={tenant_id}",
                "-v",
                f"subscription_id={subscription_id}",
                "-f",
                "-",
            ],
            sql,
        )
        if code != 0:
            return code

    print(
        f"registered ARM_TENANT_ID={tenant_id} ARM_SUBSCRIPTION_ID={subscription_id}",
        file=os.sys.stderr,
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
