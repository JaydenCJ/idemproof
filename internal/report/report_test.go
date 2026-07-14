// Tests for verdict assembly and both renderers. The report layer is
// where every observation turns into a pass/fail claim, so each rule
// (fs no-op, output stability, exit-code stability, --require-zero)
// gets its own case, plus renderer determinism.
package report

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaydenCJ/idemproof/internal/fsdiff"
	"github.com/JaydenCJ/idemproof/internal/outdiff"
)

// base returns a two-run report that is perfectly idempotent; tests
// mutate one aspect each.
func base() *Report {
	r := New([]string{"/bin/sh", "-c", "true"}, true, []string{"."})
	r.Runs = []RunDetail{
		{Run: 1, ExitCode: 0, Changes: []fsdiff.Change{{Type: fsdiff.Created, Path: "out.txt", Kind: "file"}}},
		{Run: 2, ExitCode: 0},
	}
	r.Output = Output{
		Compared: true,
		RunA:     1,
		RunB:     2,
		Stdout:   outdiff.Compare("done\n", "done\n"),
		Stderr:   outdiff.Compare("", ""),
	}
	return r
}

func TestCleanSecondRunIsIdempotent(t *testing.T) {
	r := base()
	r.Finalize()
	if r.Verdict != VerdictIdempotent {
		t.Fatalf("expected idempotent, got %s (%v)", r.Verdict, r.Violations)
	}
	if r.LastEffectRun != 1 {
		t.Fatalf("expected convergence after run 1, got %d", r.LastEffectRun)
	}
	// Run 1 doing work is the whole point of a setup script; only
	// repeated-run changes count against the verdict.
	r.Runs[0].Changes = append(r.Runs[0].Changes,
		fsdiff.Change{Type: fsdiff.Created, Path: "more.txt", Kind: "file"})
	r.Finalize()
	if len(r.Violations) != 0 {
		t.Fatalf("first-run changes must not be violations: %v", r.Violations)
	}
}

func TestSecondRunFilesystemChangeIsViolation(t *testing.T) {
	r := base()
	r.Runs[1].Changes = []fsdiff.Change{{Type: fsdiff.Modified, Path: "out.txt", Kind: "file", Fields: []string{"content"}}}
	r.Finalize()
	if r.Verdict != VerdictNotIdempotent {
		t.Fatal("expected not-idempotent")
	}
	if len(r.Violations) != 1 || !strings.Contains(r.Violations[0], "run 2 changed 1 path") {
		t.Fatalf("unexpected violations: %v", r.Violations)
	}
	if r.LastEffectRun != 2 {
		t.Fatalf("expected last effect at run 2, got %d", r.LastEffectRun)
	}
}

func TestStdoutDriftIsViolation(t *testing.T) {
	r := base()
	r.Output.Stdout = outdiff.Compare("created 3 files\n", "nothing to do\n")
	r.Finalize()
	if r.Verdict != VerdictNotIdempotent {
		t.Fatal("expected not-idempotent")
	}
	if !strings.Contains(strings.Join(r.Violations, "\n"), "stdout differs between run 1 and run 2") {
		t.Fatalf("expected stdout violation, got %v", r.Violations)
	}
	r2 := base()
	r2.Output.Stderr = outdiff.Compare("", "warning: already exists\n")
	r2.Finalize()
	if !strings.Contains(strings.Join(r2.Violations, "\n"), "stderr differs") {
		t.Fatalf("expected stderr violation, got %v", r2.Violations)
	}
}

func TestOutputNotComparedMeansNoOutputViolations(t *testing.T) {
	r := base()
	r.Output = Output{Compared: false}
	r.Finalize()
	if r.Verdict != VerdictIdempotent {
		t.Fatalf("uncompared output must not fail the proof: %v", r.Violations)
	}
}

func TestExitDriftIsViolationUnlessAllowed(t *testing.T) {
	r := base()
	r.Runs[1].ExitCode = 1
	r.Finalize()
	if !strings.Contains(strings.Join(r.Violations, "\n"), "exit code drifted: run 1 exited 0, run 2 exited 1") {
		t.Fatalf("expected exit drift violation, got %v", r.Violations)
	}
	r2 := base()
	r2.Runs[1].ExitCode = 1
	r2.AllowExitChange = true
	r2.Finalize()
	if r2.Verdict != VerdictIdempotent {
		t.Fatalf("--allow-exit-change must suppress the drift violation: %v", r2.Violations)
	}
}

