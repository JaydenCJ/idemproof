// Package glob implements the small, predictable glob dialect used by
// --ignore patterns: '*' matches within a path segment, '?' matches a
// single character, and '**' matches any number of whole segments
// (including zero). Patterns and paths are slash-separated and relative.
package glob

import "strings"

// Match reports whether path matches pattern.
//
// A pattern that contains no '/' is matched against every individual
// segment of the path (gitignore-style), so "*.log" ignores log files at
// any depth. A pattern with '/' is anchored to the path root.
func Match(pattern, path string) bool {
	if pattern == "" {
		return false
	}
	if !strings.Contains(pattern, "/") {
		for _, seg := range strings.Split(path, "/") {
			if matchSegment(pattern, seg) {
				return true
			}
		}
		return false
	}
	return matchSegments(strings.Split(pattern, "/"), strings.Split(path, "/"))
}

// MatchAny reports whether any of the patterns matches path.
func MatchAny(patterns []string, path string) bool {
	for _, p := range patterns {
		if Match(p, path) {
			return true
		}
	}
	return false
}

// matchSegments matches a slash-split pattern against a slash-split path,
// giving '**' its cross-segment meaning.
func matchSegments(pat, segs []string) bool {
	if len(pat) == 0 {
		return len(segs) == 0
	}
	if pat[0] == "**" {
		// '**' absorbs zero or more leading segments.
		for skip := 0; skip <= len(segs); skip++ {
			if matchSegments(pat[1:], segs[skip:]) {
				return true
			}
		}
		return false
	}
	if len(segs) == 0 {
		return false
	}
	if !matchSegment(pat[0], segs[0]) {
		return false
	}
	return matchSegments(pat[1:], segs[1:])
}

// matchSegment matches a single pattern segment ('*' and '?' wildcards)
// against a single path segment, iteratively with backtracking so
// pathological patterns cannot blow the stack.
func matchSegment(pat, seg string) bool {
	pi, si := 0, 0
	starPi, starSi := -1, 0
	for si < len(seg) {
		switch {
		case pi < len(pat) && (pat[pi] == '?' || pat[pi] == seg[si]):
			pi++
			si++
		case pi < len(pat) && pat[pi] == '*':
			starPi, starSi = pi, si
			pi++
		case starPi >= 0:
			// Backtrack: let the last '*' swallow one more character.
			starSi++
			pi, si = starPi+1, starSi
		default:
			return false
		}
	}
	for pi < len(pat) && pat[pi] == '*' {
		pi++
	}
	return pi == len(pat)
}
