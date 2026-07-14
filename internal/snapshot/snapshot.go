// Package snapshot captures a deterministic picture of a directory tree:
// one sorted entry per path with kind, permissions, size, symlink target,
// and a SHA-256 content hash for regular files. Two snapshots taken around
// a command run are the raw material for effect diffing.
package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/JaydenCJ/idemproof/internal/glob"
)

// Kind classifies a filesystem entry.
type Kind string

const (
	KindFile    Kind = "file"
	KindDir     Kind = "dir"
	KindSymlink Kind = "symlink"
	KindOther   Kind = "other" // sockets, devices, fifos
)

// Entry is one recorded path inside a snapshot. Paths are slash-separated
// and relative to the snapshot root.
type Entry struct {
	Path    string
	Kind    Kind
	Mode    fs.FileMode // permission bits only
	Size    int64       // regular files only
	Hash    string      // sha256 hex of content; "" for non-files and oversized files
	Target  string      // symlink target
	ModTime time.Time   // recorded only when Options.Times is set
}

// Snapshot is the state of one watch root at a point in time.
type Snapshot struct {
	Root    string
	Entries []Entry // sorted by Path
	byPath  map[string]int
}

// Options controls what a snapshot records and compares.
type Options struct {
	// Ignore holds glob patterns (see internal/glob); a matching directory
	// prunes its whole subtree.
	Ignore []string
	// Times records modification times so the diff can flag mtime-only
	// changes (off by default: mtimes are noisy on most filesystems).
	Times bool
	// MaxFileSize caps content hashing. Files larger than this are recorded
	// with an empty hash and compared by size only. Zero means no cap.
	MaxFileSize int64
}

// Take walks root and returns its snapshot. The root itself is not an
// entry. Files that vanish mid-walk (a concurrent process cleaning up)
// are skipped rather than failing the whole proof.
func Take(root string, opts Options) (*Snapshot, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("watch path %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("watch path %q is not a directory", root)
	}
	snap := &Snapshot{Root: root, byPath: map[string]int{}}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return nil // vanished mid-walk
			}
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if glob.MatchAny(opts.Ignore, rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		entry, err := describe(path, rel, d, opts)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // vanished between readdir and lstat
			}
			return err
		}
		snap.Entries = append(snap.Entries, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(snap.Entries, func(i, j int) bool {
		return snap.Entries[i].Path < snap.Entries[j].Path
	})
	for i := range snap.Entries {
		snap.byPath[snap.Entries[i].Path] = i
	}
	return snap, nil
}

// Lookup returns the entry for a relative path, if present.
func (s *Snapshot) Lookup(rel string) (Entry, bool) {
	i, ok := s.byPath[rel]
	if !ok {
		return Entry{}, false
	}
	return s.Entries[i], true
}

// describe builds the Entry for a single walked path.
func describe(path, rel string, d fs.DirEntry, opts Options) (Entry, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Entry{}, err
	}
	e := Entry{
		Path: rel,
		Mode: info.Mode().Perm(),
	}
	if opts.Times {
		e.ModTime = info.ModTime().UTC()
	}
	switch {
	case info.Mode().IsDir():
		e.Kind = KindDir
	case info.Mode()&fs.ModeSymlink != 0:
		e.Kind = KindSymlink
		target, err := os.Readlink(path)
		if err != nil {
			return Entry{}, err
		}
		e.Target = target
	case info.Mode().IsRegular():
		e.Kind = KindFile
		e.Size = info.Size()
		if opts.MaxFileSize == 0 || info.Size() <= opts.MaxFileSize {
			hash, err := hashFile(path)
			if err != nil {
				return Entry{}, err
			}
			e.Hash = hash
		}
	default:
		e.Kind = KindOther
	}
	return e, nil
}

// hashFile returns the lowercase hex SHA-256 of the file content.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Label renders a watch root for display: the cleaned path, with "." kept
// as-is so reports stay stable regardless of the absolute working dir.
func Label(root string) string {
	clean := strings.TrimSuffix(filepath.ToSlash(filepath.Clean(root)), "/")
	if clean == "" {
		return "/"
	}
	return clean
}