func TestRequireZeroFlagsNonzeroRuns(t *testing.T) {
	r := base()
	r.Runs[0].ExitCode = 3
	r.Runs[1].ExitCode = 3
	r.RequireZero = true
	r.Finalize()
	joined := strings.Join(r.Violations, "\n")
	if !strings.Contains(joined, "run 1 exited 3") || !strings.Contains(joined, "run 2 exited 3") {
		t.Fatalf("expected two require-zero violations, got %v", r.Violations)
	}
}

func TestFinalizeIsItselfIdempotent(t *testing.T) {
	r := base()
	r.Runs[1].ExitCode = 1
	r.Finalize()
	first := append([]string{}, r.Violations...)
	r.Finalize()
	if len(r.Violations) != len(first) {
		t.Fatalf("Finalize doubled violations: %v", r.Violations)
	}
}

func TestNoEffectsAtAllConvergenceMessage(t *testing.T) {
	r := base()
	r.Runs[0].Changes = nil
	r.Finalize()
	text := RenderText(r, false)
	if !strings.Contains(text, "no filesystem effects at all") {
		t.Fatalf("expected pure-command convergence message, got:\n%s", text)
	}
}

func TestTextRenderShapes(t *testing.T) {
	r := base()
	r.Runs[1].Changes = []fsdiff.Change{{
		Type: fsdiff.Modified, Path: "out.txt", Kind: "file",
		Fields: []string{"content", "size"}, Detail: "4 B -> 8 B",
	}}
	r.Finalize()
	text := RenderText(r, false)
	for _, want := range []string{
		"idemproof — 2 runs of: sh -c true",
		"run 1  exit 0   1 filesystem change",
		"first-run effects",
		"+ created   out.txt",
		"run 2 violations",
		"~ modified  out.txt   content, size (4 B -> 8 B)",
		"verdict: NOT IDEMPOTENT — 1 violation",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("text report missing %q:\n%s", want, text)
		}
	}
}

func TestQuietRenderIsOnlyTheVerdictLine(t *testing.T) {
	r := base()
	r.Finalize()
	text := RenderText(r, true)
	if text != "verdict: IDEMPOTENT — converged after run 1\n" {
		t.Fatalf("unexpected quiet output: %q", text)
	}
}

func TestTextRenderCapsLongChangeLists(t *testing.T) {
	r := base()
	r.Runs[0].Changes = nil
	for i := 0; i < 30; i++ {
		r.Runs[0].Changes = append(r.Runs[0].Changes,
			fsdiff.Change{Type: fsdiff.Created, Path: strings.Repeat("x", i+1), Kind: "file"})
	}
	r.Finalize()
	text := RenderText(r, false)
	if !strings.Contains(text, "… and 10 more") {
		t.Fatalf("expected capped listing, got:\n%s", text)
	}
}

func TestJSONRenderIsStableAndParseable(t *testing.T) {
	r := base()
	r.Finalize()
	out1, err := RenderJSON(r)
	if err != nil {
		t.Fatal(err)
	}
	out2, err := RenderJSON(r)
	if err != nil {
		t.Fatal(err)
	}
	if out1 != out2 {
		t.Fatal("JSON rendering must be deterministic")
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out1), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed["tool"] != "idemproof" || parsed["schema_version"] != float64(1) {
		t.Fatalf("missing envelope: %v", parsed)
	}
	if parsed["verdict"] != "idempotent" {
		t.Fatalf("wrong verdict in JSON: %v", parsed["verdict"])
	}
	if strings.Contains(out1, `"filesystem_changes": null`) {
		t.Fatal("nil change lists must render as [] for consumers")
	}
}

func TestDisplayCommandQuotesArguments(t *testing.T) {
	r := New([]string{"deploy", "--message", "hello world"}, false, []string{"."})
	r.Runs = []RunDetail{{Run: 1}, {Run: 2}}
	r.Finalize()
	text := RenderText(r, false)
	if !strings.Contains(text, "deploy --message 'hello world'") {
		t.Fatalf("arguments with spaces must be quoted:\n%s", text)
	}
}
