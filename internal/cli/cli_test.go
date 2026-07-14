// End-to-end tests: the full CLI is driven in-process against real
// commands (/bin/sh with instant, deterministic snippets) in temp
// directories. Every exit code and every user-facing behavior of the
// README's CLI reference is proven here.
package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// run drives the CLI in-process and returns (exit code, stdout, stderr).
func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code := Run(args, &out, &errBuf)
	return code, out.String(), errBuf.String()
}

// proveShell is the common invocation shape: watch dir, run dir, shell
// command, plus any extra flags.
func proveShell(t *testing.T, dir, command string, extra ...string) (int, string, string) {
	t.Helper()
	args := append([]string{"--watch", dir, "--dir", dir}, extra...)
	args = append(args, "--shell", "--", command)
	return run(t, args...)
}

// counterCmd returns a shell snippet whose output changes deterministically
// on every run by bumping a counter file OUTSIDE the watched directory.
func counterCmd(t *testing.T, format string) string {
	t.Helper()
	c := filepath.Join(t.TempDir(), "counter")
	return `n=$(cat ` + c + ` 2>/dev/null || echo 0); n=$((n+1)); echo $n > ` + c + `; ` + format
}

func TestVersionSubcommandAndFlag(t *testing.T) {
	for _, argv := range [][]string{{"version"}, {"--version"}} {
		code, out, _ := run(t, argv...)
		if code != ExitIdempotent || out != "idemproof 0.1.0\n" {
			t.Fatalf("%v: code=%d out=%q", argv, code, out)
		}
	}
}

func TestHelpPrintsUsage(t *testing.T) {
	code, out, _ := run(t, "--help")
	if code != ExitIdempotent {
		t.Fatalf("--help exit = %d", code)
	}
	for _, want := range []string{"Usage:", "--watch", "--normalize", "Exit codes"} {
		if !strings.Contains(out, want) {
			t.Fatalf("usage missing %q", want)
		}
	}
}

func TestUsageErrorsExitTwo(t *testing.T) {
	cases := [][]string{
		{},                                     // no command at all
		{"--format", "yaml", "--", "true"},     // bad format
		{"--runs", "1", "--", "true"},          // below minimum
		{"--runs", "11", "--", "true"},         // above maximum
		{"--normalize", "bogus", "--", "true"}, // unknown normalizer
		{"--scrub", "(", "--", "true"},         // invalid regexp
		{"--env", "NOEQUALS", "--", "true"},    // malformed env
	}
	for _, argv := range cases {
		code, _, errOut := run(t, argv...)
		if code != ExitUsage {
			t.Fatalf("%v: expected exit %d, got %d (stderr %q)", argv, ExitUsage, code, errOut)
		}
		if !strings.Contains(errOut, "idemproof:") {
			t.Fatalf("%v: stderr should carry a diagnostic, got %q", argv, errOut)
		}
	}
}

func TestRuntimeErrorsExitThree(t *testing.T) {
	dir := t.TempDir()
	// Missing binary.
	code, _, errOut := run(t, "--watch", dir, "--", "/nonexistent/no-such-tool")
	if code != ExitRuntime || !strings.Contains(errOut, "cannot run") {
		t.Fatalf("missing binary: code=%d stderr=%q", code, errOut)
	}
	// Missing watch directory.
	code, _, errOut = run(t, "--watch", filepath.Join(dir, "missing"), "--", "/bin/sh", "-c", "true")
	if code != ExitRuntime || !strings.Contains(errOut, "watch path") {
		t.Fatalf("missing watch dir: code=%d stderr=%q", code, errOut)
	}
}

func TestIdempotentSetupPasses(t *testing.T) {
	dir := t.TempDir()
	code, out, _ := proveShell(t, dir, `mkdir -p app/config && printf 'port=8080\n' > app/config/app.conf`)
	if code != ExitIdempotent {
		t.Fatalf("expected exit 0, got %d:\n%s", code, out)
	}
	for _, want := range []string{
		"run 1  exit 0   3 filesystem changes",
		"run 2  exit 0   0 filesystem changes",
		"+ created   app/",
		"+ created   app/config/app.conf",
		"verdict: IDEMPOTENT — converged after run 1",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("report missing %q:\n%s", want, out)
		}
	}
}

func TestAppendingCommandFails(t *testing.T) {
	dir := t.TempDir()
	code, out, _ := proveShell(t, dir, "echo run >> log.txt")
	if code != ExitNotIdempotent {
		t.Fatalf("expected exit 1, got %d:\n%s", code, out)
	}
	for _, want := range []string{
		"run 2 violations",
		"~ modified  log.txt   content, size (4 B -> 8 B)",
		"verdict: NOT IDEMPOTENT — 1 violation",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("report missing %q:\n%s", want, out)
		}
	}
}

