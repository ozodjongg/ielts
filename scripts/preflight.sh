#!/usr/bin/env sh
set -eu

fail=0
need() {
  if command -v "$1" >/dev/null 2>&1; then
    echo "[ok] $1"
  else
    echo "[missing] $1"
    fail=1
  fi
}

need go
need node
need npm
need python3

if command -v psql >/dev/null 2>&1; then
  echo "[ok] psql (local database CLI available)"
else
  echo "[optional] psql not found; required only for native local database workflows"
fi

python3 tools/qa_v5.py || fail=1

if [ "$fail" -ne 0 ]; then
  echo "Preflight FAILED"
  exit 1
fi

echo "Preflight PASSED (see QA_REPORT.md for build-environment limitations)"
