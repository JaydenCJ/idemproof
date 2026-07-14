// Tests for the pure diff engine. Snapshots are built from real temp
// trees (via the snapshot package) so the diff is exercised on exactly
// the entries production sees.
package fsdiff

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JaydenCJ/idemproof/internal/snapshot"
)

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	abs := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func take(t *testing.T, dir string, opts snapshot.Options) *snapshot.Snapshot {
	t.Helper()
	s, err := snapshot.Take(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// one asserts the diff has exactly one change and returns it.
func one(t *testing.T, changes []Change) Change {
	t.Helper()
	if len(changes) != 1 {
		t.Fatalf("expected exactly 1 change, got %d: %+v", len(changes), changes)
	}
	return changes[0]
}

func hasField(c Change, f string) bool {
	for _, x := range c.Fields {
		if x == f {
			return true
		}
	}
	return false
}

func TestNoChangesOnIdenticalTrees(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a/b.txt", "stable")
	before := take(t, dir, snapshot.Options{})
	after := take(t, dir, snapshot.Options{})
	if changes := Diff(before, after); len(changes) != 0 {
		t.Fatalf("expected no changes, got %+v", changes)
	}
}

func TestCreatedFile(t *testing.T) {
	dir := t.TempDir()
	before := take(t, dir, snapshot.Options{})
	write(t, dir, "new.txt", "hi")
	after := take(t, dir, snapshot.Options{})
	c := one(t, Diff(before, after))
	if c.Type != Created || c.Path != "new.txt" || c.Kind != "file" {
		t.Fatalf("unexpected change: %+v", c)
	}
}

func TestRemovedFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "gone.txt", "bye")
	before := take(t, dir, snapshot.Options{})
	if err := os.Remove(filepath.Join(dir, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	after := take(t, dir, snapshot.Options{})
	c := one(t, Diff(before, after))
	if c.Type != Removed || c.Path != "gone.txt" {
		t.Fatalf("unexpected change: %+v", c)
	}
}

func TestContentEditFlagsContentAndSize(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", "one")
	before := take(t, dir, snapshot.Options{})
	write(t, dir, "f", "one-more")
	after := take(t, dir, snapshot.Options{})
	c := one(t, Diff(before, after))
	if c.Type != Modified || !hasField(c, "content") || !hasField(c, "size") {
		t.Fatalf("expected content+size modification, got %+v", c)
	}
	if c.Detail != "3 B -> 8 B" {
		t.Fatalf("expected size detail, got %q", c.Detail)
	}
}

func TestSameSizeContentEditIsStillCaught(t *testing.T) {
	// A same-length rewrite is the classic case mtime-less, size-only
	// tools miss; the content hash must catch it.
	dir := t.TempDir()
	write(t, dir, "f", "aaaa")
	before := take(t, dir, snapshot.Options{})
	write(t, dir, "f", "bbbb")
	after := take(t, dir, snapshot.Options{})
	c := one(t, Diff(before, after))
	if !hasField(c, "content") || hasField(c, "size") {
		t.Fatalf("expected content-only modification, got %+v", c)
	}
}

func TestModeChange(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "run.sh", "#!/bin/sh\n")
	before := take(t, dir, snapshot.Options{})
	if err := os.Chmod(filepath.Join(dir, "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	after := take(t, dir, snapshot.Options{})
	c := one(t, Diff(before, after))
	if !hasField(c, "mode") {
		t.Fatalf("expected mode change, got %+v", c)
	}
	if c.Detail == "" {
		t.Fatal("mode change should carry an old -> new detail")
	}
}

func TestTypeChangeFileToDir(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "thing", "file first")
	before := take(t, dir, snapshot.Options{})
	if err := os.Remove(filepath.Join(dir, "thing")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "thing"), 0o755); err != nil {
		t.Fatal(err)
	}
	after := take(t, dir, snapshot.Options{})
	c := one(t, Diff(before, after))
	if !hasField(c, "type") || c.Detail != "file -> dir" {
		t.Fatalf("expected type change file -> dir, got %+v", c)
	}
}

func TestSymlinkRetarget(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "v1", "1")
	write(t, dir, "v2", "2")
	if err := os.Symlink("v1", filepath.Join(dir, "current")); err != nil {
		t.Skipf("symlinks unsupported here: %v", err)
	}
	before := take(t, dir, snapshot.Options{})
	if err := os.Remove(filepath.Join(dir, "current")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("v2", filepath.Join(dir, "current")); err != nil {
		t.Fatal(err)
	}
	after := take(t, dir, snapshot.Options{})
	c := one(t, Diff(before, after))
	if !hasField(c, "target") || c.Detail != "v1 -> v2" {
		t.Fatalf("expected symlink retarget, got %+v", c)
	}
}

func TestMtimeOnlyChangeRequiresTimesOption(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", "same")
	past := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(dir, "f"), past, past); err != nil {
		t.Fatal(err)
	}
	// Default snapshots: touching the mtime is invisible.
	before := take(t, dir, snapshot.Options{})
	later := past.Add(time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "f"), later, later); err != nil {
		t.Fatal(err)
	}
	after := take(t, dir, snapshot.Options{})
	if changes := Diff(before, after); len(changes) != 0 {
		t.Fatalf("mtime-only change must be ignored by default, got %+v", changes)
	}
	// Strict snapshots: the same touch is an effect.
	if err := os.Chtimes(filepath.Join(dir, "f"), past, past); err != nil {
		t.Fatal(err)
	}
	strictBefore := take(t, dir, snapshot.Options{Times: true})
	if err := os.Chtimes(filepath.Join(dir, "f"), later, later); err != nil {
		t.Fatal(err)
	}
	strictAfter := take(t, dir, snapshot.Options{Times: true})
	c := one(t, Diff(strictBefore, strictAfter))
	if !hasField(c, "mtime") {
		t.Fatalf("expected mtime change under Times option, got %+v", c)
	}
}

func TestOversizedFilesFallBackToSizeComparison(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "big", "0123456789")
	opts := snapshot.Options{MaxFileSize: 5}
	before := take(t, dir, opts)
	write(t, dir, "big", "01234567890123456789")
	after := take(t, dir, opts)
	c := one(t, Diff(before, after))
	if !hasField(c, "size") {
		t.Fatalf("expected size change on oversized file, got %+v", c)
	}
}

func TestChangesAreSortedByPath(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "m.txt", "m")
	before := take(t, dir, snapshot.Options{})
	write(t, dir, "z.txt", "z")
	write(t, dir, "a.txt", "a")
	if err := os.Remove(filepath.Join(dir, "m.txt")); err != nil {
		t.Fatal(err)
	}
	after := take(t, dir, snapshot.Options{})
	changes := Diff(before, after)
	if len(changes) != 3 {
		t.Fatalf("expected 3 changes, got %d", len(changes))
	}
	for i := 1; i < len(changes); i++ {
		if changes[i-1].Path >= changes[i].Path {
			t.Fatalf("changes not sorted: %q >= %q", changes[i-1].Path, changes[i].Path)
		}
	}
}

func TestHumanSizeRendering(t *testing.T) {
	cases := map[int64]string{
		0:               "0 B",
		512:             "512 B",
		2048:            "2.0 KiB",
		3 * 1024 * 1024: "3.0 MiB",
		5 << 30:         "5.0 GiB",
		1536:            "1.5 KiB",
	}
	for in, want := range cases {
		if got := humanSize(in); got != want {
			t.Fatalf("humanSize(%d) = %q, want %q", in, got, want)
		}
	}
}
