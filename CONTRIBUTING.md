# Contributing to idemproof

Issues, discussions and pull requests are all welcome.

## Getting started

You need Go ≥1.22 and a POSIX `/bin/sh` (for the integration tests and
examples); nothing else.

```bash
git clone https://github.com/JaydenCJ/idemproof && cd idemproof
go build ./...
go test ./...
bash scripts/smoke.sh
```

`scripts/smoke.sh` builds the binary and proves/disproves idempotency of
real shell commands in a temp dir, asserting on actual CLI output and
exit codes across every subfeature; it must finish by printing `SMOKE OK`.

## Before you open a pull request

1. `gofmt -l .` reports nothing (formatting is enforced).
2. `go vet ./...` passes with no findings.
3. `go test ./...` passes (90 deterministic tests, no network).
4. `bash scripts/smoke.sh` prints `SMOKE OK`.
5. Add tests for behavior changes; keep logic in pure, unit-testable
   modules (only `internal/execrun` starts processes, only
   `internal/snapshot` touches the filesystem).

## Ground rules

- Keep dependencies at zero — idemproof is standard library only, and a
  proof tool above all must be easy to audit. Adding a dependency needs
  strong justification in the PR.
- No network calls, ever, and no telemetry. The only processes idemproof
  starts are the ones the user asks it to prove.
- Determinism first: identical observations must produce byte-identical
  reports, including all orderings. New report fields need a JSON
  `schema_version` discussion in the PR.
- New normalizers are data: add the rule to `internal/scrub/scrub.go`
  with positive AND negative test lines (what must survive scrubbing),
  and a row in `docs/method.md`.
- Code comments and doc comments are written in English.

## Reporting bugs

Include the output of `idemproof version`, the full command line you ran,
the report output (text or JSON), and — for verdict disputes — a minimal
script that reproduces the behavior, since the classifier sees only
snapshots, streams, and exit codes.

## Security

Please do not open public issues for security problems; use GitHub's
private vulnerability reporting on this repository instead.
