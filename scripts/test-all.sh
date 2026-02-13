#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "==> Running control plane tests"
(cd "$ROOT_DIR/control-plane" && go test ./...)

echo "==> Running Go SDK tests"
(cd "$ROOT_DIR/sdk/go" && go test ./...)

echo "==> Running Python SDK tests"
(cd "$ROOT_DIR/sdk/python" && pytest)

if command -v npm >/dev/null 2>&1; then
  echo "==> Linting control plane web UI"
  (cd "$ROOT_DIR/control-plane/web/client" && npm run lint)
else
  echo "npm not found; skipping web UI lint."
fi

echo "All tests passed."
