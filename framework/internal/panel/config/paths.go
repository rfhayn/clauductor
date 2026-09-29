package config

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// PanelDir is ~/.clauductor/panel: the machine's panel files (port, pid, owner,
// token, lock, logs) and one directory per project (ProjectDir).
func PanelDir(home string) string { return filepath.Join(home, ".clauductor", "panel") }

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
