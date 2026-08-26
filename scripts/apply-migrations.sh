#!/usr/bin/env sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"

echo "Applying all module schemas to the single PostgreSQL database..."
docker compose --profile tools run --rm compact-migrate
echo "All V5 monolith migrations applied successfully."
