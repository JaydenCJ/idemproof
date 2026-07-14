// Tests for output normalization. Each built-in rule gets a realistic
// log line (the kind of volatile token that legitimately differs between
// two otherwise-identical runs) plus the negative cases that must NOT be
// scrubbed, since over-eager scrubbing would hide real drift.
package scrub

import (
	"strings"
	"testing"
)

func apply(t *testing.T, names []string, text string) string {
	t.Helper()
	s, err := Build(names, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s.Apply(text)
}

func TestTimestampsISO8601(t *testing.T) {
	in := "created at 2026-07-13T09:15:42Z and again 2026-07-13 09:15:42.123+02:00"
	got := apply(t, []string{"timestamps"}, in)
	want := "created at <TIMESTAMP> and again <TIMESTAMP>"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if bare := apply(t, []string{"timestamps"}, "backup-2026-07-13.tar.gz"); bare != "backup-<TIMESTAMP>.tar.gz" {
		t.Fatalf("bare date not scrubbed: %q", bare)
	}
	if clock := apply(t, []string{"times"}, "12:04:59 starting worker"); clock != "<TIME> starting worker" {
		t.Fatalf("bare clock time not scrubbed: %q", clock)
	}
}

func TestUUIDs(t *testing.T) {
	got := apply(t, []string{"uuids"}, "request 3f2b8a10-9c4d-4e2a-b1aa-0d9f6c1e7b22 done")
	if got != "request <UUID> done" {
		t.Fatalf("got %q", got)
	}
}

func TestHexDigestsButNotShortHex(t *testing.T) {
	got := apply(t, []string{"hex"}, "digest e3b0c44298fc1c149afbf4c8 at cafe")
	if got != "digest <HEX> at cafe" {
		t.Fatalf("short hex words must survive; got %q", got)
	}
}

func TestPIDsCaseInsensitive(t *testing.T) {
	got := apply(t, []string{"pids"}, "server started, PID=4821 (pid: 4821)")
	if got != "server started, PID <PID> (pid <PID>)" {
		t.Fatalf("got %q", got)
	}
}

func TestDurations(t *testing.T) {
	got := apply(t, []string{"durations"}, "done in 1.42s (io 350ms, cpu 90us)")
	if got != "done in <DURATION> (io <DURATION>, cpu <DURATION>)" {
		t.Fatalf("got %q", got)
	}
}

func TestDurationsDoNotEatIdentifiers(t *testing.T) {
	// "k8s" contains a digit+s but has no word boundary before the digit.
	got := apply(t, []string{"durations"}, "deploying to k8s cluster")
	if got != "deploying to k8s cluster" {
		t.Fatalf("identifier was wrongly scrubbed: %q", got)
	}
}

func TestTmpPaths(t *testing.T) {
	got := apply(t, []string{"tmppaths"}, "workdir /tmp/build-8f3k2/stage ready")
	if got != "workdir <TMPPATH> ready" {
		t.Fatalf("got %q", got)
	}
}

func TestAllAppliesEveryRuleInStableOrder(t *testing.T) {
	in := "2026-07-13T00:00:00Z pid=99 in 3ms at /tmp/x"
	got := apply(t, []string{"all"}, in)
	want := "<TIMESTAMP> pid <PID> in <DURATION> at <TMPPATH>"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	// Duplicates and mixed order must not change the outcome.
	again := apply(t, []string{"tmppaths", "all", "pids"}, in)
	if again != want {
		t.Fatalf("order-sensitivity detected: %q vs %q", again, want)
	}
	// Names() is the --help contract: sorted and covering every rule.
	names := Names()
	if len(names) != len(named) {
		t.Fatalf("Names() returned %d of %d rules", len(names), len(named))
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Fatalf("Names() not sorted: %q >= %q", names[i-1], names[i])
		}
	}
}

func TestCustomRegexpScrubs(t *testing.T) {
	s, err := Build(nil, []string{`session-[0-9]+`})
	if err != nil {
		t.Fatal(err)
	}
	got := s.Apply("joined session-42 ok")
	if got != "joined <SCRUBBED> ok" {
		t.Fatalf("got %q", got)
	}
	if _, err := Build(nil, []string{"("}); err == nil {
		t.Fatal("expected error for invalid regexp")
	}
}

func TestUnknownNormalizerIsAnError(t *testing.T) {
	_, err := Build([]string{"nonsense"}, nil)
	if err == nil {
		t.Fatal("expected error for unknown normalizer")
	}
	if !strings.Contains(err.Error(), "nonsense") {
		t.Fatalf("error should name the bad normalizer: %v", err)
	}
}

func TestEmptyScrubberIsIdentity(t *testing.T) {
	s, err := Build(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	in := "2026-07-13 pid=1 /tmp/x abc123"
	if got := s.Apply(in); got != in {
		t.Fatalf("empty scrubber must not touch text: %q", got)
	}
	if len(s.Active()) != 0 {
		t.Fatal("empty scrubber must report no active rules")
	}
}
