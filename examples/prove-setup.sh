#!/usr/bin/env bash
# Prove a small "provision a config tree" script idempotent, then break
# it on purpose to show what a failing proof looks like.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

BIN="$WORKDIR/idemproof"
(cd "$ROOT" && go build -o "$BIN" ./cmd/idemproof)

# --- 1. A well-behaved setup script -----------------------------------
cat > "$WORKDIR/setup.sh" <<'EOF'
#!/bin/sh
mkdir -p app/config app/data
printf 'port=8080\nworkers=4\n' > app/config/app.conf
[ -f app/data/seed.db ] || : > app/data/seed.db
EOF
chmod +x "$WORKDIR/setup.sh"

TARGET="$WORKDIR/target-good"
mkdir -p "$TARGET"
echo "== proving setup.sh (expected: IDEMPOTENT) =="
"$BIN" --watch "$TARGET" --dir "$TARGET" -- "$WORKDIR/setup.sh"

# --- 2. The same script with a classic bug: an append ------------------
cat > "$WORKDIR/setup-buggy.sh" <<'EOF'
#!/bin/sh
mkdir -p app/config
printf 'port=8080\n' > app/config/app.conf
echo "provisioned $(hostname)" >> app/provision.log   # grows every run!
EOF
chmod +x "$WORKDIR/setup-buggy.sh"

TARGET="$WORKDIR/target-bad"
mkdir -p "$TARGET"
echo
echo "== proving setup-buggy.sh (expected: NOT IDEMPOTENT, exit 1) =="
if "$BIN" --watch "$TARGET" --dir "$TARGET" -- "$WORKDIR/setup-buggy.sh"; then
  echo "unexpected: buggy script passed" >&2
  exit 1
else
  echo "(idemproof exited 1, as it should)"
fi
