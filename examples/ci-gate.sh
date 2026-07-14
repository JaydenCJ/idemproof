#!/usr/bin/env bash
# Use idemproof as a repeatability gate: a migration must converge and
# stay silent on re-runs before it is allowed to ship. Exit codes do all
# the work — 0 passes the gate, anything else blocks it.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

BIN="$WORKDIR/idemproof"
(cd "$ROOT" && go build -o "$BIN" ./cmd/idemproof)

# A guarded migration: does its work once, then no-ops with a marker file.
cat > "$WORKDIR/migrate.sh" <<'EOF'
#!/bin/sh
if [ -f .migrated-001 ]; then
  exit 0
fi
mkdir -p schema
printf 'version=1\n' > schema/version.ini
echo "applied migration 001"
: > .migrated-001
EOF
chmod +x "$WORKDIR/migrate.sh"

DB="$WORKDIR/db"
mkdir -p "$DB"

# --runs 3 gives the migration one settling run: run 1 may do work, runs
# 2 and 3 must be byte-identical no-ops. JSON goes to the build artifact.
echo "== repeatability gate: migrate.sh =="
if "$BIN" --watch "$DB" --dir "$DB" --runs 3 --require-zero \
    --format json -- "$WORKDIR/migrate.sh" > "$WORKDIR/proof.json"; then
  echo "GATE PASS — proof stored at proof.json"
  grep '"verdict"' "$WORKDIR/proof.json"
else
  echo "GATE FAIL — migration is not safely re-runnable" >&2
  cat "$WORKDIR/proof.json" >&2
  exit 1
fi
