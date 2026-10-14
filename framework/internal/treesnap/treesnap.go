// Package treesnap is test support: it records every file and directory under a root,
// .git included, so a test can prove an operation wrote nothing there (PANEL-42,
// design.md D8: attaching and serving a repository leave it untouched). Only tests
// import it.
//
// `git status --porcelain --ignored` is not enough for that: it does not see .git
// itself, and a file rewritten with the same bytes looks unchanged to it. A snapshot
// holds each entry's mode, size, modification time and SHA-256, so a rewrite with the
// same bytes still shows (its mtime), and so does a lock file git created and removed
// (its directory's mtime).
package treesnap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// Entry is one file, directory or symlink.
type Entry struct {
	Mode    fs.FileMode
	Size    int64
	ModTime int64  // nanoseconds
	SHA256  string // a file's bytes, or a symlink's target; "" for a directory
}

// Snapshot is every entry under a root, by its slash-separated path from the root
// ("." is the root itself).
type Snapshot map[string]Entry

// Take records every entry under root, failing the test when one cannot be read.
func Take(t testing.TB, root string) Snapshot {
	t.Helper()
	s, err := TakeErr(root)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TakeErr is Take for a caller with no test to fail.
func TakeErr(root string) (Snapshot, error) {
	s := Snapshot{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := os.Lstat(path) // a symlink is recorded, never followed
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		e := Entry{Mode: fi.Mode(), Size: fi.Size(), ModTime: fi.ModTime().UnixNano()}
		switch {
		case fi.Mode()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			e.SHA256 = sum([]byte(target))
		case fi.Mode().IsRegular():
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			e.SHA256 = sum(b)
		}
		s[filepath.ToSlash(rel)] = e
		return nil
	})
	return s, err
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Diff names every entry that differs between before and after, one line each,
// sorted: added, removed, or changed with what changed. Empty when they are the same.
func Diff(before, after Snapshot) []string {
	var out []string
	for p, a := range after {
		b, ok := before[p]
		switch {
		case !ok:
			out = append(out, "added "+p)
		case a != b:
			out = append(out, fmt.Sprintf("changed %s: %s", p, what(b, a)))
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			out = append(out, "removed "+p)
		}
	}
	sort.Strings(out)
	return out
}

func what(b, a Entry) string {
	var parts []string
	if a.Mode != b.Mode {
		parts = append(parts, fmt.Sprintf("mode %v → %v", b.Mode, a.Mode))
	}
	if a.Size != b.Size {
		parts = append(parts, fmt.Sprintf("size %d → %d", b.Size, a.Size))
	}
	if a.ModTime != b.ModTime {
		parts = append(parts, "mtime")
	}
	if a.SHA256 != b.SHA256 {
		parts = append(parts, "content")
	}
	return fmt.Sprint(parts)
}
