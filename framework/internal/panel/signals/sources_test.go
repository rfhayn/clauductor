package signals

import (
	"strings"
	"testing"
)

func TestParseWorktreePorcelain(t *testing.T) {
	wts, err := parseWorktreePorcelain(fixture(t, "worktrees-fixture.porcelain"))
	if err != nil {
		t.Fatal(err)
	}
	if len(wts) != 4 || wts[1].Branch != "change/add-feature" || wts[3].Branch != "" {
		t.Fatalf("%+v", wts)
	}
	real, err := parseWorktreePorcelain(fixture(t, "worktrees-real.porcelain"))
	if err != nil || len(real) < 2 || real[0].Branch == "" {
		t.Fatalf("real capture: %+v %v", real, err)
	}
	if _, err := parseWorktreePorcelain([]byte("")); err == nil {
		t.Fatal("empty output accepted as a worktree list")
	}
}

func TestParsePRsSummarisesChecks(t *testing.T) {
	prs, err := ParsePRs(fixture(t, "prs.json"))
	if err != nil {
		t.Fatal(err)
	}
	p := prs[1]
	if p.Author != "rfhayn" || !p.IsDraft || p.ChecksPass != 2 || p.ChecksFail != 1 || p.ChecksWait != 2 {
		t.Fatalf("%+v", p)
	}
	if _, err := ParsePRs([]byte("gh: To get started with GitHub CLI, please run: gh auth login")); err == nil {
		t.Fatal("non-JSON gh output accepted")
	}
}

func TestParseCardOutput(t *testing.T) {
	if o := ParseCardOutput([]byte(" [ {\"title\": \"a\"} ]\n")); o.Kind != "json" || string(o.JSON) != `[{"title":"a"}]` {
		t.Fatalf("%+v", o)
	}
	o := ParseCardOutput([]byte("- [ ] one\n\n- [ ] two\n"))
	if o.Kind != "text" || len(o.Lines) != 2 {
		t.Fatalf("%+v", o)
	}
	if o := ParseCardOutput([]byte("{not json")); o.Kind != "text" {
		t.Fatal("invalid JSON not treated as text")
	}
	long := strings.Repeat("line\n", maxCardLines+10)
	if o := ParseCardOutput([]byte(long)); len(o.Lines) != maxCardLines+1 {
		t.Fatalf("lines %d", len(o.Lines))
	}
}
