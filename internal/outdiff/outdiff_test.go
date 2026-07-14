// Tests for stream comparison: byte-honest equality plus a precise,
// human-oriented first-divergence pointer.
package outdiff

import "testing"

func TestIdenticalStreams(t *testing.T) {
	r := Compare("a\nb\n", "a\nb\n")
	if !r.Identical || r.FirstDiffLine != 0 {
		t.Fatalf("expected identical, got %+v", r)
	}
	if r.ALines != 2 || r.BLines != 2 {
		t.Fatalf("expected 2 lines each, got %+v", r)
	}
	if e := Compare("", ""); !e.Identical || e.ALines != 0 || e.BLines != 0 {
		t.Fatalf("expected identical empties, got %+v", e)
	}
}

func TestFirstDivergenceIsLocated(t *testing.T) {
	r := Compare("same\nold value\ntail\n", "same\nnew value\ntail\n")
	if r.Identical {
		t.Fatal("expected difference")
	}
	if r.FirstDiffLine != 2 {
		t.Fatalf("expected diff at line 2, got %d", r.FirstDiffLine)
	}
	if r.ALine != "old value" || r.BLine != "new value" {
		t.Fatalf("wrong lines captured: %+v", r)
	}
}

func TestLongerStreamBIsAdditionalLine(t *testing.T) {
	r := Compare("a\n", "a\nextra\n")
	if r.Identical || r.FirstDiffLine != 2 {
		t.Fatalf("expected diff at line 2, got %+v", r)
	}
	if r.ALine != "" || r.BLine != "extra" {
		t.Fatalf("expected missing-vs-extra, got %+v", r)
	}
}

func TestShorterStreamBIsMissingLine(t *testing.T) {
	r := Compare("a\nb\n", "a\n")
	if r.Identical || r.FirstDiffLine != 2 {
		t.Fatalf("expected diff at line 2, got %+v", r)
	}
	if r.ALine != "b" || r.BLine != "" {
		t.Fatalf("expected b-vs-missing, got %+v", r)
	}
}

func TestEmptyVersusNonEmpty(t *testing.T) {
	r := Compare("", "surprise\n")
	if r.Identical || r.FirstDiffLine != 1 || r.BLine != "surprise" {
		t.Fatalf("expected diff at line 1, got %+v", r)
	}
}

func TestTrailingNewlineOnlyDrift(t *testing.T) {
	// Byte-honesty: "done" vs "done\n" is a real difference, but the
	// report must explain it instead of showing two identical lines.
	r := Compare("done", "done\n")
	if r.Identical {
		t.Fatal("trailing-newline drift must not be identical")
	}
	if r.Note == "" {
		t.Fatal("expected an explanatory note for newline-only drift")
	}
	if r.FirstDiffLine != 1 {
		t.Fatalf("expected pointer at line 1, got %d", r.FirstDiffLine)
	}
	// Line counts stay newline-agnostic even when bytes differ.
	if c := Compare("a\nb", "a\nb\n"); c.ALines != 2 || c.BLines != 2 {
		t.Fatalf("final newline must not add a phantom line: %+v", c)
	}
}

func TestCRLFIsNotSilentlyEqualToLF(t *testing.T) {
	// Windows-style output from run 2 is a genuine behavioral change.
	r := Compare("a\n", "a\r\n")
	if r.Identical {
		t.Fatal("CRLF vs LF must be reported")
	}
	if r.FirstDiffLine != 1 {
		t.Fatalf("expected diff at line 1, got %d", r.FirstDiffLine)
	}
}
