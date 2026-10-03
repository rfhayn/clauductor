package web

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// start-plan.js decides the New lane dialog's start (PANEL-29, D4): which template and
// name, where the lane runs, which "Where it runs" choices the server would accept, and
// what is in a build's way. This runs the embedded file in node against a page state
// shaped like the View: Standing Tee's build and propose templates share change/{name}.
const startPlanHarness = `
const vm = require("vm"), fs = require("fs");
const window = {};
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), { window });
const P = window.StartPlan.planStart;
const tpl = (id, branchPattern, firstPrompt) => ({ id, title: id, laneType: "build", branchPattern, firstPrompt });
const base = {
  root: "/repo",
  templates: [
    tpl("build", "change/{name}", '/build-change {"change": "{name}"}'),
    tpl("propose", "change/{name}", "/propose {name}: its row of the change queue."),
    tpl("fix", "fix/{name}", "Fix GitHub issue {issue} on this branch."),
    tpl("bug", "bug/{issue}-{name}", "Fix bug {issue}."),
    tpl("mention", "spike/{name}", "Read the notes, then run /build-change {name} when ready."),
  ],
  suggestions: {
    build: { source: { ok: true }, items: [{ name: "add-score-photo" }] },
    propose: { source: { ok: true }, items: [{ name: "add-group-card-entry" }] },
  },
  lanes: [
    { path: "/repo", branch: "main", name: "main" },
    { path: "/repo/w/add-score-photo", branch: "change/add-score-photo", name: "add-score-photo" },
  ],
  quietWorktrees: [
    { path: "/repo/w/add-group-card-entry", branch: "change/add-group-card-entry", name: "add-group-card-entry" },
    { path: "/repo/w/listed-nowhere", branch: "change/listed-nowhere", name: "listed-nowhere" },
    { path: "/repo/w/12-login", branch: "bug/12-login", name: "12-login" },
    { path: "/repo/w/My Scratch", branch: "scratch/thing", name: "My Scratch" },
  ],
  approvals: { "add-score-photo": true },
};
const unapproved = Object.assign({}, base, { approvals: { "add-score-photo": false, "add-group-card-entry": false } });
const exists = "A branch named change/old-work already exists. Start on the branch in a new worktree.";
const inWT = "A branch named change/old-work already exists, in the worktree /elsewhere/old-work. Start the lane on that worktree.";
const midRebase = "A branch named change/old-work is in the middle of a rebase in the worktree /repo/w/old-work. Finish or abort it there, then start the lane on that worktree.";
const old = (facts) => ({ from: "branch", template: "build", name: "old-work", facts: Object.assign({ branch: "change/old-work" }, facts) });
const conflict = (error) => ({ from: "conflict", template: "build", name: "old-work", error: Object.assign({ ok: false, code: "branch-exists", branch: "change/old-work", local: true, remote: true }, error) });
console.log(JSON.stringify({
  tplWorktree: P(base, { from: "template", template: "build", name: "add-score-photo" }),
  nextWorktree: P(base, { from: "next", template: "build", item: { name: "add-score-photo" } }),
  factsWorktree: P(base, old({ local: true, remote: true, worktree: "/elsewhere/old-work" })),
  tplNoBranch: P(base, { from: "template", template: "build", name: "brand-new" }),
  factsLocal: P(base, old({ local: true, remote: false })),
  factsOrigin: P(base, old({ local: false, remote: true })),
  factsNone: P(base, old({ local: false, remote: false })),
  factsStale: P(base, { from: "branch", template: "build", name: "other-name", facts: { branch: "change/old-work", local: true } }),
  factsBusy: P(base, old({ local: true, remote: true, worktree: "/repo/w/old-work", busy: "rebase" })),
  conflictBranch: P(base, conflict({ error: exists, local: true, remote: false })),
  conflictWorktree: P(base, conflict({ error: inWT, worktree: "/elsewhere/old-work" })),
  conflictBusy: P(base, conflict({ error: midRebase, worktree: "/repo/w/old-work", busy: "rebase" })),
  conflictPlain: P(base, { from: "conflict", template: "", name: "old-work", error: { code: "branch-exists", error: "A branch named build/old-work already exists. Choose another name for the lane.", branch: "build/old-work", local: true } }),
  hereBuild: P(base, { from: "worktree", path: "/repo/w/add-score-photo" }),
  herePropose: P(base, { from: "worktree", path: "/repo/w/add-group-card-entry" }),
  hereFirst: P(base, { from: "worktree", path: "/repo/w/listed-nowhere" }),
  hereIssue: P(base, { from: "worktree", path: "/repo/w/12-login" }),
  hereNone: P(base, { from: "worktree", path: "/repo/w/My Scratch" }),
  hereMain: P(base, { from: "worktree", path: "/repo" }),
  issueMissing: P(base, { from: "template", template: "bug", name: "login" }),
  issueGiven: P(base, { from: "template", template: "bug", name: "login", issue: "12" }),
  plain: P(base, { from: "template", template: "", name: "anything" }),
  buildUnapproved: P(unapproved, { from: "template", template: "build", name: "add-score-photo" }),
  hereUnapproved: P(unapproved, { from: "worktree", path: "/repo/w/add-score-photo" }),
  buildApproved: P(base, { from: "template", template: "build", name: "add-score-photo" }),
  buildNoProposal: P(base, { from: "template", template: "build", name: "no-proposal-yet" }),
  proposeUnapproved: P(unapproved, { from: "template", template: "propose", name: "add-group-card-entry" }),
  proposeNoProposal: P(base, { from: "template", template: "propose", name: "add-group-card-entry" }),
  mentionUnapproved: P(unapproved, { from: "template", template: "mention", name: "add-score-photo" }),
}));
`

