// Package report assembles the observations from N runs into a verdict
// and renders it for humans (text) and machines (JSON, schema_version 1).
// All verdict logic lives here, pure and unit-testable: the CLI only
// gathers inputs.
package report

import (
	"fmt"

	"github.com/JaydenCJ/idemproof/internal/fsdiff"
	"github.com/JaydenCJ/idemproof/internal/outdiff"
	"github.com/JaydenCJ/idemproof/internal/version"
)

// Verdict values.
const (
	VerdictIdempotent    = "idempotent"
	VerdictNotIdempotent = "not-idempotent"
)

// RunDetail records one execution and its filesystem effects relative to
// the state immediately before it (run 1's changes are the command's
// legitimate first-run work; changes in any later run are violations).
type RunDetail struct {
	Run        int             `json:"run"`
	ExitCode   int             `json:"exit_code"`
	DurationMS int64           `json:"duration_ms"`
	Changes    []fsdiff.Change `json:"filesystem_changes"`
}

// Output records the stream comparison between the last two runs.
type Output struct {
	Compared    bool           `json:"compared"`
	RunA        int            `json:"run_a,omitempty"`
	RunB        int            `json:"run_b,omitempty"`
	Normalizers []string       `json:"normalizers,omitempty"`
	Stdout      outdiff.Result `json:"stdout"`
	Stderr      outdiff.Result `json:"stderr"`
}

// Report is the full result of a proof.
type Report struct {
	Tool            string      `json:"tool"`
	SchemaVersion   int         `json:"schema_version"`
	Version         string      `json:"version"`
	Command         []string    `json:"command"`
	Shell           bool        `json:"shell"`
	Watch           []string    `json:"watch"`
	Runs            []RunDetail `json:"runs"`
	Output          Output      `json:"output"`
	AllowExitChange bool        `json:"allow_exit_change"`
	RequireZero     bool        `json:"require_zero"`
	LastEffectRun   int         `json:"last_effect_run"` // last run with fs changes; 0 = none at all
	Verdict         string      `json:"verdict"`
	Violations      []string    `json:"violations"`
}

// New seeds a report with the tool envelope.
func New(command []string, shell bool, watch []string) *Report {
	return &Report{
		Tool:          "idemproof",
		SchemaVersion: 1,
		Version:       version.Version,
		Command:       command,
		Shell:         shell,
		Watch:         watch,
		Violations:    []string{},
	}
}

// Finalize computes the verdict from the recorded runs and output
// comparison. It is idempotent itself: calling it twice yields the same
// report.
func (r *Report) Finalize() {
	r.Violations = r.Violations[:0]
	r.LastEffectRun = 0
	for _, run := range r.Runs {
		if len(run.Changes) > 0 {
			r.LastEffectRun = run.Run
		}
		if run.Run >= 2 && len(run.Changes) > 0 {
			r.Violations = append(r.Violations,
				fmt.Sprintf("run %d changed %s — a repeated run must be a filesystem no-op", run.Run, plural(len(run.Changes), "path")))
		}
	}
	if r.Output.Compared {
		if !r.Output.Stdout.Identical {
			r.Violations = append(r.Violations, streamViolation("stdout", r.Output))
		}
		if !r.Output.Stderr.Identical {
			r.Violations = append(r.Violations, streamViolation("stderr", r.Output))
		}
	}
	if !r.AllowExitChange && len(r.Runs) > 1 {
		first := r.Runs[0].ExitCode
		if run := r.exitDrift(first); run != nil {
			r.Violations = append(r.Violations,
				fmt.Sprintf("exit code drifted: run 1 exited %d, run %d exited %d", first, run.Run, run.ExitCode))
		}
	}
	if r.RequireZero {
		for _, run := range r.Runs {
			if run.ExitCode != 0 {
				r.Violations = append(r.Violations,
					fmt.Sprintf("run %d exited %d (--require-zero)", run.Run, run.ExitCode))
			}
		}
	}
	if len(r.Violations) == 0 {
		r.Verdict = VerdictIdempotent
	} else {
		r.Verdict = VerdictNotIdempotent
	}
}

// exitDrift returns the first later run whose exit code differs from the
// first run's, or nil.
func (r *Report) exitDrift(first int) *RunDetail {
	for i := 1; i < len(r.Runs); i++ {
		if r.Runs[i].ExitCode != first {
			return &r.Runs[i]
		}
	}
	return nil
}

func streamViolation(name string, o Output) string {
	res := o.Stdout
	if name == "stderr" {
		res = o.Stderr
	}
	if res.Note != "" {
		return fmt.Sprintf("%s differs between run %d and run %d (%s)", name, o.RunA, o.RunB, res.Note)
	}
	return fmt.Sprintf("%s differs between run %d and run %d (first at line %d)", name, o.RunA, o.RunB, res.FirstDiffLine)
}
