package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// catch-up.json records catch-up per root: a root it lists is on, one it does not list
// is off, as is every root while the file is absent; turning a root off keeps the
// others; the file is written private, and turning off a root never listed writes no
// file. A test of the part cites no scenario: the CLI test TestPanelCatchUpPerProject
// drives the per-project switch through `stridal panel catch-up`.
func TestCatchUpRecordedPerRoot(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	a, b := "/repos/a", "/repos/b"

	if on, err := CatchUpOn(home, a); err != nil || on {
		t.Fatalf("no file: on=%v err=%v, want off", on, err)
	}
	if err := SetCatchUp(home, b, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(CatchUpPath(home)); !os.IsNotExist(err) {
		t.Fatalf("turning off a root never listed wrote %s (%v)", CatchUpPath(home), err)
	}

	if err := SetCatchUp(home, a, true); err != nil {
		t.Fatal(err)
	}
	if err := SetCatchUp(home, a, true); err != nil { // twice: still one entry
		t.Fatal(err)
	}
	c, err := LoadCatchUp(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Roots) != 1 || c.Roots[0] != a || !c.On(a) || c.On(b) {
		t.Fatalf("after on for A: %+v, want A listed alone", c)
	}
	fi, err := os.Stat(CatchUpPath(home))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("catch-up.json mode: %v %v, want 0600", fi, err)
	}

	if err := SetCatchUp(home, b, true); err != nil {
		t.Fatal(err)
	}
	if err := SetCatchUp(home, a, false); err != nil {
		t.Fatal(err)
	}
	if on, _ := CatchUpOn(home, a); on {
		t.Fatal("A still on after off")
	}
	if on, _ := CatchUpOn(home, b+"/"); !on { // a root is compared cleaned
		t.Fatal("B off after A was turned off")
	}
}

// A catch-up.json that is not what this binary writes is an error, which every reader
// takes as off, never as a list it guessed at.
func TestCatchUpRejectsAFileItCannotRead(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"unknown field": `{"roots":["/a"],"on":true}`,
		"bare list":     `["/a"]`,
		"relative root": `{"roots":["a"]}`,
		"two values":    `{"roots":["/a"]}{}`,
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			if err := os.MkdirAll(filepath.Dir(CatchUpPath(home)), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(CatchUpPath(home), []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			on, err := CatchUpOn(home, "/a")
			if err == nil || on {
				t.Fatalf("on=%v err=%v, want an error and off", on, err)
			}
			if !strings.Contains(err.Error(), "catch-up.json") {
				t.Fatalf("error %q does not name the file", err)
			}
			if err := SetCatchUp(home, "/b", true); err == nil {
				t.Fatal("SetCatchUp rewrote a file it could not read")
			}
		})
	}
}
