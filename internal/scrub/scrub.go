// Package scrub normalizes volatile tokens out of command output before
// comparison. Idempotent tools often print timestamps, PIDs, temp paths,
// or request IDs that legitimately differ between runs; scrubbing replaces
// each with a stable placeholder so only meaningful drift is flagged.
package scrub

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// rule is one named normalizer: a compiled pattern and its replacement.
type rule struct {
	name string
	re   *regexp.Regexp
	repl string
}

// named holds every built-in normalizer, applied in declaration order.
// Timestamps run before hex so date digits are never half-eaten.
var named = []rule{
	{
		name: "timestamps",
		// ISO 8601 dates with optional time, fraction, and zone.
		re:   regexp.MustCompile(`\d{4}-\d{2}-\d{2}([T ]\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:?\d{2})?)?`),
		repl: "<TIMESTAMP>",
	},
	{
		name: "times",
		// Bare wall-clock times (log prefixes like "12:04:59").
		re:   regexp.MustCompile(`\b\d{2}:\d{2}:\d{2}\b`),
		repl: "<TIME>",
	},
	{
		name: "uuids",
		re:   regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`),
		repl: "<UUID>",
	},
	{
		name: "hex",
		// Long lowercase hex runs: digests, request IDs, build hashes.
		re:   regexp.MustCompile(`\b[0-9a-f]{12,64}\b`),
		repl: "<HEX>",
	},
	{
		name: "pids",
		re:   regexp.MustCompile(`(?i)\b(pid)\s*[=: ]\s*\d+`),
		repl: "$1 <PID>",
	},
	{
		name: "durations",
		re:   regexp.MustCompile(`\b\d+(\.\d+)?\s?(ns|µs|us|ms|s|sec|secs|seconds?|min|mins|minutes?)\b`),
		repl: "<DURATION>",
	},
	{
		name: "tmppaths",
		re:   regexp.MustCompile(`/tmp/[A-Za-z0-9._/-]+`),
		repl: "<TMPPATH>",
	},
}

// Names lists the built-in normalizer names, sorted, for --help and errors.
func Names() []string {
	out := make([]string, len(named))
	for i, r := range named {
		out[i] = r.name
	}
	sort.Strings(out)
	return out
}

// Scrubber applies a fixed sequence of normalizers to text.
type Scrubber struct {
	rules []rule
}

// Build assembles a scrubber from built-in normalizer names (or the alias
// "all") plus custom regular expressions, which replace their matches with
// <SCRUBBED>. An unknown name or an invalid regexp is a usage error.
func Build(names []string, custom []string) (*Scrubber, error) {
	s := &Scrubber{}
	seen := map[string]bool{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		if n == "all" {
			for _, r := range named {
				if !seen[r.name] {
					seen[r.name] = true
					s.rules = append(s.rules, r)
				}
			}
			continue
		}
		r, ok := lookup(n)
		if !ok {
			return nil, fmt.Errorf("unknown normalizer %q (available: %s, all)", n, strings.Join(Names(), ", "))
		}
		s.rules = append(s.rules, r)
	}
	// Keep built-in application order stable regardless of flag order.
	sort.SliceStable(s.rules, func(i, j int) bool {
		return declIndex(s.rules[i].name) < declIndex(s.rules[j].name)
	})
	for _, expr := range custom {
		re, err := regexp.Compile(expr)
		if err != nil {
			return nil, fmt.Errorf("invalid --scrub pattern %q: %v", expr, err)
		}
		s.rules = append(s.rules, rule{name: "custom", re: re, repl: "<SCRUBBED>"})
	}
	return s, nil
}

// Active reports the names of the rules this scrubber applies, in order.
func (s *Scrubber) Active() []string {
	out := make([]string, len(s.rules))
	for i, r := range s.rules {
		out[i] = r.name
	}
	return out
}

// Apply runs every rule over the text, in order.
func (s *Scrubber) Apply(text string) string {
	for _, r := range s.rules {
		text = r.re.ReplaceAllString(text, r.repl)
	}
	return text
}

func lookup(name string) (rule, bool) {
	for _, r := range named {
		if r.name == name {
			return r, true
		}
	}
	return rule{}, false
}

func declIndex(name string) int {
	for i, r := range named {
		if r.name == name {
			return i
		}
	}
	return len(named) // customs sort last
}
