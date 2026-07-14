// Tests for the snapshot walker: entry kinds, hashing, ignore pruning,
// deterministic ordering, and the options that control comparison
// granularity. Everything runs in t.TempDir() — no fixtures on disk.
package snapshot

import (
	"os"
	"path/filepath"
	"testing"
)

// write creates rel under dir with content, making parents as needed.
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

func take(t *testing.T, dir string, opts Options) *Snapshot {
	t.Helper()
	s, err := Take(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRecordsFilesDirsAndNesting(t *testing.T) {
	if empty := take(t, t.TempDir(), Options{}); len(empty.Entries) != 0 {
		t.Fatalf("empty dir should yield 0 entries, got %d", len(empty.Entries))
	}
	dir := t.TempDir()
	write(t, dir, "a.txt", "hello")
	write(t, dir, "sub/b.txt", "world")
	s := take(t, dir, Options{})
	want := []string{"a.txt", "sub", "sub/b.txt"}
	if len(s.Entries) != len(want) {
		t.Fatalf("expected %d entries, got %d", len(want), len(s.Entries))
	}
	for i, p := range want {
		if s.Entries[i].Path != p {
			t.Fatalf("entry %d: expected %q, got %q", i, p, s.Entries[i].Path)
		}
	}
	if s.Entries[1].Kind != KindDir {
		t.Fatalf("sub should be a dir, got %s", s.Entries[1].Kind)
	}
}

func TestEntriesAreSortedRegardlessOfCreationOrder(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"zz.txt", "aa.txt", "mm.txt"} {
		write(t, dir, name, name)
	}
	s := take(t, dir, Options{})
	for i := 1; i < len(s.Entries); i++ {
		if s.Entries[i-1].Path >= s.Entries[i].Path {
			t.Fatalf("entries not sorted: %q >= %q", s.Entries[i-1].Path, s.Entries[i].Path)
		}
	}
}

func TestFileHashIsContentSHA256(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", "abc")
	s := take(t, dir, Options{})
	e, ok := s.Lookup("f")
	if !ok {
		t.Fatal("f not found")
	}
	// sha256("abc") — pinned so a hash-algorithm change cannot slip in
	// silently and invalidate stored comparisons.
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if e.Hash != want {
		t.Fatalf("hash mismatch: got %s", e.Hash)
	}
	if e.Size != 3 {
		t.Fatalf("size mismatch: got %d", e.Size)
	}
	// And the equivalence property that the diff engine builds on:
	dir = t.TempDir()
	write(t, dir, "a", "same")
	write(t, dir, "b", "same")
	write(t, dir, "c", "different")
	s = take(t, dir, Options{})
	a, _ := s.Lookup("a")
	b, _ := s.Lookup("b")
	c, _ := s.Lookup("c")
	if a.Hash != b.Hash {
		t.Fatal("identical content must hash identically")
	}
	if a.Hash == c.Hash {
		t.Fatal("different content must hash differently")
	}
}

func TestSymlinkRecordsTargetNotContent(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "real.txt", "data")
	if err := os.Symlink("real.txt", filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks unsupported here: %v", err)
	}
	s := take(t, dir, Options{})
	e, ok := s.Lookup("link")
	if !ok {
		t.Fatal("link not found")
	}
	if e.Kind != KindSymlink {
		t.Fatalf("expected symlink kind, got %s", e.Kind)
	}
	if e.Target != "real.txt" {
		t.Fatalf("expected target real.txt, got %q", e.Target)
	}
	if e.Hash != "" {
		t.Fatal("symlinks must not be content-hashed")
	}
}

func TestIgnorePrunesWholeSubtree(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "keep.txt", "k")
	write(t, dir, ".git/objects/ab/cd", "blob")
	write(t, dir, ".git/HEAD", "ref")
	s := take(t, dir, Options{Ignore: []string{".git"}})
	if len(s.Entries) != 1 || s.Entries[0].Path != "keep.txt" {
		t.Fatalf("expected only keep.txt, got %+v", s.Entries)
	}
}

func TestIgnoreGlobMatchesBasenameAtAnyDepth(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "app.log", "x")
	write(t, dir, "sub/deep/app.log", "y")
	write(t, dir, "sub/app.go", "z")
	s := take(t, dir, Options{Ignore: []string{"*.log"}})
	for _, e := range s.Entries {
		if filepath.Ext(e.Path) == ".log" {
			t.Fatalf("log file leaked through ignore: %s", e.Path)
		}
	}
	if _, ok := s.Lookup("sub/app.go"); !ok {
		t.Fatal("non-matching file was wrongly ignored")
	}
}

func TestModTimeRecordedOnlyWithTimesOption(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", "x")
	plain := take(t, dir, Options{})
	timed := take(t, dir, Options{Times: true})
	pe, _ := plain.Lookup("f")
	te, _ := timed.Lookup("f")
	if !pe.ModTime.IsZero() {
		t.Fatal("mtime must not be recorded by default")
	}
	if te.ModTime.IsZero() {
		t.Fatal("mtime must be recorded with Times: true")
	}
}

func TestMaxFileSizeSkipsHashingButKeepsSize(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "big", "0123456789")
	write(t, dir, "small", "ok")
	s := take(t, dir, Options{MaxFileSize: 5})
	big, _ := s.Lookup("big")
	small, _ := s.Lookup("small")
	if big.Hash != "" {
		t.Fatal("oversized file must not be hashed")
	}
	if big.Size != 10 {
		t.Fatal("oversized file must still record its size")
	}
	if small.Hash == "" {
		t.Fatal("small file must still be hashed")
	}
}

func TestWatchPathMustBeADirectory(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f", "x")
	if _, err := Take(filepath.Join(dir, "f"), Options{}); err == nil {
		t.Fatal("expected error for non-directory watch path")
	}
	if _, err := Take(filepath.Join(dir, "missing"), Options{}); err == nil {
		t.Fatal("expected error for missing watch path")
	}
}

func TestModeRecordsPermissionBits(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "script.sh", "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(dir, "script.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := take(t, dir, Options{})
	e, _ := s.Lookup("script.sh")
	if e.Mode.Perm() != 0o755 {
		t.Fatalf("expected mode 0755, got %v", e.Mode)
	}
}

func TestLabelNormalizesRoots(t *testing.T) {
	cases := map[string]string{
		".":      ".",
		"./sub/": "sub",
		"/":      "/",
		"a//b":   "a/b",
	}
	for in, want := range cases {
		if got := Label(in); got != want {
			t.Fatalf("Label(%q) = %q, want %q", in, got, want)
		}
	}
}
