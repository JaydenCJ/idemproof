# How idemproof decides

A command is *idempotent* when running it once and running it N times
leave the system in the same state, with the same observable behavior.
idemproof turns that definition into a mechanical proof.

## The proof loop

```
snapshot S0
run 1            → effects(1) = diff(S0, S1)     # legitimate work
snapshot S1
run 2            → effects(2) = diff(S1, S2)     # must be empty
snapshot S2
…                                                # for --runs N
compare output of the final two runs
```

- **Run 1 may change anything.** Its diff is reported as *first-run
  effects*, purely informational — a setup script that does nothing would
  be suspicious, not virtuous.
- **Every later run must be a filesystem no-op.** Any created, removed,
  or modified path in run ≥ 2 is a violation.
- **Output must stabilize.** stdout and stderr of the final two runs are
  compared byte-for-byte (after optional normalization). With the default
  `--runs 2` this compares run 1 vs run 2, which is strict: a tool that
  prints "created 3 files" then "nothing to do" fails. That is often
  exactly what you want to know; when it is not, use `--runs 3` so the
  comparison happens between two steady-state runs, or `--no-output`.
- **The exit code must not drift** across runs (waivable with
  `--allow-exit-change`). `--require-zero` additionally demands success.

## What a snapshot records

| Attribute | Compared | Notes |
|---|---|---|
| existence + kind | always | file / dir / symlink / other |
| content (SHA-256) | always for files | catches same-size rewrites |
| size | always for files | reported with a human detail (`4 B -> 8 B`) |
| permission bits | always | `chmod` drift is a real effect |
| symlink target | always | retargeting counts as a modification |
| mtime | only with `--strict-times` | noisy on most filesystems, off by default |

Files larger than `--max-file-size` (default 256 MiB) are compared by
size only, so proofs over large data directories stay fast.

Ignored paths (`--ignore`, glob dialect `*`, `?`, `**`; bare names match
at any depth like `.gitignore`) are pruned before hashing — an ignored
directory's subtree is never even walked.

## Scope and honesty

idemproof observes the filesystem under `--watch` (plus streams and exit
codes) and nothing else. Effects outside that scope — database rows,
remote APIs, running processes — are invisible to the proof. Point
`--watch` at every directory the command may touch, and remember that a
passing proof is evidence under the observed scope, not a universal
guarantee. Two runs also cannot detect effects that only trip on run 47;
`--runs` up to 10 raises confidence, not certainty.

## Normalizers

Volatile-but-legitimate tokens can be scrubbed from output before
comparison with `--normalize` (built-ins) and `--scrub REGEXP` (custom,
replaced with `<SCRUBBED>`):

| Name | Replaces | With |
|---|---|---|
| `timestamps` | ISO 8601 dates/times (`2026-07-13T09:15:42Z`) | `<TIMESTAMP>` |
| `times` | bare clock times (`12:04:59`) | `<TIME>` |
| `uuids` | RFC 4122 UUIDs | `<UUID>` |
| `hex` | lowercase hex runs of 12–64 chars | `<HEX>` |
| `pids` | `pid=1234`, `PID: 1234` | `pid <PID>` |
| `durations` | `1.42s`, `350ms`, `3 min` | `<DURATION>` |
| `tmppaths` | `/tmp/...` paths | `<TMPPATH>` |
| `all` | every built-in above | — |

Normalization applies only to the output comparison, never to the
filesystem diff, and the report always lists which normalizers were
active so a "pass" can be audited.
