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
	// Version 3's keys sit inside older ones, and are refused where they sit.
	for _, r := range []struct{ body, key string }{
		{`{"name":"T","version":2,` + lanes + `,"templates":[{"id":"a","lane_type":"build","first_prompt":"x","suggest":{"command":["true"],"refresh":"interval:60"}}]}`, `"templates[].suggest"`},
		{`{"name":"T","version":2,"cards":[{"id":"a","command":["true"],"refresh":"interval:60","pin":true}]}`, `"cards[].pin"`},
	} {
		_, err := parseConfig([]byte(r.body))
		if err == nil || !strings.Contains(err.Error(), r.key) || !strings.Contains(err.Error(), `"version": 3`) {
			t.Errorf("version 2 and %s: %v", r.key, err)
		}
	}
	// PANEL-19: metrics is version 4.
	if _, err := parseConfig([]byte(`{"name":"T","version":3,"metrics":{"command":["true"]}}`)); err == nil ||
		!strings.Contains(err.Error(), `"metrics"`) || !strings.Contains(err.Error(), `"version": 4`) {
		t.Errorf("version 3 and metrics: %v", err)
	}
	if c, err := parseConfig([]byte(`{"name":"T","version":4,"metrics":{"command":["sh","m.sh"],"refresh":"watch:changes","card":false}}`)); err != nil ||
		c.MetricsRefresh() != "watch:changes" || c.FlowCard() || len(c.MetricsCommand()) != 2 {
		t.Errorf("a version 4 metrics config: %v %+v", err, c)
	}
	if c, err := parseConfig([]byte(`{"name":"T","version":4,"metrics":{"command":["sh","m.sh"]}}`)); err != nil ||
		c.MetricsRefresh() != DefaultMetricsRefresh || !c.FlowCard() || !strings.Contains(strings.Join(c.RunList(), "\n"), `metrics runs ["sh" "m.sh"]`) {
		t.Errorf("metrics defaults and the trust list: %v %v", err, c.RunList())
	}
	// Every version 1 key is accepted at version 1.
	c, err := parseConfig([]byte(`{"$schema":"x","name":"T","version":1,` + lanes + `,"tmux_socket":"s","worktree_dir":"wt","base":"origin/main",
		"lane_types":{"build":{"model":"opus","effort":"high"}},"cards":[{"id":"a","title":"A","command":["true"],"refresh":"interval:60"}]}`))
	if err != nil || len(c.Notices) != 0 || c.Version != 1 {
		t.Fatalf("a full version 1 config: %v %v", err, c)
	}
	// An explicit version outside the supported range is refused, 0 included.
	for _, v := range []string{"0", "5", "-1"} {
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
	if len(c.Notices) != 1 || !strings.Contains(c.Notices[0], `read as version 4`) || !strings.Contains(c.Notices[0], `Add "version": 4`) {
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
		// A key can be newer than the key it sits in (jsonSchema bans it in place),
		// never older: a version 1 key inside a version 2 one means nothing.
		if parent := parentPath(f.Path); parent != "" {
			if p, ok := fields[parent]; !ok || f.Version < p.Version {
				t.Errorf("Fields[%q] is version %d inside %q (version %d, known %v); a key is never older than its parent", f.Path, f.Version, parent, p.Version, ok)
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
