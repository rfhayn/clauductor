package config

import (
	"os"
	"path/filepath"
	"testing"
)

func testConfig(t *testing.T) *Config {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "panel.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := parseConfig(b)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func v2Config(t *testing.T, extra string) *Config {
	t.Helper()
	c, err := parseConfig([]byte(`{"name":"T","lanes":{"change/":"build","fix/":"fix","main":"orchestrator"}` + extra + `}`))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

const tplJSON = `,"templates":[
 {"id":"build","title":"Build a change","lane_type":"build","branch_pattern":"change/{name}","first_prompt":"Run build-change for {name}","model":"opus","effort":"high"},
 {"id":"fix","title":"Fix an issue","lane_type":"fix","first_prompt":"Fix issue {issue} on branch fix/{name}"}]`
