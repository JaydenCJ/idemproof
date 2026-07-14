#!/usr/bin/env bash
# End-to-end smoke test for idemproof: builds the binary, then proves and
# disproves idempotency of real shell commands in a temp dir, asserting on
# actual CLI output and exit codes. No network, idempotent, seconds to run.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

fail() {
  echo "SMOKE FAIL: $*" >&2
  exit 1
}

BIN="$WORKDIR/idemproof"

echo "1. build"
(cd "$ROOT" && go build -o "$BIN" ./cmd/idemproof) || fail "go build failed"

echo "2. version matches manifest"
"$BIN" --version | grep -qx "idemproof 0.1.0" || fail "--version mismatch"

echo "3. an idempotent setup script passes (exit 0)"
D1="$WORKDIR/pass"; mkdir -p "$D1"
OUT="$("$BIN" --watch "$D1" --dir "$D1" --shell -- \
  'mkdir -p app/config && printf "port=8080\n" > app/config/app.conf')" \
  || fail "idempotent command should exit 0"
echo "$OUT" | grep -q "verdict: IDEMPOTENT — converged after run 1" || fail "missing pass verdict"
echo "$OUT" | grep -q "+ created   app/config/app.conf" || fail "first-run effects missing"

echo "4. an appending command fails (exit 1) with the exact violation"
D2="$WORKDIR/fail"; mkdir -p "$D2"
set +e
OUT="$("$BIN" --watch "$D2" --dir "$D2" --shell -- 'echo run >> log.txt')"
CODE=$?
set -e
[ "$CODE" -eq 1 ] || fail "appending command should exit 1, got $CODE"
echo "$OUT" | grep -q "run 2 violations" || fail "violation section missing"
echo "$OUT" | grep -q "~ modified  log.txt   content, size (4 B -> 8 B)" || fail "modified detail missing"

echo "5. JSON report is machine-readable"
D3="$WORKDIR/json"; mkdir -p "$D3"
set +e
JSON="$("$BIN" --watch "$D3" --dir "$D3" --format json --shell -- 'echo x >> grow.txt')"
set -e
echo "$JSON" | grep -q '"tool": "idemproof"' || fail "json envelope missing"
echo "$JSON" | grep -q '"schema_version": 1' || fail "json schema_version missing"
echo "$JSON" | grep -q '"verdict": "not-idempotent"' || fail "json verdict wrong"
echo "$JSON" | grep -q '"path": "grow.txt"' || fail "json change path missing"

echo "6. --ignore excludes log noise from the proof"
D4="$WORKDIR/ignore"; mkdir -p "$D4"
"$BIN" --watch "$D4" --dir "$D4" --ignore '*.log' --shell -- \
  'touch state.flag && echo ran >> debug.log' >/dev/null \
  || fail "--ignore should rescue the proof"

echo "7. --runs 3 proves convergence of a first-run-only script"
D5="$WORKDIR/settle"; mkdir -p "$D5"
OUT="$("$BIN" --watch "$D5" --dir "$D5" --runs 3 --shell -- \
  '[ -f done ] || { echo setup; touch done; }')" \
  || fail "3-run proof should pass"
echo "$OUT" | grep -q "output (run 2 vs run 3)" || fail "steady-state comparison missing"

echo "8. --normalize absorbs volatile output tokens"
D6="$WORKDIR/norm"; mkdir -p "$D6"
CMD='n=$(cat '"$WORKDIR"'/ctr 2>/dev/null || echo 0); n=$((n+1)); echo $n > '"$WORKDIR"'/ctr; echo "finished in ${n}ms"'
set +e
"$BIN" --watch "$D6" --dir "$D6" --shell -- "$CMD" >/dev/null 2>&1
[ $? -eq 1 ] || fail "raw volatile output should fail"
set -e
"$BIN" --watch "$D6" --dir "$D6" --normalize durations --shell -- "$CMD" >/dev/null \
  || fail "--normalize durations should absorb the drift"

echo "9. usage errors exit 2"
set +e
"$BIN" --format yaml -- true >/dev/null 2>&1
[ $? -eq 2 ] || fail "bad --format should exit 2"
"$BIN" >/dev/null 2>&1
[ $? -eq 2 ] || fail "missing command should exit 2"
set -e

echo "10. --quiet prints exactly one verdict line"
D7="$WORKDIR/quiet"; mkdir -p "$D7"
LINES="$("$BIN" --watch "$D7" --dir "$D7" --quiet --shell -- 'mkdir -p sub' | wc -l)"
[ "$LINES" -eq 1 ] || fail "--quiet should print 1 line, got $LINES"

echo "SMOKE OK"
