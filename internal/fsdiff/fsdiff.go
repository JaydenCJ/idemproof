// Package fsdiff computes the effect set between two snapshots of the
// same root: which paths a run created, removed, or modified, and — for
// modifications — exactly which attributes changed. The diff is pure,
// deterministic, and sorted, so identical inputs always render the same
// report byte for byte.
package fsdiff

import (
	"fmt"
	"sort"

	"github.com/JaydenCJ/idemproof/internal/snapshot"
)

// ChangeType classifies one effect.
type ChangeType string

const (
	Created  ChangeType = "created"
	Removed  ChangeType = "removed"
	Modified ChangeType = "modified"
)

// Change is a single filesystem effect between two snapshots.
type Change struct {
	Type   ChangeType `json:"type"`
	Path   string     `json:"path"`
	Kind   string     `json:"kind"`
	Fields []string   `json:"fields,omitempty"` // modified only: content, size, mode, type, target, mtime
	Detail string     `json:"detail,omitempty"` // human hint, e.g. "4 B -> 8 B"
}

// Diff returns all effects from before to after, sorted by path with a
// stable created < removed ordering for unrelated paths at equal names
// (impossible in practice, but keeps the comparator total).
func Diff(before, after *snapshot.Snapshot) []Change {
	var changes []Change
	for _, e := range after.Entries {
		prev, ok := before.Lookup(e.Path)
		if !ok {
			changes = append(changes, Change{Type: Created, Path: e.Path, Kind: string(e.Kind)})
			continue
		}
		if c, changed := compare(prev, e); changed {
			changes = append(changes, c)
		}
	}
	for _, e := range before.Entries {
		if _, ok := after.Lookup(e.Path); !ok {
			changes = append(changes, Change{Type: Removed, Path: e.Path, Kind: string(e.Kind)})
		}
	}
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Path != changes[j].Path {
			return changes[i].Path < changes[j].Path
		}
		return changes[i].Type < changes[j].Type
	})
	return changes
}

// compare inspects one path present in both snapshots and reports what
// changed, if anything.
func compare(prev, cur snapshot.Entry) (Change, bool) {
	c := Change{Type: Modified, Path: cur.Path, Kind: string(cur.Kind)}
	if prev.Kind != cur.Kind {
		c.Fields = []string{"type"}
		c.Detail = fmt.Sprintf("%s -> %s", prev.Kind, cur.Kind)
		return c, true
	}
	var details []string
	if cur.Kind == snapshot.KindFile {
		// Hash is authoritative when both sides have one; oversized files
		// (empty hash) fall back to size comparison alone.
		if prev.Hash != cur.Hash && (prev.Hash != "" || cur.Hash != "") {
			c.Fields = append(c.Fields, "content")
		}
		if prev.Size != cur.Size {
			c.Fields = append(c.Fields, "size")
			details = append(details, fmt.Sprintf("%s -> %s", humanSize(prev.Size), humanSize(cur.Size)))
		}
	}
	if cur.Kind == snapshot.KindSymlink && prev.Target != cur.Target {
		c.Fields = append(c.Fields, "target")
		details = append(details, fmt.Sprintf("%s -> %s", prev.Target, cur.Target))
	}
	if prev.Mode != cur.Mode {
		c.Fields = append(c.Fields, "mode")
		details = append(details, fmt.Sprintf("%s -> %s", prev.Mode, cur.Mode))
	}
	if !prev.ModTime.IsZero() && !cur.ModTime.IsZero() && !prev.ModTime.Equal(cur.ModTime) && len(c.Fields) == 0 {
		// mtime is only worth reporting when nothing substantive changed;
		// content edits already imply a touched mtime.
		c.Fields = append(c.Fields, "mtime")
	}
	if len(c.Fields) == 0 {
		return Change{}, false
	}
	if len(details) > 0 {
		c.Detail = details[0]
	}
	return c, true
}

// humanSize renders a byte count compactly (B, KiB, MiB, GiB).
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMG"[exp])
}
