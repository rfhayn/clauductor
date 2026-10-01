package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/state"
)

// PANEL-19: economy mode switches on at the threshold and off only 3 points below it;
// it writes only on a switch; no reading keeps the mode; unset, it turns a leftover
// "on" off and otherwise does nothing.
func TestEconomyNext(t *testing.T) {
	t.Parallel()
	now := time.Unix(1790000000, 0)
	p := func(v float64) *float64 { return &v }
	var cur EconomyFile
	step := func(pct *float64, th float64) bool {
		next, changed := economyNext(cur, pct, th, now)
		cur = next
		now = now.Add(time.Minute)
		return changed
	}
	if !step(p(50), 85) || cur.Economy || cur.Since == 0 {
		t.Fatalf("the first reading writes off: %+v", cur)
	}
	if step(p(60), 85) {
		t.Fatal("no switch, no write")
	}
	if !step(p(87), 85) || !cur.Economy || cur.Reason != "5-hour quota 87% ≥ 85%" {
		t.Fatalf("on: %+v", cur)
	}
	since := cur.Since
	for _, v := range []float64{90, 84, 82.5} {
		if step(p(v), 85) || !cur.Economy || cur.Since != since {
			t.Fatalf("at %v it must stay on, unwritten: %+v", v, cur)
		}
	}
	if step(nil, 85) || !cur.Economy {
		t.Fatal("no reading must keep the mode")
	}
	if !step(p(81), 85) || cur.Economy || cur.Reason != "5-hour quota 81% < 82% (on at 85%)" {
		t.Fatalf("off 3 below: %+v", cur)
	}
	cur = EconomyFile{Economy: true, Since: 5, Reason: "x"}
	if !step(p(99), 0) || cur.Economy || cur.Reason != "quota_economy is not set" {
		t.Fatalf("unset turns a leftover on off: %+v", cur)
	}
	if step(p(99), 0) {
		t.Fatal("unset and off writes nothing")
	}
}

// The badge's roles come from model-roles.json's "economy" mapping, in either form.
func TestEconomyRoles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if r, note := economyRoles(root); r != nil || note == "" {
		t.Fatalf("no file: %v %q", r, note)
	}
	writeFile(t, filepath.Join(root, ".claude", "model-roles.json"), `{"roles":{},"economy":{"_why":"x","scribe":{"model":"sonnet","effort":"low"},"mechanic":"haiku/low","bad":3}}`)
	r, note := economyRoles(root)
	if note != "" || len(r) != 2 || r[0] != (state.EconomyRole{Role: "mechanic", To: "haiku, low"}) || r[1] != (state.EconomyRole{Role: "scribe", To: "sonnet, low"}) {
		t.Fatalf("roles %+v %q", r, note)
	}
	writeFile(t, filepath.Join(root, ".claude", "model-roles.json"), `{"economy":{"roles":{"orient":"sonnet"}}}`)
	if r, _ := economyRoles(root); len(r) != 1 || r[0].Role != "orient" {
		t.Fatalf("nested roles %+v", r)
	}
	writeFile(t, filepath.Join(root, ".claude", "model-roles.json"), `{"roles":{}}`)
	if _, note := economyRoles(root); note == "" {
		t.Fatal("no mapping says so")
	}
}

// End to end, in a temp HOME: a status post at 87% turns economy on (economy.json and
// the badge's view, with the project's roles); one at 80% turns it off.
func TestEconomyEndToEnd(t *testing.T) {
	t.Parallel()
	root, home := setupProject(t)
	writeFile(t, filepath.Join(root, config.DefaultConfigRel), `{"name":"Eco","version":4,"lanes":{"main":"orchestrator"},"tmux_socket":"`+noServerSocket()+`",
		"quota_economy":{"five_hour_pct":85}}`)
	writeFile(t, filepath.Join(root, ".claude", "model-roles.json"), `{"economy":{"scribe":"sonnet/low"}}`)
	polls := newPollCounter()
	ctx, cancel := context.WithCancel(context.Background())
	ready, done := make(chan string, 1), make(chan error, 1)
	ticks := fastTicks()
	ticks.Trends, ticks.Spend = 50*time.Millisecond, 50*time.Millisecond
	go func() {
		done <- Run(ctx, Options{Project: root, Port: 0, NoOpen: true, Home: home, TrustConfig: true, Runner: fakeRunner(root), Ticks: ticks,
			OnPoll: polls.hook, OnReady: func(u string) { ready <- u }})
	}()
	defer func() { cancel(); <-done }()
	var c liveClient
	select {
	case launch := <-ready:
		u, _ := url.Parse(launch)
		port, _ := strconv.Atoi(u.Port())
		c = liveClient{base: "http://" + u.Host, cookie: fmt.Sprintf("clauductor_panel_%d=%s", port, u.Query().Get("t"))}
	case err := <-done:
		t.Fatalf("Run exited: %v", err)
	}
	read := func() EconomyFile {
		var f EconomyFile
		b, err := os.ReadFile(EconomyPath(home))
		if err != nil {
			return f
		}
		_ = json.Unmarshal(b, &f)
		return f
	}
	post := func(pct int) {
		if code := c.post(t, "/status", fmt.Sprintf(`{"session_id":"s9","cwd":"/elsewhere","rate_limits":{"five_hour":{"used_percentage":%d}}}`, pct)); code != 204 {
			t.Fatalf("status post: %d", code)
		}
	}
	post(87)
	waitUntil(t, "economy on", 15*time.Second, func() bool { return read().Economy })
	if f := read(); f.Reason != "5-hour quota 87% ≥ 85%" || f.Since == 0 {
		t.Fatalf("economy.json %+v", f)
	}
	if fi, err := os.Stat(EconomyPath(home)); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("economy.json mode %v %v", fi, err)
	}
	waitUntil(t, "the badge", 15*time.Second, func() bool {
		v := c.state(t)
		return v.Economy != nil && v.Economy.Active && len(v.Economy.Roles) == 1 && v.Economy.Roles[0].Role == "scribe"
	})
	post(80)
	waitUntil(t, "economy off", 15*time.Second, func() bool { return !read().Economy })
	waitUntil(t, "no badge", 15*time.Second, func() bool { return c.state(t).Economy == nil })
}
