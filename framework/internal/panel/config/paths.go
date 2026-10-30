package config

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/rfhayn/stridal/internal/brand"
)

// PanelDir is ~/.clauductor/panel: the machine's panel files (port, pid, owner,
// token, lock, logs) and one directory per project (ProjectDir). A legacy name's
// directory is read while only that one exists (brand.Names).
func PanelDir(home string) string { return panelDir(home, brand.Names()) }

func panelDir(home string, names []string) string {
	ps := make([]string, len(names))
	for i, n := range names {
		ps[i] = filepath.Join(home, brand.Dir(n), "panel")
	}
	return brand.Existing(ps...)
}

// SidecarPath is the config `panel attach` writes for a project, outside its
// repository (PANEL-42, design.md D2): ~/.clauductor/projects/<id>/panel.json. A
// readable folder per id, not ProjectDir's hash, because the owner edits it by hand.
// The id must already match ProjectIDRe: it is a path segment here. A legacy name's
// sidecar is read while only that one exists.
func SidecarPath(home, id string) string { return sidecarPath(home, id, brand.Names()) }

func sidecarPath(home, id string, names []string) string {
	ps := make([]string, len(names))
	for i, n := range names {
		ps[i] = filepath.Join(home, brand.Dir(n), "projects", id, "panel.json")
	}
	return brand.Existing(ps...)
}

// ResolveExisting is p with symlinks evaluated as far as it exists, component by
// component as the kernel does: a symlink is followed before a later `..` applies, so
// `link/..` is the link target's parent, not the link's (PANEL-42, design.md D5). From
// the first component that does not exist, the rest is joined on lexically: a path not
// there yet is where it will be written, through any symlinked folder above it. A `..`
// that climbs back out of the missing part, to the folder that exists, resolves through
// symlinks again: `mkdir -p x/../link/..` makes x, and the kernel then follows link. A
// relative p is taken from the working directory. Never Clean p first: cleaning
// applies `..` lexically, before the symlink under it is seen.
func ResolveExisting(p string) string {
	sep := string(filepath.Separator)
	if !filepath.IsAbs(p) {
		if wd, err := os.Getwd(); err == nil {
			p = wd + sep + p
		}
	}
	vol := filepath.VolumeName(p)
	resolved := vol + sep
	pending := strings.Split(p[len(vol):], sep)
	// missing counts the components joined lexically past the last one that exists,
	// so a `..` that brings it to 0 is back on a real, symlink-free folder.
	missing, links := 0, 0
	for len(pending) > 0 {
		c := pending[0]
		pending = pending[1:]
		switch c {
		case "", ".":
			continue
		case "..":
			// resolved's existing part holds no symlink (each was followed), so its
			// parent is the real one.
			resolved = filepath.Dir(resolved)
			if missing > 0 {
				missing--
			}
			continue
		}
		next := filepath.Join(resolved, c)
		if missing > 0 {
			missing++
			resolved = next
			continue
		}
		fi, err := os.Lstat(next)
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			if err != nil {
				missing = 1
			}
			resolved = next
			continue
		}
		target, err := os.Readlink(next)
		if links++; err != nil || links > 255 {
			// A loop (the kernel's ELOOP) or an unreadable link: nothing past it opens.
			// The link itself is the first missing component, so a `..` back over it
			// leaves its (real) parent.
			missing, resolved = 1, next
			continue
		}
		if filepath.IsAbs(target) {
			tv := filepath.VolumeName(target)
			resolved, target = tv+sep, target[len(tv):]
		}
		pending = append(strings.Split(target, sep), pending...)
	}
	return resolved
}

// Within: p is root or inside it. Both are resolved.
func Within(p, root string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// ProjectDir holds one project's panel files: the lane registry, trusted config
// hashes, the notifier's state and queue logs. The hash keeps projects apart without
// putting a path in a file name.
func ProjectDir(home, project string) string {
	sum := sha256.Sum256([]byte(project))
	return filepath.Join(PanelDir(home), hex.EncodeToString(sum[:8]))
}

// EnsurePrivateDir makes dir (0700), and tightens it if it already exists looser.
func EnsurePrivateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700)
}

// WriteAtomic writes data to path through a temp file and a rename, so a reader
// sees the old file or the new one, never half of one.
func WriteAtomic(path string, data []byte, mode os.FileMode) error {
	return WriteAtomicChecked(path, data, mode, nil)
}

// WriteAtomicChecked writes a temp file, then runs check (if set) right before the
// rename; a check error abandons the write.
func WriteAtomicChecked(path string, data []byte, mode os.FileMode, check func() error) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".settings.json.tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, mode); err != nil {
		return err
	}
	if check != nil {
		if err := check(); err != nil {
			return err
		}
	}
	return os.Rename(name, path)
}
