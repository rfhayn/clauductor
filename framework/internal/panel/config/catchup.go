package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Catch-up's switch (OPS-118 D2): the machine's catch-up.json, {"roots": ["/abs/root",
// ...]}, the repository roots whose main checkout the panel fast-forwards after a merge.
// A root it does not list, or no file at all, is off. It is a file of its own, not a
// field of projects.json, because a binary that predates catch-up refuses a registry
// with a field it does not know (projects.json is decoded strictly), and never a
// panel.json key, because a repository must not grant itself writes to the owner's
// checkout. It is keyed by root, not by project id: an id is a slug of the config's
// name and can be taken again after a removal.

// CatchUp is catch-up.json.
type CatchUp struct {
	Roots []string `json:"roots"`
}

// CatchUpPath is catch-up.json's path: the machine's panel directory, beside
// projects.json.
func CatchUpPath(home string) string { return filepath.Join(PanelDir(home), "catch-up.json") }

// LoadCatchUp reads catch-up.json: no roots when the file is absent, and an error when
// it cannot be read or is not one object with a list of roots (a field it does not
// know included), which every reader takes as off.
func LoadCatchUp(home string) (*CatchUp, error) {
	path := CatchUpPath(home)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &CatchUp{}, nil
	}
	if err != nil {
		return nil, err
	}
	var c CatchUp
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	if dec.More() {
		return nil, fmt.Errorf("%s: trailing data after the object", path)
	}
	for _, r := range c.Roots {
		if !filepath.IsAbs(r) {
			return nil, fmt.Errorf("%s: root %q is not an absolute path", path, r)
		}
	}
	return &c, nil
}

// On reports whether root has catch-up on.
func (c *CatchUp) On(root string) bool {
	root = filepath.Clean(root)
	for _, r := range c.Roots {
		if filepath.Clean(r) == root {
			return true
		}
	}
	return false
}

// CatchUpOn reports whether root has catch-up on; an unreadable file is an error, which
// a caller takes as off.
func CatchUpOn(home, root string) (bool, error) {
	c, err := LoadCatchUp(home)
	if err != nil {
		return false, err
	}
	return c.On(root), nil
}

// SetCatchUp turns catch-up on or off for root, writing catch-up.json atomically
// (0600, in the private panel directory). Nothing is written when root is already as
// asked, so turning off a root never listed creates no file.
func SetCatchUp(home, root string, on bool) error {
	c, err := LoadCatchUp(home)
	if err != nil {
		return err
	}
	root = filepath.Clean(root)
	if !filepath.IsAbs(root) {
		return fmt.Errorf("catch-up: root %q is not an absolute path", root)
	}
	if c.On(root) == on {
		return nil
	}
	kept := []string{}
	for _, r := range c.Roots {
		if filepath.Clean(r) != root {
			kept = append(kept, r)
		}
	}
	if on {
		kept = append(kept, root)
	}
	c.Roots = kept
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := EnsurePrivateDir(PanelDir(home)); err != nil {
		return err
	}
	return WriteAtomic(CatchUpPath(home), append(b, '\n'), 0o600)
}
