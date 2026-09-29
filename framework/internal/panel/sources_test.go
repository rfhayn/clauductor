package panel

import (
	"testing"
	"time"
)

func TestConfigExampleFixtureParses(t *testing.T) {
	cfg := testConfig(t)
	if cfg.Name != "My Project" || len(cfg.Cards) != 2 {
		t.Fatalf("%+v", cfg)
	}
}

func TestConfigRejects(t *testing.T) {
	tests := map[string]string{
		"missing name":        `{"lanes":{}}`,
		"unknown key":         `{"name":"x","lane":{}}`,
		"bad card id":         `{"name":"x","cards":[{"id":"Bad Id","command":["true"],"refresh":"interval:60"}]}`,
		"duplicate card id":   `{"name":"x","cards":[{"id":"a","command":["true"],"refresh":"interval:60"},{"id":"a","command":["true"],"refresh":"interval:60"}]}`,
		"empty command":       `{"name":"x","cards":[{"id":"a","command":[],"refresh":"interval:60"}]}`,
		"string command":      `{"name":"x","cards":[{"id":"a","command":"ls -la","refresh":"interval:60"}]}`,
		"bad refresh":         `{"name":"x","cards":[{"id":"a","command":["true"],"refresh":"every:60"}]}`,
		"watch escapes repo":  `{"name":"x","cards":[{"id":"a","command":["true"],"refresh":"watch:../x"}]}`,
		"watch absolute path": `{"name":"x","cards":[{"id":"a","command":["true"],"refresh":"watch:/etc/passwd"}]}`,
		"zero interval":       `{"name":"x","cards":[{"id":"a","command":["true"],"refresh":"interval:0"}]}`,
		"empty lane type":     `{"name":"x","lanes":{"fix/":""}}`,
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseConfig([]byte(raw)); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

func TestRefreshIntervalFloor(t *testing.T) {
	r, err := ParseRefresh("interval:1")
	if err != nil || r.Interval != minCardInterval {
		t.Fatalf("%v %v", r, err)
	}
	r, _ = ParseRefresh("watch:docs/./founder-queue.md")
	if r.WatchRel != "docs/founder-queue.md" {
		t.Fatalf("%q", r.WatchRel)
	}
}

func TestLaneFor(t *testing.T) {
	cfg := &Config{Name: "x", Lanes: map[string]string{
		"change/": "build", "change/propose-*": "propose", "fix/": "fix", "main": "orchestrator",
	}}
	tests := []struct{ branch, typ, name string }{
		{"change/add-x", "build", "add-x"},
		{"change/propose-2c24", "propose", "2c24"}, // longest rule wins
		{"fix/bug", "fix", "bug"},
		{"main", "orchestrator", "main"},
		{"maintenance", "other", "maintenance"}, // exact rules are not prefixes
		{"feature/y", "other", "feature/y"},
		{"", "detached", "(detached)"},
	}
	for _, tc := range tests {
		typ, name := cfg.LaneFor(tc.branch)
		if typ != tc.typ || name != tc.name {
			t.Errorf("%q → %s/%s, want %s/%s", tc.branch, typ, name, tc.typ, tc.name)
		}
	}
}

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
