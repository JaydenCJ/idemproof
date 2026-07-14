// Package cli parses arguments and orchestrates a proof: snapshot,
// run, snapshot, run, snapshot, diff, verdict. All decision logic lives
// in the pure packages underneath; this layer only wires them together.
package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/JaydenCJ/idemproof/internal/scrub"
	"github.com/JaydenCJ/idemproof/internal/version"
)

// Exit codes, aligned with the README's CLI reference.
const (
	ExitIdempotent    = 0
	ExitNotIdempotent = 1
	ExitUsage         = 2
	ExitRuntime       = 3
)

const (
	minRuns = 2
	maxRuns = 10
)

// multiFlag collects a repeatable string flag.
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

// options is the fully parsed invocation.
type options struct {
	watch           []string
	ignore          []string
	runs            int
	format          string
	normalize       []string
	scrubExprs      []string
	noOutput        bool
	allowExitChange bool
	requireZero     bool
	strictTimes     bool
	shell           bool
	quiet           bool
	dir             string
	env             []string
	maxFileSize     int64
	command         []string
}

// Run is the entire CLI: parse, prove, render. It returns the process
// exit code and never calls os.Exit, so tests drive it in-process.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "version" || args[0] == "--version" || args[0] == "-V") {
		fmt.Fprintf(stdout, "idemproof %s\n", version.Version)
		return ExitIdempotent
	}
	opts, err := parse(args)
	if err != nil {
		fmt.Fprintf(stderr, "idemproof: %v\n", err)
		fmt.Fprintf(stderr, "run 'idemproof --help' for usage\n")
		return ExitUsage
	}
	if opts == nil { // --help
		fmt.Fprint(stdout, usage)
		return ExitIdempotent
	}
	return prove(opts, stdout, stderr)
}

// parse turns raw args into options. A nil options with nil error means
// help was requested.
func parse(args []string) (*options, error) {
	opts := &options{}
	var watch, ignore, normalize, scrubExprs, env multiFlag
	fs := flag.NewFlagSet("idemproof", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Var(&watch, "watch", "directory to watch for effects (repeatable; default .)")
	fs.Var(&ignore, "ignore", "glob of paths to ignore (repeatable; *, ?, **)")
	fs.IntVar(&opts.runs, "runs", 2, "number of runs (2-10)")
	fs.StringVar(&opts.format, "format", "text", "report format: text or json")
	fs.Var(&normalize, "normalize", "built-in output normalizers, comma-separable (see --help)")
	fs.Var(&scrubExprs, "scrub", "custom regexp scrubbed from output before comparison (repeatable)")
	fs.BoolVar(&opts.noOutput, "no-output", false, "skip stdout/stderr comparison")
	fs.BoolVar(&opts.allowExitChange, "allow-exit-change", false, "do not require a stable exit code across runs")
	fs.BoolVar(&opts.requireZero, "require-zero", false, "every run must exit 0")
	fs.BoolVar(&opts.strictTimes, "strict-times", false, "treat mtime-only changes as effects")
	fs.BoolVar(&opts.shell, "shell", false, "run the command through /bin/sh -c")
	fs.BoolVar(&opts.quiet, "quiet", false, "print only the verdict line")
	fs.StringVar(&opts.dir, "dir", "", "working directory for the command")
	fs.Var(&env, "env", "extra KEY=VAL for the command's environment (repeatable)")
	fs.Int64Var(&opts.maxFileSize, "max-file-size", 256<<20, "files larger than N bytes are compared by size only")
	help := fs.Bool("help", false, "show usage")

	// Everything after a literal "--" is the command, untouched by the
	// flag parser so the command's own flags survive.
	flagArgs, command := splitAtDashDash(args)
	if err := fs.Parse(flagArgs); err != nil {
		if err == flag.ErrHelp {
			return nil, nil
		}
		return nil, err
	}
	if *help {
		return nil, nil
	}
	if command == nil {
		command = fs.Args()
	} else {
		command = append(command, fs.Args()...)
	}
	opts.watch, opts.ignore, opts.scrubExprs, opts.env = watch, ignore, scrubExprs, env
	for _, n := range normalize {
		for _, part := range strings.Split(n, ",") {
			if part = strings.TrimSpace(part); part != "" {
				opts.normalize = append(opts.normalize, part)
			}
		}
	}
	opts.command = command
	return opts, validate(opts)
}

// splitAtDashDash separates flag arguments from the command after "--".
// It returns (flags, nil) when no "--" is present.
func splitAtDashDash(args []string) (flags []string, command []string) {
	for i, a := range args {
		if a == "--" {
			return args[:i], append([]string{}, args[i+1:]...)
		}
	}
	return args, nil
}

func validate(opts *options) error {
	if len(opts.command) == 0 {
		return fmt.Errorf("no command given (usage: idemproof [flags] -- <command> [args...])")
	}
	if opts.runs < minRuns || opts.runs > maxRuns {
		return fmt.Errorf("--runs must be between %d and %d, got %d", minRuns, maxRuns, opts.runs)
	}
	if opts.format != "text" && opts.format != "json" {
		return fmt.Errorf("--format must be text or json, got %q", opts.format)
	}
	if opts.maxFileSize < 0 {
		return fmt.Errorf("--max-file-size must be >= 0")
	}
	for _, e := range opts.env {
		if !strings.Contains(e, "=") {
			return fmt.Errorf("--env entries must look like KEY=VAL, got %q", e)
		}
	}
	if _, err := scrub.Build(opts.normalize, opts.scrubExprs); err != nil {
		return err
	}
	if len(opts.watch) == 0 {
		opts.watch = []string{"."}
	}
	return nil
}

var usage = `idemproof ` + version.Version + ` — prove a command is idempotent

Usage:
  idemproof [flags] -- <command> [args...]
  idemproof --shell [flags] -- '<shell string>'
  idemproof version | --version

The command is run --runs times (default 2). Run 1 may change the
filesystem (its legitimate work); every later run must be a no-op.
Stdout/stderr of the final two runs are compared byte-for-byte after
optional normalization, and the exit code must stay stable.

Exit codes: 0 idempotent · 1 not idempotent · 2 usage error · 3 runtime error

Flags:
  --watch DIR            directory to watch for effects (repeatable; default .)
  --ignore GLOB          ignore matching paths (repeatable; *, ?, ** supported)
  --runs N               number of runs, 2-10 (default 2)
  --format FMT           text (default) or json
  --normalize NAMES      built-in output normalizers, comma-separable:
                         ` + strings.Join(scrub.Names(), ", ") + `, all
  --scrub REGEXP         custom pattern scrubbed from output (repeatable)
  --no-output            skip stdout/stderr comparison
  --allow-exit-change    do not require a stable exit code across runs
  --require-zero         every run must exit 0
  --strict-times         treat mtime-only changes as effects
  --shell                run the command through /bin/sh -c
  --dir DIR              working directory for the command
  --env KEY=VAL          extra environment for the command (repeatable)
  --max-file-size N      files larger than N bytes are compared by size
                         only, not hashed (default 268435456)
  --quiet                print only the verdict line
`
