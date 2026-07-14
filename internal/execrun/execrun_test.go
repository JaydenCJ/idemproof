// Tests for the process harness. These execute /bin/sh with trivial,
// instant commands — offline and deterministic, but real processes, so
// exit-code plumbing and stream capture are proven against the OS.
package execrun

import (
	"strings"
	"testing"
)

func TestCapturesStdoutAndStderrSeparately(t *testing.T) {
	spec := ShellSpec("echo out; echo err >&2")
	if len(spec.Argv) != 3 || spec.Argv[0] != "/bin/sh" || spec.Argv[1] != "-c" || !spec.Shell {
		t.Fatalf("unexpected shell spec: %+v", spec)
	}
	res, err := Run(spec)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stdout != "out\n" {
		t.Fatalf("stdout = %q", res.Stdout)
	}
	if res.Stderr != "err\n" {
		t.Fatalf("stderr = %q", res.Stderr)
	}
}

func TestNonZeroExitIsAResultNotAnError(t *testing.T) {
	res, err := Run(ShellSpec("exit 7"))
	if err != nil {
		t.Fatalf("nonzero exit must not be an error: %v", err)
	}
	if res.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", res.ExitCode)
	}
}

func TestMissingBinaryIsAStartError(t *testing.T) {
	_, err := Run(Spec{Argv: []string{"/nonexistent/idemproof-no-such-binary"}})
	if err == nil {
		t.Fatal("expected a start error for a missing binary")
	}
	if !strings.Contains(err.Error(), "cannot run") {
		t.Fatalf("unhelpful error: %v", err)
	}
}

func TestEmptyArgvIsAnError(t *testing.T) {
	if _, err := Run(Spec{}); err == nil {
		t.Fatal("expected error for empty argv")
	}
}

func TestWorkingDirectoryIsApplied(t *testing.T) {
	dir := t.TempDir()
	res, err := Run(Spec{Argv: []string{"/bin/sh", "-c", "pwd"}, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	// macOS tempdirs resolve through /private; compare by suffix.
	if !strings.HasSuffix(strings.TrimSpace(res.Stdout), strings.TrimPrefix(dir, "/private")) {
		t.Fatalf("pwd = %q, want %q", res.Stdout, dir)
	}
}

func TestExtraEnvIsVisibleToTheCommand(t *testing.T) {
	res, err := Run(Spec{
		Argv: []string{"/bin/sh", "-c", `printf '%s' "$IDEMPROOF_PROBE"`},
		Env:  []string{"IDEMPROOF_PROBE=marker-42"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Stdout != "marker-42" {
		t.Fatalf("env not delivered: %q", res.Stdout)
	}
}

func TestStdinIsClosedNotInteractive(t *testing.T) {
	// A command that reads stdin must see EOF immediately, never hang
	// waiting for a terminal — proofs run unattended.
	res, err := Run(ShellSpec("cat; echo done"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Stdout != "done\n" {
		t.Fatalf("stdin was not /dev/null: %q", res.Stdout)
	}
}