// startPlan is one plan as the harness prints it.
type startPlan struct {
	Template string          `json:"template"`
	Name     string          `json:"name"`
	Mode     *string         `json:"mode"`
	Worktree string          `json:"worktree"`
	Choices  map[string]bool `json:"choices"`
	Warnings []string        `json:"warnings"`
	Note     string          `json:"note"`
}

// mode is a plan's mode, "<none>" when it offers none.
func (p startPlan) mode() string {
	if p.Mode == nil {
		return "<none>"
	}
	return *p.Mode
}

// enabled lists the "Where it runs" choices a plan enables, in a fixed order.
func (p startPlan) enabled() string {
	var on []string
	for _, m := range []string{"new", "existing", "branch", "root"} {
		if p.Choices[m] {
			on = append(on, m)
		}
	}
	return strings.Join(on, ",")
}

func runStartPlan(t *testing.T) map[string]startPlan {
	t.Helper()
	if testing.Short() {
		t.Skip("-short: runs node against start-plan.js")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; the New lane dialog's start plan is untested here")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "start-plan.js")
	if err := os.WriteFile(src, []byte(readWeb(t, "start-plan.js")), 0o644); err != nil {
		t.Fatal(err)
	}
	h := filepath.Join(dir, "harness.js")
	if err := os.WriteFile(h, []byte(startPlanHarness), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := exec.Command(node, h, src).CombinedOutput()
	if err != nil {
		t.Fatalf("harness: %v\n%s", err, b)
	}
	var got map[string]startPlan
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("harness output: %v\n%s", err, b)
	}
	return got
}

// placed is where a plan runs the lane: template, name, mode, worktree and the enabled choices.
type placed struct{ template, name, mode, worktree, enabled string }

func (p startPlan) placed() placed {
	return placed{p.Template, p.Name, p.mode(), p.Worktree, p.enabled()}
}

func TestStartPlan(t *testing.T) {
	t.Parallel()
	got := runStartPlan(t)
	check := func(id, key string, want placed) {
		t.Helper()
		if g := got[key].placed(); g != want {
			t.Errorf("%s %s: got %+v, want %+v", id, key, g, want)
		}
	}
	// [NEWLANE-2-S2] The build template and add-score-photo, whose branch already has a
	// worktree: "An existing worktree" with that worktree, and "New branch and worktree"
	// neither chosen nor offered. The same from Up next, and from the server's answer.
	check("[NEWLANE-2-S2]", "tplWorktree", placed{"build", "add-score-photo", "existing", "/repo/w/add-score-photo", "existing"})
	check("[NEWLANE-2-S2]", "nextWorktree", placed{"build", "add-score-photo", "existing", "/repo/w/add-score-photo", "existing"})
	check("[NEWLANE-2-S2]", "factsWorktree", placed{"build", "old-work", "existing", "/elsewhere/old-work", "existing"})

	// [NEWLANE-2-S3] The server reports the branch with no worktree: "Its existing branch,
	// in a new worktree" is chosen, with a line saying why.
	check("[NEWLANE-2-S3]", "factsLocal", placed{"build", "old-work", "branch", "", "branch"})
	check("[NEWLANE-2-S3]", "factsOrigin", placed{"build", "old-work", "branch", "", "branch"})
	if n := got["factsLocal"].Note; !strings.Contains(n, "A branch named change/old-work already exists") || !strings.Contains(n, "new worktree") || strings.Contains(n, "on origin") {
		t.Errorf("[NEWLANE-2-S3] factsLocal: the line saying why is %q", n)
	}
	if n := got["factsOrigin"].Note; !strings.Contains(n, "already exists on origin") {
		t.Errorf("[NEWLANE-2-S3] factsOrigin: the line saying why is %q, want it to say origin has the branch", n)
	}
	for _, k := range []string{"factsLocal", "factsOrigin"} {
		if len(got[k].Warnings) != 0 {
			t.Errorf("[NEWLANE-2-S3] %s: the line saying why is not a warning: %q", k, got[k].Warnings)
		}
	}
	// A branch that exists nowhere is a new one; an answer about another branch (the
	// name changed while it was asked) is ignored, so the start stays new.
	check("tplNoBranch", "tplNoBranch", placed{"build", "brand-new", "new", "", "new"})
	check("factsNone", "factsNone", placed{"build", "old-work", "new", "", "new"})
	check("factsStale", "factsStale", placed{"build", "other-name", "new", "", "new"})

	// [NEWLANE-2-S4] A Start refused with branch-exists, naming no worktree: "Its existing
	// branch, in a new worktree" is chosen, with the refusal's sentence shown.
	check("[NEWLANE-2-S4]", "conflictBranch", placed{"build", "old-work", "branch", "", "branch"})
	if n := got["conflictBranch"].Note; n != "A branch named change/old-work already exists. Start on the branch in a new worktree." {
		t.Errorf("[NEWLANE-2-S4] conflictBranch shows %q, want the refusal's sentence", n)
	}
	check("conflictWorktree", "conflictWorktree", placed{"build", "old-work", "existing", "/elsewhere/old-work", "existing"})
	if n := got["conflictWorktree"].Note; !strings.HasPrefix(n, "A branch named change/old-work already exists, in the worktree") {
		t.Errorf("conflictWorktree shows %q, want the refusal's sentence", n)
	}
	// A plain lane's refusal leaves where it runs to the person, and shows the sentence.
	check("conflictPlain", "conflictPlain", placed{"", "old-work", "<none>", "", "new,existing,root"})
	if n := got["conflictPlain"].Note; !strings.HasSuffix(n, "Choose another name for the lane.") {
		t.Errorf("conflictPlain shows %q", n)
	}

	// The owner's ruling (group 1, review round 4): a branch a stopped rebase or bisect
	// holds offers no start anywhere, and the server's sentence is the warning.
	busy := "A branch named change/old-work is in the middle of a rebase in the worktree /repo/w/old-work. Finish or abort it there, then start the lane on that worktree."
	for _, k := range []string{"factsBusy", "conflictBusy"} {
		check("busy", k, placed{"build", "old-work", "<none>", "", ""})
		if w := got[k].Warnings; !reflect.DeepEqual(w, []string{busy}) {
			t.Errorf("busy %s: warnings %q, want the server's sentence", k, w)
		}
	}

	// [NEWLANE-3-S1] "New lane here" on change/add-score-photo, which Up next lists under
	// build: the build template, the name and that worktree are selected.
	check("[NEWLANE-3-S1]", "hereBuild", placed{"build", "add-score-photo", "existing", "/repo/w/add-score-photo", "existing"})
	// [NEWLANE-3-S2] build and propose both name change/{name}; Up next lists
	// add-group-card-entry under propose only, so propose is selected.
	check("[NEWLANE-3-S2]", "herePropose", placed{"propose", "add-group-card-entry", "existing", "/repo/w/add-group-card-entry", "existing"})
	// D5: listed by no Up next, the first template in the config's order wins.
	check("D5", "hereFirst", placed{"build", "listed-nowhere", "existing", "/repo/w/listed-nowhere", "existing"})
	// A pattern with an issue still yields the name.
	check("issue", "hereIssue", placed{"bug", "login", "existing", "/repo/w/12-login", "existing"})
	// [NEWLANE-3-S3] A branch no template names: no template, and the worktree selected.
	check("[NEWLANE-3-S3]", "hereNone", placed{"", "my-scratch", "existing", "/repo/w/My Scratch", "new,existing,root"})
	check("[NEWLANE-3-S3]", "hereMain", placed{"", "repo", "existing", "/repo", "new,existing,root"})

	// The branch is known only once the issue it names is typed.
	check("issue", "issueMissing", placed{"bug", "login", "new", "", "new"})
	check("issue", "issueGiven", placed{"bug", "login", "existing", "/repo/w/12-login", "existing"})
	// No template: the plan leaves where it runs to the person and the lane type.
	check("plain", "plain", placed{"", "anything", "<none>", "", "new,existing,root"})

	// [NEWLANE-4-S1] A build of add-score-photo, whose proposal has no Approved line: the
	// plan warns that /build-change will stop for it, and still offers the start.
	const unapproved = "add-score-photo has no Approved line in its proposal; /build-change will stop for it"
	for _, k := range []string{"buildUnapproved", "hereUnapproved"} {
		if w := got[k].Warnings; !reflect.DeepEqual(w, []string{unapproved}) {
			t.Errorf("[NEWLANE-4-S1] %s: warnings %q, want %q", k, w, unapproved)
		}
		check("[NEWLANE-4-S1]", k, placed{"build", "add-score-photo", "existing", "/repo/w/add-score-photo", "existing"})
	}
	// [NEWLANE-4-S2] The same build with an Approved line carries no approval warning; a
	// change with no proposal yet gets none either.
	for _, k := range []string{"buildApproved", "buildNoProposal"} {
		if w := got[k].Warnings; len(w) != 0 {
			t.Errorf("[NEWLANE-4-S2] %s: warnings %q, want none", k, w)
		}
	}
	// [NEWLANE-4-S4] A propose lane never gets an approval warning, with no proposal or
	// with one that has no Approved line; nor does a template that only mentions
	// /build-change later in its first prompt.
	for _, k := range []string{"proposeUnapproved", "proposeNoProposal", "mentionUnapproved"} {
		if w := got[k].Warnings; len(w) != 0 {
			t.Errorf("[NEWLANE-4-S4] %s: warnings %q, want none", k, w)
		}
	}
}
