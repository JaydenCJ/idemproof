package report

import (
	"fmt"
	"strings"

	"github.com/JaydenCJ/idemproof/internal/fsdiff"
	"github.com/JaydenCJ/idemproof/internal/outdiff"
)

// maxListed caps per-section change listings so a proof over a huge tree
// stays readable; the counts are always exact.
const maxListed = 20

// RenderText renders the human report. quiet reduces it to the verdict
// line, for scripting and tight CI logs.
func RenderText(r *Report, quiet bool) string {
	var b strings.Builder
	if !quiet {
		fmt.Fprintf(&b, "idemproof — %d runs of: %s\n", len(r.Runs), displayCommand(r))
		fmt.Fprintf(&b, "watch: %s\n\n", strings.Join(r.Watch, ", "))
		for _, run := range r.Runs {
			fmt.Fprintf(&b, "run %d  exit %d   %s\n", run.Run, run.ExitCode, countChanges(len(run.Changes)))
		}
		if len(r.Runs) > 0 && len(r.Runs[0].Changes) > 0 {
			b.WriteString("\nfirst-run effects\n")
			writeChanges(&b, r.Runs[0].Changes)
		}
		for _, run := range r.Runs[1:] {
			if len(run.Changes) > 0 {
				fmt.Fprintf(&b, "\nrun %d violations\n", run.Run)
				writeChanges(&b, run.Changes)
			}
		}
		if r.Output.Compared {
			fmt.Fprintf(&b, "\noutput (run %d vs run %d%s)\n", r.Output.RunA, r.Output.RunB, normalizerSuffix(r.Output.Normalizers))
			writeStream(&b, "stdout", r.Output.Stdout, r.Output)
			writeStream(&b, "stderr", r.Output.Stderr, r.Output)
		}
		if len(r.Violations) > 0 {
			b.WriteString("\nviolations\n")
			for i, v := range r.Violations {
				fmt.Fprintf(&b, "  %d. %s\n", i+1, v)
			}
		}
		b.WriteString("\n")
	}
	b.WriteString(verdictLine(r))
	b.WriteString("\n")
	return b.String()
}

// verdictLine is the one-line summary shared by quiet and full modes.
func verdictLine(r *Report) string {
	if r.Verdict == VerdictIdempotent {
		return "verdict: IDEMPOTENT — " + convergence(r)
	}
	return "verdict: NOT IDEMPOTENT — " + plural(len(r.Violations), "violation")
}

// plural renders a count with a correctly pluralized noun: "1 line",
// "0 lines", "3 filesystem changes".
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func convergence(r *Report) string {
	if r.LastEffectRun == 0 {
		return "command has no filesystem effects at all"
	}
	return fmt.Sprintf("converged after run %d", r.LastEffectRun)
}

func countChanges(n int) string {
	return plural(n, "filesystem change")
}

func writeChanges(b *strings.Builder, changes []fsdiff.Change) {
	for i, c := range changes {
		if i == maxListed {
			fmt.Fprintf(b, "  … and %d more\n", len(changes)-maxListed)
			return
		}
		marker := map[fsdiff.ChangeType]string{
			fsdiff.Created:  "+ created ",
			fsdiff.Removed:  "- removed ",
			fsdiff.Modified: "~ modified",
		}[c.Type]
		line := fmt.Sprintf("  %s  %s", marker, c.Path)
		if c.Kind == "dir" {
			line += "/"
		}
		if len(c.Fields) > 0 {
			line += "   " + strings.Join(c.Fields, ", ")
			if c.Detail != "" {
				line += " (" + c.Detail + ")"
			}
		} else if c.Detail != "" {
			line += "   (" + c.Detail + ")"
		}
		b.WriteString(line + "\n")
	}
}

func writeStream(b *strings.Builder, name string, res outdiff.Result, o Output) {
	if res.Identical {
		fmt.Fprintf(b, "  %s: identical (%s)\n", name, plural(res.BLines, "line"))
		return
	}
	if res.Note != "" {
		fmt.Fprintf(b, "  %s: differs — %s\n", name, res.Note)
		return
	}
	fmt.Fprintf(b, "  %s: differs at line %d\n", name, res.FirstDiffLine)
	fmt.Fprintf(b, "    run %d | %s\n", o.RunA, orMissing(res.ALine))
	fmt.Fprintf(b, "    run %d | %s\n", o.RunB, orMissing(res.BLine))
}

func orMissing(line string) string {
	if line == "" {
		return "(missing)"
	}
	return line
}

func normalizerSuffix(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return ", normalized: " + strings.Join(names, ", ")
}

// displayCommand renders the argv the way the user typed it, quoting
// arguments that contain whitespace.
func displayCommand(r *Report) string {
	if r.Shell && len(r.Command) == 3 {
		return fmt.Sprintf("sh -c %s", quote(r.Command[2]))
	}
	parts := make([]string, len(r.Command))
	for i, a := range r.Command {
		parts[i] = quote(a)
	}
	return strings.Join(parts, " ")
}

func quote(s string) string {
	if s == "" || strings.ContainsAny(s, " \t\n'\"$&|;<>()") {
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return s
}
