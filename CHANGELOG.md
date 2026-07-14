# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-07-13

### Added

- Core proof loop: snapshot → run → snapshot → run → snapshot, with the
  first run's effects reported as legitimate work and any later run's
  filesystem change flagged as a violation; `--runs 2..10` for
  convergence checks between steady-state runs.
- Filesystem snapshot engine: kind, SHA-256 content hash, size,
  permission bits, and symlink targets per path; optional mtime
  comparison via `--strict-times`; `--max-file-size` cap that falls back
  to size-only comparison; multiple `--watch` roots with prefixed paths.
- Ignore globs (`--ignore`, repeatable) with `*`, `?`, and cross-segment
  `**`, gitignore-style bare-name matching at any depth, and whole-subtree
  pruning of ignored directories.
- Output stability check: byte-honest stdout/stderr comparison of the
  final two runs with a located first divergence (line number plus both
  lines quoted), trailing-newline drift detection, and `--no-output` to
  opt out.
- Output normalizers: `timestamps`, `times`, `uuids`, `hex`, `pids`,
  `durations`, `tmppaths`, the `all` alias, and custom `--scrub REGEXP`
  patterns; active normalizers are always disclosed in the report.
- Exit-code stability check with `--allow-exit-change` waiver and
  `--require-zero` strictness; process harness with /dev/null stdin,
  `--dir`, and repeatable `--env KEY=VAL`.
- Reports: human text (first-run effects, per-run violations, numbered
  violation summary, verdict line) with `--quiet` single-line mode, and
  stable JSON (`schema_version: 1`) via `--format json`.
- CLI exit codes as an API: 0 idempotent, 1 not idempotent, 2 usage
  error, 3 runtime error — plus `--shell` for one-liner proofs.
- Runnable examples (`examples/prove-setup.sh`, `examples/ci-gate.sh`)
  and a methodology reference (`docs/method.md`).
- 90 deterministic offline tests (unit + in-process CLI integration
  against real shell commands) and `scripts/smoke.sh`.

[0.1.0]: https://github.com/JaydenCJ/idemproof/releases/tag/v0.1.0
