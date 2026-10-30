package install

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rfhayn/stridal/internal/panel/config"
	"github.com/rfhayn/stridal/internal/panel/signals"
	"github.com/rfhayn/stridal/internal/testbin"
)

// OPS-111: init's starter templates keep the shipped config's Up next refresh and ops prompt.
// panel-suggest.sh reads origin/main, which a working-tree watch never sees move, and an ops lane
// named after a queued row builds that row; a starter that kept the old values would do neither.
func TestInitTemplatesMatchTheShippedConfig(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "template", ".stridal", "panel.json"))
	if err != nil {
		t.Fatal(err)
	}
	shipped, err := config.ParseConfig(body)
	if err != nil {
		t.Fatal(err)
	}
	root := newRepo(t, nil)
	for _, name := range []string{".claude/owner-queue.sh", ".claude/roadmap-queue.sh", ".claude/panel-suggest.sh"} {
		os.MkdirAll(filepath.Join(root, filepath.Dir(name)), 0o755)
		testbin.Write(t, filepath.Join(root, name), "#!/bin/sh\n")
	}
	res, err := InitConfig(context.Background(), signals.ExecRunner, root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := config.ParseConfig(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]config.TemplateConfig{}
	for _, tp := range shipped.Templates {
		want[tp.ID] = tp
	}
	if len(got.Templates) != len(want) {
		t.Errorf("init writes %d templates, the shipped config has %d", len(got.Templates), len(want))
	}
	for _, tp := range got.Templates {
		w, ok := want[tp.ID]
		if !ok || tp.Suggest == nil || w.Suggest == nil {
			t.Errorf("template %s: shipped %v, suggest %+v", tp.ID, ok, tp.Suggest)
			continue
		}
		if tp.Suggest.Refresh != w.Suggest.Refresh {
			t.Errorf("template %s refreshes %q, the shipped config %q", tp.ID, tp.Suggest.Refresh, w.Suggest.Refresh)
		}
		// OPS-125: every first prompt, since each now starts and closes its row's session.
		if tp.FirstPrompt != w.FirstPrompt {
			t.Errorf("the %s prompt differs from the shipped config's:\n%s\n%s", tp.ID, tp.FirstPrompt, w.FirstPrompt)
		}
	}
}
