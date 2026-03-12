#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"

CERT_DIR="${CERT_DIR:-${REPO_ROOT}/examples/edge/certs}"

require_cmd() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "error: missing required command: $1" >&2
		exit 2
	fi
}

require_cmd openssl

mkdir -p "$CERT_DIR"

CA_KEY="$CERT_DIR/ca.key"
CA_CRT="$CERT_DIR/ca.crt"
TLS_KEY="$CERT_DIR/tls.key"
TLS_CRT="$CERT_DIR/tls.crt"

if [[ -s "$CA_CRT" && -s "$TLS_CRT" && -s "$TLS_KEY" ]]; then
	echo "dev TLS certs already exist in: $CERT_DIR" >&2
	exit 0
fi

echo "generating dev CA + server certs in: $CERT_DIR" >&2

# CA
if [[ ! -s "$CA_KEY" || ! -s "$CA_CRT" ]]; then
	openssl genrsa -out "$CA_KEY" 2048 >/dev/null 2>&1
	openssl req -x509 -new -nodes -key "$CA_KEY" -sha256 -days 3650 \
		-subj "/CN=groundstack-dev-ca" \
		-out "$CA_CRT" >/dev/null 2>&1
fi

# Server key + csr
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

SERVER_KEY="$TMP_DIR/server.key"
SERVER_CSR="$TMP_DIR/server.csr"
SERVER_EXT="$TMP_DIR/server.ext"

openssl genrsa -out "$SERVER_KEY" 2048 >/dev/null 2>&1

# CN doesn't matter much as long as SANs are correct.
openssl req -new -key "$SERVER_KEY" -subj "/CN=management.azure.com" -out "$SERVER_CSR" >/dev/null 2>&1

cat >"$SERVER_EXT" <<'EOF'
subjectAltName = @alt_names
extendedKeyUsage = serverAuth
keyUsage = digitalSignature, keyEncipherment

[alt_names]
DNS.1 = management.azure.com
DNS.2 = login.microsoftonline.com
DNS.3 = graph.microsoft.com
EOF

openssl x509 -req -in "$SERVER_CSR" -CA "$CA_CRT" -CAkey "$CA_KEY" -CAcreateserial \
	-out "$TLS_CRT" -days 825 -sha256 -extfile "$SERVER_EXT" >/dev/null 2>&1

cp "$SERVER_KEY" "$TLS_KEY"

chmod 600 "$CA_KEY" "$TLS_KEY" || true

echo "wrote: $CA_CRT" >&2
echo "wrote: $TLS_CRT" >&2
echo "wrote: $TLS_KEY" >&2
