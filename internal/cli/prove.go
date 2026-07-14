package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/JaydenCJ/idemproof/internal/execrun"
	"github.com/JaydenCJ/idemproof/internal/fsdiff"
	"github.com/JaydenCJ/idemproof/internal/outdiff"
	"github.com/JaydenCJ/idemproof/internal/report"
	"github.com/JaydenCJ/idemproof/internal/scrub"
	"github.com/JaydenCJ/idemproof/internal/snapshot"
)

// prove executes the full proof loop and renders the report.
func prove(opts *options, stdout, stderr io.Writer) int {
	scrubber, err := scrub.Build(opts.normalize, opts.scrubExprs)
	if err != nil { // validate() already vetted this; belt and braces
		fmt.Fprintf(stderr, "idemproof: %v\n", err)
		return ExitUsage
	}

	spec := buildSpec(opts)
	snapOpts := snapshot.Options{
		Ignore:      opts.ignore,
		Times:       opts.strictTimes,
		MaxFileSize: opts.maxFileSize,
	}

	labels := make([]string, len(opts.watch))
	for i, w := range opts.watch {
		labels[i] = snapshot.Label(w)
	}
	rep := report.New(spec.Argv, spec.Shell, labels)
	rep.AllowExitChange = opts.allowExitChange
	rep.RequireZero = opts.requireZero

	prev, err := takeAll(opts.watch, snapOpts)
	if err != nil {
		fmt.Fprintf(stderr, "idemproof: %v\n", err)
		return ExitRuntime
	}

	results := make([]execrun.Result, 0, opts.runs)
	for i := 1; i <= opts.runs; i++ {
		res, err := execrun.Run(spec)
		if err != nil {
			fmt.Fprintf(stderr, "idemproof: run %d: %v\n", i, err)
			return ExitRuntime
		}
		results = append(results, res)

		cur, err := takeAll(opts.watch, snapOpts)
		if err != nil {
			fmt.Fprintf(stderr, "idemproof: %v\n", err)
			return ExitRuntime
		}
		rep.Runs = append(rep.Runs, report.RunDetail{
			Run:        i,
			ExitCode:   res.ExitCode,
			DurationMS: res.Duration.Milliseconds(),
			Changes:    diffAll(prev, cur, labels),
		})
		prev = cur
	}

	if !opts.noOutput {
		a, b := results[len(results)-2], results[len(results)-1]
		rep.Output = report.Output{
			Compared:    true,
			RunA:        opts.runs - 1,
			RunB:        opts.runs,
			Normalizers: scrubber.Active(),
			Stdout:      outdiff.Compare(scrubber.Apply(a.Stdout), scrubber.Apply(b.Stdout)),
			Stderr:      outdiff.Compare(scrubber.Apply(a.Stderr), scrubber.Apply(b.Stderr)),
		}
	}
	rep.Finalize()

	var rendered string
	if opts.format == "json" {
		rendered, err = report.RenderJSON(rep)
		if err != nil {
			fmt.Fprintf(stderr, "idemproof: %v\n", err)
			return ExitRuntime
		}
	} else {
		rendered = report.RenderText(rep, opts.quiet)
	}
	fmt.Fprint(stdout, rendered)

	if rep.Verdict == report.VerdictIdempotent {
		return ExitIdempotent
	}
	return ExitNotIdempotent
}

// buildSpec resolves the command spec from options.
func buildSpec(opts *options) execrun.Spec {
	var spec execrun.Spec
	if opts.shell {
		spec = execrun.ShellSpec(strings.Join(opts.command, " "))
	} else {
		spec = execrun.Spec{Argv: opts.command}
	}
	spec.Dir = opts.dir
	spec.Env = opts.env
	return spec
}

// takeAll snapshots every watch root, in flag order.
func takeAll(roots []string, opts snapshot.Options) ([]*snapshot.Snapshot, error) {
	snaps := make([]*snapshot.Snapshot, len(roots))
	for i, root := range roots {
		s, err := snapshot.Take(root, opts)
		if err != nil {
			return nil, err
		}
		snaps[i] = s
	}
	return snaps, nil
}

// diffAll diffs each root pairwise and merges the changes. With multiple
// watch roots, paths are prefixed with their root label so a report line
// is unambiguous.
func diffAll(before, after []*snapshot.Snapshot, labels []string) []fsdiff.Change {
	var all []fsdiff.Change
	for i := range after {
		changes := fsdiff.Diff(before[i], after[i])
		if len(labels) > 1 {
			for j := range changes {
				changes[j].Path = labels[i] + "/" + changes[j].Path
			}
		}
		all = append(all, changes...)
	}
	return all
}
