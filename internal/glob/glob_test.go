// Tests for the ignore-glob dialect. Each case documents the exact
// contract users rely on when writing --ignore patterns, especially the
// gitignore-style bare-name behavior and cross-segment '**'.
package glob

import "testing"

func TestStarMatchesWithinSegment(t *testing.T) {
	if !Match("*.log", "server.log") {
		t.Fatal("*.log should match server.log")
	}
	if Match("src/*.log", "src/deep/server.log") {
		t.Fatal("single * must not cross a slash")
	}
}

func TestQuestionMarkMatchesExactlyOneChar(t *testing.T) {
	if !Match("file?.txt", "file1.txt") {
		t.Fatal("? should match one character")
	}
	if Match("file?.txt", "file10.txt") {
		t.Fatal("? must not match two characters")
	}
	if Match("file?.txt", "file.txt") {
		t.Fatal("? must not match zero characters")
	}
}

func TestDoubleStarSpansSegments(t *testing.T) {
	for _, path := range []string{"vendor/a.go", "vendor/x/y/z/a.go"} {
		if !Match("vendor/**", path) {
			t.Fatalf("vendor/** should match %q", path)
		}
	}
	// "dir/**" also matches the bare directory ("** may match zero
	// segments"), which lets the snapshot walker prune the whole subtree
	// in one check instead of visiting every child first.
	if !Match("vendor/**", "vendor") {
		t.Fatal("dir/** should match bare dir (zero-segment **)")
	}
}

func TestDoubleStarMatchesZeroSegments(t *testing.T) {
	// "**/target" must also match a top-level "target" — the zero-segment
	// case is the one hand-rolled matchers usually get wrong.
	if !Match("**/target", "target") {
		t.Fatal("** should be allowed to match zero segments")
	}
	if !Match("**/target", "a/b/target") {
		t.Fatal("**/target should match nested target")
	}
}

func TestDoubleStarInTheMiddle(t *testing.T) {
	if !Match("src/**/testdata", "src/pkg/sub/testdata") {
		t.Fatal("mid-pattern ** should span segments")
	}
	if Match("src/**/testdata", "lib/pkg/testdata") {
		t.Fatal("anchored prefix must still be honored")
	}
}

func TestBareNameMatchesAnySegment(t *testing.T) {
	// gitignore-style: a pattern without '/' applies at any depth.
	if !Match("node_modules", "web/node_modules/pkg") {
		t.Fatal("bare name should match a middle segment")
	}
	if !Match("*.tmp", "deep/nested/x.tmp") {
		t.Fatal("bare wildcard should match the basename at any depth")
	}
}

func TestSlashedPatternIsAnchored(t *testing.T) {
	if Match("build/out", "x/build/out") {
		t.Fatal("a slashed pattern is anchored to the root")
	}
	if !Match("build/out", "build/out") {
		t.Fatal("anchored exact path should match")
	}
}

func TestEdgeCases(t *testing.T) {
	if Match("", "anything") {
		t.Fatal("empty pattern must match nothing")
	}
	if Match("a.c", "abc") {
		t.Fatal("'.' must be literal, not regex-any")
	}
}

func TestBacktrackingStarStress(t *testing.T) {
	// The iterative matcher must survive adversarial star patterns that
	// would blow up naive recursive implementations.
	if !Match("a*a*a*a*a*b", "aaaaaaaaaaaaaaaaaaaab") {
		t.Fatal("expected match after heavy backtracking")
	}
	if Match("a*a*a*a*a*b", "aaaaaaaaaaaaaaaaaaaac") {
		t.Fatal("expected mismatch after heavy backtracking")
	}
}

func TestMatchAny(t *testing.T) {
	patterns := []string{"*.log", ".git/**"}
	if !MatchAny(patterns, ".git/objects/ab") {
		t.Fatal("MatchAny should hit the second pattern")
	}
	if MatchAny(patterns, "main.go") {
		t.Fatal("MatchAny should miss on main.go")
	}
	if MatchAny(nil, "main.go") {
		t.Fatal("no patterns should match nothing")
	}
}
