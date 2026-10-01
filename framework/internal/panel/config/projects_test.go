package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// PANEL-16: projects.json is read strictly: a file that does not validate is an
// error, never a silent reset to no projects.
func TestProjectsLoadAndValidate(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	p, err := LoadProjects(home)
	if err != nil || len(p.Projects) != 0 || p.Version != ProjectsVersion {
		t.Fatalf("a missing file is an empty registry: %+v %v", p, err)
	}
	good := `{"version":1,"default":"app","projects":[{"id":"app","root":"/r/app","config":"","tmux_socket":"clauductor","added":1},
		{"id":"web","root":"/r/web","config":"/r/web/panel.json","tmux_socket":"clauductor-web","added":2}]}`
	for name, body := range map[string]string{
		"unknown field":   strings.Replace(good, `"added":1`, `"added":1,"lane_auth":"any"`, 1),
		"duplicate id":    strings.Replace(good, `"id":"web"`, `"id":"app"`, 1),
		"duplicate root":  strings.Replace(good, `"root":"/r/web"`, `"root":"/r/app"`, 1),
		"shared socket":   strings.Replace(good, `"clauductor-web"`, `"clauductor"`, 1),
		"bad id":          strings.Replace(good, `"id":"web"`, `"id":"Web!"`, 1),
		"relative root":   strings.Replace(good, `"root":"/r/web"`, `"root":"r/web"`, 1),
		"unclean root":    strings.Replace(good, `"root":"/r/web"`, `"root":"/r/../web"`, 1),
		"relative config": strings.Replace(good, `"config":"/r/web/panel.json"`, `"config":"panel.json"`, 1),
		"bad socket":      strings.Replace(good, `"clauductor-web"`, `"a b"`, 1),
		"unknown default": strings.Replace(good, `"default":"app"`, `"default":"nope"`, 1),
		"no default":      strings.Replace(good, `"default":"app"`, `"default":""`, 1),
		"another version": strings.Replace(good, `"version":1`, `"version":2`, 1),
		"trailing data":   good + `{}`,
		"not json":        `projects: app`,
	} {
		writeProjects(t, home, body)
		if _, err := LoadProjects(home); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	writeProjects(t, home, good)
	p, err = LoadProjects(home)
	if err != nil || len(p.Projects) != 2 || p.DefaultEntry().ID != "app" {
		t.Fatalf("good registry: %+v %v", p, err)
	}
	if e := p.Find("/r/web"); e == nil || e.ID != "web" || e.ConfigPath() != "/r/web/panel.json" {
		t.Fatalf("find by root: %+v", e)
	}
	if p.Find("app").ConfigPath() != filepath.Join("/r/app", DefaultConfigRel) {
		t.Fatal("an empty config is the project's default path")
	}
	// Written 0600, atomically, and read back the same.
	if err := SaveProjects(home, p); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(ProjectsPath(home)); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("projects.json mode: %v %v", fi, err)
	}
	p.Projects[1].ID = "app"
	if SaveProjects(home, p) == nil {
		t.Fatal("an invalid registry was saved")
	}
}

func writeProjects(t *testing.T, home, body string) {
	t.Helper()
	if err := os.MkdirAll(PanelDir(home), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ProjectsPath(home), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Ids come from names; the first project keeps the historical socket, so lanes
// started before PANEL-16 carry on, and every later one gets its own.
func TestProjectsSlugAndSockets(t *testing.T) {
	t.Parallel()
	p := &Projects{Version: 1}
	for name, want := range map[string]string{"StandingT": "standingt", "My App!": "my-app", "  ": "project", "--x--": "x",
		strings.Repeat("a", 50): strings.Repeat("a", 36)} {
		if got := p.Slug(name); got != want {
			t.Errorf("Slug(%q) = %q, want %q", name, got, want)
		}
	}
	if s := p.NewSocket("app"); s != DefaultTmuxSocket {
		t.Fatalf("the first project's socket: %s", s)
	}
	p.Projects = append(p.Projects, ProjectEntry{ID: "app", Root: "/a", TmuxSocket: DefaultTmuxSocket})
	p.Default = "app"
	if s := p.NewSocket("web"); s != "clauductor-web" {
		t.Fatalf("a later project's socket: %s", s)
	}
	if id := p.Slug("App"); id != "app-2" {
		t.Fatalf("a taken id: %s", id)
	}
	// panel.json's tmux_socket wins over the recorded one; neither: the default.
	e := ProjectEntry{TmuxSocket: "clauductor-web"}
	if e.Socket(&Config{TmuxSocket: "mine"}) != "mine" || e.Socket(&Config{}) != "clauductor-web" || (ProjectEntry{}).Socket(nil) != DefaultTmuxSocket {
		t.Fatal("socket resolution")
	}
}
