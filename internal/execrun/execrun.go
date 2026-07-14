// Package execrun executes the command under proof with a pinned,
// reproducible harness: fixed working directory, /dev/null stdin, fully
// captured stdout/stderr, and an explicit exit code even for signal
// deaths. It is the only package in idemproof that starts a process.
package execrun

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// Result captures everything observable from one run.
type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

// Spec describes how to launch the command for every run.
type Spec struct {
	Argv  []string // resolved argv; Argv[0] is the program
	Dir   string   // working directory ("" = current)
	Env   []string // extra KEY=VAL entries appended to the inherited env
	Shell bool     // informational: Argv was built via a shell wrapper
}

// ShellSpec wraps a single shell string into an argv using /bin/sh -c,
// mirroring what CI systems and Makefiles do.
func ShellSpec(command string) Spec {
	return Spec{Argv: []string{"/bin/sh", "-c", command}, Shell: true}
}

// Run executes the spec once. A non-zero exit is NOT an error — the exit
// code is part of the observed behavior. An error is returned only when
// the process cannot be started at all (missing binary, bad directory).
func Run(spec Spec) (Result, error) {
	if len(spec.Argv) == 0 {
		return Result{}, errors.New("empty command")
	}
	cmd := exec.Command(spec.Argv[0], spec.Argv[1:]...)
	cmd.Dir = spec.Dir
	cmd.Env = append(os.Environ(), spec.Env...)
	cmd.Stdin = nil // /dev/null: interactive prompts must not block a proof
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	start := time.Now()
	err := cmd.Run()
	res := Result{
		Stdout:   outBuf.String(),
		Stderr:   errBuf.String(),
		Duration: time.Since(start),
	}
	if err == nil {
		return res, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		if res.ExitCode < 0 {
			// Killed by a signal. The shell's 128+N convention is not
			// portably recoverable from ExitCode, so report a plain 1.
			res.ExitCode = 1
		}
		return res, nil
	}
	return res, fmt.Errorf("cannot run %q: %w", spec.Argv[0], err)
}
