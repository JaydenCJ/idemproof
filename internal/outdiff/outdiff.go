// Package outdiff compares two captured output streams line by line and
// pinpoints the first divergence, so a failed proof can show exactly where
// run N stopped matching run N-1 instead of dumping both streams.
package outdiff

import "strings"

// Result describes the comparison of stream A (earlier run) against
// stream B (later run).
type Result struct {
	Identical     bool   `json:"identical"`
	FirstDiffLine int    `json:"first_diff_line,omitempty"` // 1-based; 0 when identical
	ALine         string `json:"a_line,omitempty"`          // the earlier run's line ("" if absent)
	BLine         string `json:"b_line,omitempty"`          // the later run's line ("" if absent)
	ALines        int    `json:"a_lines"`
	BLines        int    `json:"b_lines"`
	Note          string `json:"note,omitempty"` // extra context, e.g. trailing-newline drift
}

// Compare diffs a against b. Comparison is exact — byte equality decides —
// but the located divergence is reported in line terms for humans. A
// missing trailing newline counts as a difference (many generators are
// sloppy about it, and byte-honesty is the point of the tool).
func Compare(a, b string) Result {
	al, bl := splitLines(a), splitLines(b)
	res := Result{ALines: len(al), BLines: len(bl)}
	if a == b {
		res.Identical = true
		return res
	}
	limit := len(al)
	if len(bl) < limit {
		limit = len(bl)
	}
	for i := 0; i < limit; i++ {
		if al[i] != bl[i] {
			res.FirstDiffLine = i + 1
			res.ALine, res.BLine = al[i], bl[i]
			return res
		}
	}
	// One stream is a prefix of the other (or they differ only in the
	// final newline).
	res.FirstDiffLine = limit + 1
	if limit < len(al) {
		res.ALine = al[limit]
	}
	if limit < len(bl) {
		res.BLine = bl[limit]
	}
	if res.ALine == "" && res.BLine == "" {
		// Only the trailing newline differs; point at the last real line.
		res.Note = "streams differ only in the trailing newline"
		if limit > 0 {
			res.FirstDiffLine = limit
			res.ALine, res.BLine = al[limit-1], bl[limit-1]
		} else {
			res.FirstDiffLine = 1
		}
	}
	return res
}

// splitLines splits text into lines without a phantom trailing element for
// a final newline. "a\nb\n" -> ["a","b"]; "a\nb" -> ["a","b"]; "" -> [].
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}
