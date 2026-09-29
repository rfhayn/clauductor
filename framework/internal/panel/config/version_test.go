package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The version means what it says: a key from a later version than the file
// declares is refused, naming the key and the version it needs.
func TestVersionGatesKeys(t *testing.T) {
	const lanes = `"lanes":{"feature/":"build","main":"orchestrator"}`
	refused := []struct{ body, key string }{
		{`{"name":"T","version":1,` + lanes + `,"templates":[{"id":"a","lane_type":"build","first_prompt":"x"}]}`, `"templates"`},
		{`{"name":"T","version":1,"queues":[{"id":"g","lock":"clauductor/gate.lock"}]}`, `"queues"`},
		{`{"name":"T","version":1,"alerts":{"idle_minutes":5}}`, `"alerts"`},
		{`{"name":"T","version":1,"quota_guard":{"five_hour_pct":90}}`, `"quota_guard"`},
		{`{"name":"T","version":1,"host_names":["app.localhost"]}`, `"host_names"`},
	}
	for _, r := range refused {
		_, err := parseConfig([]byte(r.body))
		if err == nil {
			t.Errorf("version 1 accepted %s", r.key)
			continue
		}
		if !strings.Contains(err.Error(), r.key) || !strings.Contains(err.Error(), `"version": 2`) {
			t.Errorf("the error must name %s and the version it needs: %v", r.key, err)
		}
	}
	// Every version 1 key is accepted at version 1.
	c, err := parseConfig([]byte(`{"$schema":"x","name":"T","version":1,` + lanes + `,"tmux_socket":"s","worktree_dir":"wt","base":"origin/main",
		"lane_types":{"build":{"model":"opus","effort":"high"}},"cards":[{"id":"a","title":"A","command":["true"],"refresh":"interval:60"}]}`))
	if err != nil || len(c.Notices) != 0 || c.Version != 1 {
		t.Fatalf("a full version 1 config: %v %v", err, c)
	}
	// An explicit version outside the supported range is refused, 0 included.
	for _, v := range []string{"0", "3", "-1"} {
		if _, err := parseConfig([]byte(`{"name":"T","version":` + v + `}`)); err == nil || !strings.Contains(err.Error(), "not supported") {
			t.Errorf("version %s: %v", v, err)
		}
	}
}

// A file with no version is read as the latest, with one notice; nothing an existing
// config uses is refused.
func TestMissingVersionReadsAsLatest(t *testing.T) {
	c, err := parseConfig([]byte(`{"name":"T","lanes":{"feature/":"build"},"templates":[{"id":"a","lane_type":"build","first_prompt":"x"}],"alerts":{"idle_minutes":5}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Notices) != 1 || !strings.Contains(c.Notices[0], `read as version 2`) || !strings.Contains(c.Notices[0], `Add "version": 2`) {
		t.Fatalf("notices %q", c.Notices)
	}
	c, err = parseConfig([]byte(`{"name":"T","version":2,"templates":[]}`))
	if err != nil || len(c.Notices) != 0 {
		t.Fatalf("a declared version says nothing: %v %q", err, c.Notices)
	}
	if c := testConfig(t); len(c.Notices) != 1 {
		t.Fatalf("the example fixture declares no version: %q", c.Notices)
	}
}

// Fields is checked against the authority, the Config type as encoding/json reads
// it, in both directions: a new key cannot ship without a version and a reference
// row, and a removed key cannot leave one behind.
func TestFieldsCoverEveryConfigKey(t *testing.T) {
	fields := fieldByPath()
	if len(fields) != len(Fields) {
		t.Fatal("Fields has a duplicate path")
	}
	seen := map[string]bool{}
	configPaths(reflect.TypeOf(Config{}), "", func(p string, _ reflect.Type) {
		seen[p] = true
		f, ok := fields[p]
		if !ok {
			t.Errorf("config key %q has no entry in Fields (it needs a version and a reference row)", p)
			return
		}
		if f.Version < MinVersion || f.Version > LatestVersion || f.Doc == "" || f.Type == "" {
			t.Errorf("Fields[%q] needs a version in %d..%d, a type and a doc: %+v", p, MinVersion, LatestVersion, f)
		}
	})
	if len(seen) < 30 {
		t.Fatalf("walked %d keys; the walk is not seeing Config", len(seen))
	}
	for _, f := range Fields {
		if !seen[f.Path] {
			t.Errorf("Fields[%q] names no config key", f.Path)
		}
		// The schema's version gate covers top-level keys: a nested key newer than
		// its parent would pass the schema while the panel refuses it.
		if i := strings.LastIndexAny(f.Path, ".["); i > 0 {
			parent := strings.TrimSuffix(strings.TrimSuffix(f.Path[:i], ".*"), "[]")
			if p, ok := fields[parent]; ok && p.Version != f.Version {
				t.Errorf("Fields[%q] is version %d inside %q (version %d); teach jsonSchema nested gates first", f.Path, f.Version, parent, p.Version)
			}
		}
	}
}

// Every JSON example in docs/panel.md is a config the panel accepts.
func TestDocsExamplesParse(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "panel.md"))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, block := range strings.Split(string(b), "```json\n")[1:] {
		body, _, _ := strings.Cut(block, "```")
		if !strings.Contains(body, `"name"`) {
			continue // a fragment, such as a hook entry
		}
		n++
		if _, err := parseConfig([]byte(body)); err != nil {
			t.Errorf("a docs example does not parse: %v\n%s", err, body)
		}
	}
	if n == 0 {
		t.Fatal("found no config example in docs/panel.md")
	}
}
