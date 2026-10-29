package signals

import (
	"context"
	"strings"
	"testing"
)

func TestParseWorktreePorcelain(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	// Exactly the limit is all of it: no line was cut, so none says so (PANEL-47).
	exact := ParseCardOutput([]byte(strings.Repeat("line\n", maxCardLines) + "\n\n"))
	if len(exact.Lines) != maxCardLines || exact.Lines[maxCardLines-1] != "line" {
		t.Fatalf("%d lines at the limit, ending %q", len(exact.Lines), exact.Lines[len(exact.Lines)-1])
	}
}

// ExecRunner keeps at most maxCmdOutput of a command's stdout, and of its stderr when
// asked for it: exec copies each pipe with io.Copy, which must not reach around the
// limit through bytes.Buffer's ReadFrom.
func TestExecRunnerLimitsOutput(t *testing.T) {
	t.Parallel()
	var stderr []byte
	ctx := WithStderr(context.Background(), func(b []byte) { stderr = b })
	big := "head -c 1100000 /dev/zero; head -c 1100000 /dev/zero >&2"
	out, err := ExecRunner(ctx, t.TempDir(), []string{"sh", "-c", big})
	if err != nil || len(out) != maxCmdOutput || len(stderr) != maxCmdOutput {
		t.Fatalf("kept %d bytes of stdout and %d of stderr, want %d each: %v", len(out), len(stderr), maxCmdOutput, err)
	}
}

// ATTACH-6's requirement, a sidecar's card shows its stderr: ExecRunner hands a
// successful command's stderr to WithStderr's sink, and a text card gains its lines
// after stdout's, under the line limit, while a JSON card is left as it is.
func TestCardOutputWithStderr(t *testing.T) {
	t.Parallel()
	var stderr []byte
	ctx := WithStderr(context.Background(), func(b []byte) { stderr = b })
	out, err := ExecRunner(ctx, t.TempDir(), []string{"sh", "-c", "echo out; echo err >&2"})
	if err != nil || string(out) != "out\n" || string(stderr) != "err\n" {
		t.Fatalf("stdout %q stderr %q: %v", out, stderr, err)
	}
	if got := ParseCardOutput(out).WithStderr(stderr); got.Kind != "text" || strings.Join(got.Lines, "|") != "out|err" {
		t.Errorf("text card %+v", got)
	}
	// Within stdout's limit, not the 4 KiB kept for a failure's message.
	long := "i=0; while [ $i -lt 600 ]; do echo stderr-line-$i >&2; i=$((i+1)); done"
	if _, err := ExecRunner(ctx, t.TempDir(), []string{"sh", "-c", long}); err != nil || len(stderr) < 600*len("stderr-line-0\n") {
		t.Errorf("a long stderr kept %d bytes: %v", len(stderr), err)
	}
	if got := ParseCardOutput([]byte(`{"a":1}`)).WithStderr(stderr); got.Kind != "json" || len(got.Lines) != 0 {
		t.Errorf("JSON card %+v", got)
	}
	full := ParseCardOutput([]byte(strings.Repeat("x\n", maxCardLines+5)))
	if got := full.WithStderr(stderr); len(got.Lines) != maxCardLines+1 || got.Lines[maxCardLines] != "… (truncated)" {
		t.Errorf("a full card grew: %d lines", len(got.Lines))
	}
	near := ParseCardOutput([]byte(strings.Repeat("x\n", maxCardLines-1)))
	if got := near.WithStderr([]byte("e1\ne2\n")); len(got.Lines) != maxCardLines+1 || got.Lines[maxCardLines-1] != "e1" {
		t.Errorf("the limit holds with stderr: %d lines, %q", len(got.Lines), got.Lines[maxCardLines-1:])
	}
	// Reaching the limit exactly cuts nothing; a line past it, from either stream, is
	// cut and says so (PANEL-47).
	if got := near.WithStderr([]byte("e1\n\n")); len(got.Lines) != maxCardLines || got.Lines[maxCardLines-1] != "e1" {
		t.Errorf("stderr filling the card exactly: %d lines, %q", len(got.Lines), got.Lines[maxCardLines-1:])
	}
	exact := ParseCardOutput([]byte(strings.Repeat("x\n", maxCardLines)))
	if got := exact.WithStderr(nil); len(got.Lines) != maxCardLines {
		t.Errorf("a card exactly full with no stderr: %d lines", len(got.Lines))
	}
	if got := exact.WithStderr([]byte("e1\n")); len(got.Lines) != maxCardLines+1 || got.Lines[maxCardLines] != "… (truncated)" {
		t.Errorf("stderr past a card exactly full: %d lines, %q", len(got.Lines), got.Lines[maxCardLines-1:])
	}
}
