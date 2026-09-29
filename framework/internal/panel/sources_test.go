package panel

import (
	"testing"
	"time"
)

func TestPathSignatureSeesChanges(t *testing.T) {
	dir := t.TempDir()
	a := pathSignature(dir)
	writeFile(t, dir+"/f", "x")
	b := pathSignature(dir)
	if a == b {
		t.Fatal("new file not seen")
	}
	time.Sleep(10 * time.Millisecond)
	writeFile(t, dir+"/f", "xy")
	if pathSignature(dir) == b {
		t.Fatal("changed file not seen")
	}
	if pathSignature(dir+"/nope") != "missing" {
		t.Fatal("missing path")
	}
}