func TestOutputDriftAloneFailsTheProof(t *testing.T) {
	dir := t.TempDir()
	// The counter lives outside the watch dir: the filesystem check is
	// clean, so only the output comparison can catch the drift.
	code, out, _ := proveShell(t, dir, counterCmd(t, `echo "attempt $n"`))
	if code != ExitNotIdempotent {
		t.Fatalf("expected exit 1, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "stdout: differs at line 1") {
		t.Fatalf("expected located stdout drift:\n%s", out)
	}
	if !strings.Contains(out, "run 1 | attempt 1") || !strings.Contains(out, "run 2 | attempt 2") {
		t.Fatalf("expected both differing lines quoted:\n%s", out)
	}
}

func TestNoOutputFlagSkipsStreamComparison(t *testing.T) {
	dir := t.TempDir()
	code, out, _ := proveShell(t, dir, counterCmd(t, `echo "attempt $n"`), "--no-output")
	if code != ExitIdempotent {
		t.Fatalf("--no-output should ignore stdout drift, got %d:\n%s", code, out)
	}
	if strings.Contains(out, "output (") {
		t.Fatalf("output section should be absent with --no-output:\n%s", out)
	}
}

func TestNormalizeDurationsAbsorbsTimingNoise(t *testing.T) {
	dir := t.TempDir()
	cmd := counterCmd(t, `echo "finished in ${n}ms"`)
	// Without normalization the fake timing differs...
	code, _, _ := proveShell(t, dir, cmd)
	if code != ExitNotIdempotent {
		t.Fatalf("expected drift without normalizer, got %d", code)
	}
	// ...with --normalize durations both runs read "finished in <DURATION>".
	dir2 := t.TempDir()
	code, out, _ := proveShell(t, dir2, cmd, "--normalize", "durations")
	if code != ExitIdempotent {
		t.Fatalf("expected pass with normalizer, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "normalized: durations") {
		t.Fatalf("report should list active normalizers:\n%s", out)
	}
}

func TestCustomScrubPattern(t *testing.T) {
	dir := t.TempDir()
	cmd := counterCmd(t, `echo "session id session-$n ready"`)
	code, out, _ := proveShell(t, dir, cmd, "--scrub", `session-[0-9]+`)
	if code != ExitIdempotent {
		t.Fatalf("custom scrub should absorb the drift, got %d:\n%s", code, out)
	}
}

func TestRunsThreeProvesConvergence(t *testing.T) {
	dir := t.TempDir()
	cmd := `[ -f done ] || { echo setup; touch done; }`
	// Default 2 runs: stdout "setup" vs "" is drift — strict and correct.
	code, _, _ := proveShell(t, dir, cmd)
	if code != ExitNotIdempotent {
		t.Fatalf("2-run proof should flag first-run-only output, got %d", code)
	}
	// 3 runs: output compared between runs 2 and 3, both silent no-ops.
	dir2 := t.TempDir()
	code, out, _ := proveShell(t, dir2, cmd, "--runs", "3")
	if code != ExitIdempotent {
		t.Fatalf("3-run proof should pass, got %d:\n%s", code, out)
	}
	for _, want := range []string{"run 3  exit 0", "output (run 2 vs run 3)", "converged after run 1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("report missing %q:\n%s", want, out)
		}
	}
}

func TestExitCodeDriftAndItsWaiver(t *testing.T) {
	dir := t.TempDir()
	// Creates a guard file, then exits 1 on every later run: a classic
	// "not re-runnable" migration.
	cmd := `[ -f guard ] && exit 1; touch guard`
	code, out, _ := proveShell(t, dir, cmd)
	if code != ExitNotIdempotent {
		t.Fatalf("expected exit-code drift violation, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "exit code drifted: run 1 exited 0, run 2 exited 1") {
		t.Fatalf("expected drift message:\n%s", out)
	}
	dir2 := t.TempDir()
	code, out, _ = proveShell(t, dir2, cmd, "--allow-exit-change")
	if code != ExitIdempotent {
		t.Fatalf("--allow-exit-change should waive the drift, got %d:\n%s", code, out)
	}
}

func TestRequireZero(t *testing.T) {
	dir := t.TempDir()
	// Stable nonzero exit: idempotent by default (behavior is identical)…
	code, _, _ := proveShell(t, dir, "exit 5")
	if code != ExitIdempotent {
		t.Fatalf("stable nonzero exit should pass by default, got %d", code)
	}
	// …but --require-zero turns it into a violation.
	code, out, _ := proveShell(t, dir, "exit 5", "--require-zero")
	if code != ExitNotIdempotent || !strings.Contains(out, "(--require-zero)") {
		t.Fatalf("expected require-zero violation, got %d:\n%s", code, out)
	}
}

func TestIgnoreGlobExcludesLogNoise(t *testing.T) {
	dir := t.TempDir()
	cmd := `mkdir -p state && touch state/ready && echo ran >> debug.log`
	code, _, _ := proveShell(t, dir, cmd)
	if code != ExitNotIdempotent {
		t.Fatalf("log append should fail without ignore, got %d", code)
	}
	dir2 := t.TempDir()
	code, out, _ := proveShell(t, dir2, cmd, "--ignore", "*.log")
	if code != ExitIdempotent {
		t.Fatalf("--ignore '*.log' should pass, got %d:\n%s", code, out)
	}
	for _, leak := range []string{"+ created   debug.log", "~ modified  debug.log"} {
		if strings.Contains(out, leak) {
			t.Fatalf("ignored file leaked into the report:\n%s", out)
		}
	}
}

func TestJSONFormatIsMachineReadable(t *testing.T) {
	dir := t.TempDir()
	code, out, _ := proveShell(t, dir, "echo run >> log.txt", "--format", "json")
	if code != ExitNotIdempotent {
		t.Fatalf("expected exit 1, got %d", code)
	}
	var rep struct {
		Tool          string   `json:"tool"`
		SchemaVersion int      `json:"schema_version"`
		Verdict       string   `json:"verdict"`
		Violations    []string `json:"violations"`
		Runs          []struct {
			Run      int `json:"run"`
			ExitCode int `json:"exit_code"`
			Changes  []struct {
				Type string `json:"type"`
				Path string `json:"path"`
			} `json:"filesystem_changes"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if rep.Tool != "idemproof" || rep.SchemaVersion != 1 || rep.Verdict != "not-idempotent" {
		t.Fatalf("bad envelope: %+v", rep)
	}
	if len(rep.Runs) != 2 || len(rep.Runs[1].Changes) != 1 || rep.Runs[1].Changes[0].Path != "log.txt" {
		t.Fatalf("bad runs payload: %+v", rep.Runs)
	}
	if len(rep.Violations) == 0 {
		t.Fatal("violations missing from JSON")
	}
}

func TestQuietPrintsOnlyTheVerdictLine(t *testing.T) {
	dir := t.TempDir()
	code, out, _ := proveShell(t, dir, "mkdir -p sub", "--quiet")
	if code != ExitIdempotent {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if out != "verdict: IDEMPOTENT — converged after run 1\n" {
		t.Fatalf("quiet output = %q", out)
	}
}

func TestMultipleWatchRootsPrefixPaths(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	// Idempotent in root a, appending in root b.
	cmd := `touch ` + a + `/flag; echo x >> ` + b + `/grow.txt`
	code, out, _ := run(t, "--watch", a, "--watch", b, "--shell", "--", cmd)
	if code != ExitNotIdempotent {
		t.Fatalf("expected exit 1, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, b+"/grow.txt") {
		t.Fatalf("violation path should be prefixed with its watch root:\n%s", out)
	}
}

func TestCommandFlagsAfterDashDashAreNotParsed(t *testing.T) {
	dir := t.TempDir()
	// "--format json" here belongs to the command, not to idemproof.
	code, out, _ := run(t, "--watch", dir, "--", "/bin/echo", "--format", "json")
	if code != ExitIdempotent {
		t.Fatalf("expected exit 0, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "verdict: IDEMPOTENT") || strings.Contains(out, `"tool"`) {
		t.Fatalf("flags leaked across --:\n%s", out)
	}
	if !strings.Contains(out, "no filesystem effects at all") {
		t.Fatalf("pure echo should have no effects:\n%s", out)
	}
}

func TestStrictTimesFlagsTouch(t *testing.T) {
	dir := t.TempDir()
	// touch -t pins two different mtimes on the same content, so the
	// proof is deterministic: run 2 always flips the timestamp.
	cmd := `if [ -f stamp ]; then touch -t 202601020304 stamp; else echo hi > stamp; touch -t 202601010000 stamp; fi`
	code, _, _ := proveShell(t, dir, cmd, "--allow-exit-change")
	if code != ExitIdempotent {
		t.Fatalf("mtime flip must be invisible by default, got %d", code)
	}
	dir2 := t.TempDir()
	code, out, _ := proveShell(t, dir2, cmd, "--allow-exit-change", "--strict-times")
	if code != ExitNotIdempotent || !strings.Contains(out, "mtime") {
		t.Fatalf("--strict-times should flag the touch, got %d:\n%s", code, out)
	}
}
