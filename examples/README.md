# idemproof examples

Both scripts are self-contained: they build `idemproof` from this repo,
work in a `mktemp -d` sandbox, and clean up after themselves.

| Script | What it shows |
|---|---|
| `prove-setup.sh` | Proving a realistic setup script idempotent, then breaking it on purpose and watching the proof fail with a precise violation. |
| `ci-gate.sh` | Using the exit code as a repeatability gate for a migration script, with `--runs 3` and `--format json` for machine consumption. |

Run them from the repository root:

```bash
bash examples/prove-setup.sh
bash examples/ci-gate.sh
```
