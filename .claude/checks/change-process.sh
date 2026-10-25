#!/bin/sh
# The change process's steps are wired where they run, not only described (AGENTS.md rule 4: a
# delegation target is a named thing). Each line below is a step a skill, a workflow or a lane
# template must carry; removing it from there fails this check. The steps' own behaviour is
# falsified elsewhere (changes.sh, scenarios.sh, change-tools.sh, merge-guard.sh, gate.sh).
. "$(dirname "$0")/lib.sh"
need jq

has() {  # has FILE FIXED-TEXT WHAT
  if grep -qF -- "$2" "$ROOT/$1" 2>/dev/null; then ok "$3 ($1)"; else fail "$1 no longer carries: $3 (looked for '$2')"; fi
}

# The fast path (D5): a one-sentence diff gets no proposal.
has .claude/skills/propose/SKILL.md "## The fast path" "propose states the fast path"
has "$PLAYBOOK" "fast path" "the playbook states the fast path"
for t in fix ops; do
  title=$(jq -r --arg t "$t" '.templates[] | select(.id == $t) | .title' "$ROOT/.clauductor/panel.json" 2>/dev/null)
  case "$title" in *"fast path"*) ok "the $t lane template's title offers the fast path" ;; *) fail "the $t lane template's title does not mention the fast path: '$title'" ;; esac
done

# Scenario IDs (D3), in every place a scenario is written.
for f in changes/README.md specs/README.md .claude/skills/propose/SKILL.md .claude/modules/openspec/config.yaml; do
  has "$f" "[CAP-n-Sn]" "the scenario ID grammar is given"
done

# The log (D6): build-change has the builder keep it and writes Progress per group.
has .claude/workflows/build-change.js '## Decision log' "build-change has the builder keep the Decision log"
has .claude/workflows/build-change.js '## Progress' "build-change writes Progress per committed group"
# The gate it runs is the project's (P1.7): GATE and GATE_QUICK_FLAGS, read ONCE, through
# project-config.sh --json (a second read could replace one with the other; checks/build-change.sh).
has .claude/project-config.sh '--arg quick "$GATE_QUICK_FLAGS"' "project-config.sh --json carries the project's GATE_QUICK_FLAGS"
has .claude/workflows/build-change.js 'GATE_CMD = CFG.gate' "build-change runs the project's GATE (project-config.sh) when no arg names one"
has .claude/workflows/build-change.js 'QUICK = CFG.quickFlags' "build-change runs the project's GATE_QUICK_FLAGS (project-config.sh) when no arg names them"
has .claude/agents/builder.md '## Decision log' "the builder agent keeps the Decision log"

# Verify (D7): in build-change and in merge-pr.
has .claude/workflows/build-change.js 'sh .claude/verify-change.sh' "build-change runs the verify step"
has .claude/skills/merge-pr/SKILL.md 'verify-change' "merge-pr runs the verify step"

# Approval (D8): recorded with the design's hash.
has .claude/skills/propose/SKILL.md 'change-approval.sh <id> --record' "propose records approval over the design hash"
# Review before approval (OPS-76): the rounds are spent before the owner reads it, so a finding does
# not void an approval; merge-pr then reviews only what changed since. This holds the steps in
# place; nothing checks that the review ran or that its PR-body line is true.
has .claude/skills/propose/SKILL.md '2b. **Review it to convergence BEFORE the owner reads it.**' "propose reviews the proposal before the owner reads it"
has .claude/skills/propose/SKILL.md 'an untested scenario is not a finding here' "propose tells its reviewer that tests are the build's, so a proposal can converge"
has .claude/skills/propose/SKILL.md 'end your turn with no final report and without SubagentHandback' "propose waits for its reviewer by ending its turn"
has .claude/skills/propose/SKILL.md '`Review: NOT converged after <n> round(s)' "propose writes a distinct line for a review that did not converge"
has .claude/skills/merge-pr/SKILL.md '`propose` step 2b reviewed it' "merge-pr reviews a proposal PR only since propose's review"
has .claude/skills/merge-pr/SKILL.md 'Only a body line starting `Review: converged in`' "merge-pr skips review only on a converged line"
has .claude/skills/merge-pr/SKILL.md 'the Approved line names that design hash' "merge-pr skips review only when the approval covers the reviewed design"

# merge-pr step 5 (OPS-118 D5): its three branches. The main checkout is the first entry of `git
# worktree list --porcelain`; catch-up is on only when `panel catch-up` exits 0 and prints `on`;
# the pull sits in the branch run from the main checkout itself, after the test that it is. A pull,
# or any other write to the main checkout, that the panel did not make keeps a merged panel.json
# from being trusted on arrival, and a lane cannot write the main checkout. The guard reads the
# WHOLE skill, a heading as text like any other line: one that read only its own cut of step 5
# passed a pull in the step's heading, after a `####` inside it, or in a step of its own. Inside
# step 5, every code span must be on step5_allowed's list FOR THE PART IT SITS IN, and no git or
# gh is named outside one: a list of what may be named, not of pull spellings, since a spelling
# the list lacks (`git -C <dir> merge --ff-only`, a pull after the branches) is a write it would
# pass. A part is where a line SITS, not how it starts: the parts open once each, in order, and a
# line that starts like a part's heading anywhere else is an objection, judged by the part it sits
# in (a bullet shaped like the owner's pull, put before the branches, once got the owner's list).
# Outside step 5, a code span runs only the git and gh commands on merge_pr_cmds' list, none of
# them a pull, and no text outside a span names git with a command that moves a checkout. An edit
# that names something new adds it to its list. Falsified below on copies of the skill.
step5_allowed() {  # every code span step 5 may name, as "<part> <span>"; parts as merge_pr_spans
  cat <<'EOF'
intro worktree
intro git worktree list --porcelain
intro main=$(git worktree list --porcelain | sed -n '1s/^worktree //p')
intro if cu=$(clauductor panel catch-up --project "$main" 2>/dev/null) && [ "$cu" = on ]; then echo on; else echo off; fi
intro on
intro clauductor
intro panel catch-up
on panel.json
on git fetch origin main
on git log -1 --oneline origin/main
on git merge-base --is-ancestor <squash> origin/main
main git rev-parse --path-format=absolute --git-dir
main git rev-parse --path-format=absolute --git-common-dir
main git pull --ff-only origin main
main git log --oneline -3
main main
owner git pull --ff-only
report reportTo
report framework/
report ./install.sh
report clauductor panel install
report .clauductor/panel.json
report clauductor panel trust
EOF
}
merge_pr_cmds() {  # the git and gh commands merge-pr may run outside step 5, none of them a pull
  cat <<'EOF'
git diff
git worktree remove
gh pr create
gh pr view
gh pr checks
gh pr ready
gh pr merge
gh run list
gh auth status
gh api user
EOF
}
# merge_pr_catchup: the one line that may catch the PR's branch up to origin/main outside step 5
# (OPS-124, #214), and its one place: alone between step 2's heading and its first line, where
# merge-pr is in the branch's own checkout and no text before it in the step can aim it elsewhere.
# One fixed line, not a rule about how a merge is aimed: the forms that aim one elsewhere (-C,
# --work-tree, -c core.worktree, GIT_DIR=, env -C, sh -c, a fetch into main:main, a sentence) are
# endless. Anywhere else, or worded otherwise, a merge falls to merge_pr_cmds, which lists no merge
# and no fetch. And no span outside step 5 changes directory or sets GIT_*, so the line cannot
# follow a `cd` or an export in step 1.
merge_pr_catchup() {
  echo 'In the branch'\''s checkout, first catch it up: `git fetch origin main && git merge origin/main`.'
}
# merge_pr_unanchor: the skill on stdin, less the catch-up line where it stands in its one place
# (once, outside a fence, under the first '### 2.' heading only, so a copy of the heading after the
# squash-merge anchors nothing). Renamed step 2, the line stays in and is refused: the strip fails
# closed. The line names its checkout itself, since merge-pr may start in the main checkout.
merge_pr_unanchor() {
  awk -v line="$(merge_pr_catchup)" '
    { l[NR] = $0 }
    END {
      for (i = 1; i <= NR; i++) {
        if (l[i] ~ /^```/) fence = !fence
        if (!fence && l[i] ~ /^### 2\. /) h2++
        if (!fence && !done && h2 == 1 && l[i] == line && l[i - 1] == "### 2. Gate evidence for the head commit (blocking)" &&
            index(l[i + 1], "Run the full gate from the branch\047s checkout") == 1) { done = 1; continue }
        print l[i]
      }
    }'
}
# merge_pr_spans: reads the whole skill, every line of it, and prints "L <line>" for each line of
# step 5 (from its '### 5.' heading to the next heading of level 1 to 3, fences skipped), and:
# inside step 5, "S <part> <span>" for each code span, in order (a fenced line is one span; an
# inline span may wrap), "P <part>" for each stretch of text outside spans that names git or gh,
# "E <part>" where a heading opens a part, and "X <part>" for a line that starts like that part's
# heading where it may not open; outside it, "W <part> <command>" for a command of a code span
# that names git or gh with a command merge_pr_cmds does not list, or starts with a command that
# moves a checkout (`pull`, `reset`, ... with no git before it), "Q <part> git <command>" for text
# outside spans that names git with such a command, and "X step5" for a second '### 5.' heading.
# The parts outside step 5 are before and after it. Inside it they open by POSITION, once each and
# in this order: intro (its heading, then the text before the branches), on, main and lane (the
# three branch bullets, one after another), report (from '**The owner's step, in your report.**'
# on) and owner (the pull bullet, only as the first bullet of report). An unindented line ends a
# branch, to rest (text between or after the branches, which may name nothing), and ends the
# owner's bullet, back to report; a `####` heading is such a line. A blank line ends neither: a
# list item may go on after one.
merge_pr_spans() {
  awk -v cmds="$(merge_pr_cmds | tr '\n' ',')" '
    function outside() { return part == "before" || part == "after" }
    function prose(s,   w) {
      if (outside()) { if ((w = pwrite(s)) != "") print "Q " part " " w; return }
      if ((" " s " ") ~ /[^A-Za-z0-9_.\/-](git|gh)[^A-Za-z0-9_-]/) print "P " part
    }
    function span(s,   w) {
      if (outside()) {
        if ((w = cwrite(s)) != "") print "W " part " " w
        return
      }
      print "S " part " " s
    }
    function flush(   t, i, r, j) {
      t = buf; buf = ""
      while ((i = index(t, "`")) > 0) {
        r = substr(t, i + 1); j = index(r, "`")
        if (j == 0) break
        prose(substr(t, 1, i - 1)); span(substr(r, 1, j - 1)); t = substr(r, j + 1)
      }
      prose(t)
    }
    function trim(s) { sub(/^[ \t]+/, "", s); sub(/[ \t]+$/, "", s); return s }
    # skip(t, j, m): the first word from j on that is not an option of git itself (-C <dir>, ...)
    function skip(t, j, m) {
      for (; j <= m && t[j] ~ /^-/; j++) if (t[j] ~ /^(-C|-c|--git-dir|--work-tree|--namespace)$/) j++
      return j
    }
    # cwrite(s): the first command of code span s that merge_pr_cmds does not list, one that
    # changes directory (a word cd or pushd anywhere, quoted, escaped, after then or assigned, as
    # in sh -c or c=cd), one named by a variable ($c), or one that sets GIT_* (an export too), or ""
    function cwrite(s,   n, c, k, m, t, i, j, cmd, w) {
      n = split(s, c, /\$\(|[|;&()]+/)
      for (k = 1; k <= n; k++) {
        if ((m = split(trim(c[k]), t, /[ \t]+/)) == 0) continue
        if (t[1] in moves || t[1] ~ /^["\047]?\$/) return trim(c[k])
        for (i = 1; i <= m; i++) {
          w = t[i]; gsub(/["\047\\]/, "", w)
          if (w ~ /(^|=)(cd|pushd)$/ || w ~ /^GIT_[A-Z_]*=/) return trim(c[k])
          if (t[i] == "git" && (j = skip(t, i + 1, m)) <= m) {
            cmd = "git " t[j]
            if (t[j] == "worktree") cmd = cmd " " t[j + 1]
            if (!(cmd in ok)) return trim(c[k])
          }
          if (t[i] == "gh" && i < m && !(("gh " t[i + 1] " " t[i + 2]) in ok)) return trim(c[k])
        }
      }
      return ""
    }
    # pwrite(s): "git <command>" where text s names git with a command that moves a checkout
    function pwrite(s,   m, t, i, j) {
      m = split(s, t, /[^A-Za-z0-9_-]+/)
      for (i = 1; i <= m; i++)
        if (t[i] == "git" && (j = skip(t, i + 1, m)) <= m && (t[j] in moves)) return "git " t[j]
      return ""
    }
    BEGIN {
      part = "before"; where = 0  # where: 0 before step 5, 1 in it, 2 after it
      stage = 0  # how many of the headings below have opened their part
      head[1] = "- **Catch-up on:**"; name[1] = "on"
      head[2] = "- **Catch-up off, run from the main checkout itself**"; name[2] = "main"
      head[3] = "- **Catch-up off, from a lane:**"; name[3] = "lane"
      head[4] = "**The owner\047s step, in your report.**"; name[4] = "report"
      head[5] = "- `git pull --ff-only`, unless step 5 pulled"; name[5] = "owner"
      n = split(cmds, c, ","); for (i = 1; i <= n; i++) if (c[i] != "") ok[c[i]] = 1
      n = split("pull merge reset checkout switch rebase restore stash update-ref", c, " ")
      for (i = 1; i <= n; i++) moves[c[i]] = 1
    }
    /^```/ { flush(); fence = !fence; if (where == 1) print "L " $0; next }
    fence { if (where == 1) print "L " $0; span($0); next }
    /^### 5\. / && where == 0 {
      flush(); where = 1; part = "intro"; print "E intro"; print "L " $0; buf = " " $0; flush(); next
    }
    /^##?#?$|^##?#? / && where == 1 { flush(); where = 2; part = "after" }
    /^### 5\. / { print "X step5" }
    where != 1 {
      if ($0 == "" || $0 ~ /^#/ || $0 ~ /^ *([-*]|[0-9]+\.) /) flush()
      buf = buf " " $0
      if ($0 ~ /^#/) flush()
      next
    }
    {
      print "L " $0
      np = part; k = 0; opened = 0
      for (i = 1; i <= 5; i++) if (index($0, head[i]) == 1) k = i
      if (k && k == stage + 1 && !(k == 5 && bullet)) { np = name[k]; stage = k; opened = 1 }
      else {
        if (k) print "X " name[k]
        if ($0 != "" && $0 !~ /^ /) {
          if (np == "on" || np == "main" || np == "lane") np = "rest"
          else if (np == "owner") np = "report"
        }
      }
      if (stage == 4 && index($0, "- ") == 1) bullet = 1
      if (np != part || $0 == "") flush()
      if (opened) print "E " np
      part = np; sub(/^ +/, ""); buf = buf " " $0
    }
    END { flush() }'
}
step5_part() {  # step5_part PART: the part as an objection names it
  case $1 in
    intro) echo "the text before the branches" ;;
    on) echo "the branch 'Catch-up on:'" ;;
    main) echo "the branch 'Catch-up off, run from the main checkout itself'" ;;
    lane) echo "the branch 'Catch-up off, from a lane:'" ;;
    report) echo "the owner's step outside its pull bullet" ;;
    owner) echo "the owner's pull bullet" ;;
    before) echo "the text before step 5" ;;
    after) echo "the text after step 5" ;;
    *) echo "the text between or after the branches" ;;
  esac
}
step5() {  # step5 SKILL.md: prints each rule step 5 breaks, one per line; nothing when it holds
  _rec=$(merge_pr_unanchor < "$1" | merge_pr_spans)
  _s=$(printf '%s\n' "$_rec" | sed -n 's/^L //p')
  [ -n "$_s" ] || { echo "it has no step 5"; return; }
  printf '%s\n' "$_s" | grep -qF 'git worktree list --porcelain' ||
    echo "it no longer finds the main checkout from git worktree list --porcelain"
  printf '%s\n' "$_s" | grep -qF 'clauductor panel catch-up --project' ||
    echo "it no longer asks clauductor panel catch-up --project"
  printf '%s\n' "$_s" | grep -qF 'exits 0 and prints `on`' ||
    echo "it no longer counts catch-up on only when the command exits 0 and prints on"
  for _b in 'on Catch-up on:' 'main Catch-up off, run from the main checkout itself' \
    'lane Catch-up off, from a lane:'; do
    printf '%s\n' "$_rec" | grep -qxF "E ${_b%% *}" ||
      echo "it has no branch '- **${_b#* }**' in its place (the three come once each, in this order)"
  done
  _allow=$(step5_allowed)
  printf '%s\n' "$_rec" | while IFS= read -r _r; do
    case $_r in
      'S '*)
        _ps=${_r#S }
        printf '%s\n' "$_allow" | grep -qxF -- "$_ps" ||
          printf '%s\n' "$(step5_part "${_ps%% *}") names \`${_ps#* }\`, which its list does not allow there"
        ;;
      'P '*) echo "$(step5_part "${_r#P }") names git or gh outside a code span" ;;
      'W '*)
        _w=${_r#W }
        printf '%s\n' "$(step5_part "${_w%% *}") names \`${_w#* }\`, which merge_pr_cmds does not list (outside step 5, merge-pr runs only what it lists)"
        ;;
      'Q '*)
        _q=${_r#Q }
        printf '%s\n' "$(step5_part "${_q%% *}") names \`${_q#* }\` outside a code span, a git command that moves a checkout"
        ;;
      'X step5') echo "a second '### 5.' heading: step 5 comes once" ;;
      'X owner') echo "a line starts like the owner's pull bullet but is not the first bullet of the owner's step" ;;
      'X report') echo "a second '**The owner's step, in your report.**', or one before the three branches" ;;
      'X '*) echo "$(step5_part "${_r#X }") comes again, or out of order (the three come once each, in this order)" ;;
    esac
  done
  _pull=$(printf '%s\n' "$_rec" | grep -nxF 'S main git pull --ff-only origin main' | head -n 1 | cut -d: -f1)
  if [ -z "$_pull" ]; then
    echo "it no longer pulls (git pull --ff-only origin main) from the main checkout with catch-up off"
  else
    for _t in --git-dir --git-common-dir; do
      _test=$(printf '%s\n' "$_rec" | grep -nxF "S main git rev-parse --path-format=absolute $_t" | head -n 1 | cut -d: -f1)
      if [ -z "$_test" ] || [ "$_test" -gt "$_pull" ]; then
        echo "a pull --ff-only comes before the $_t test that this is the main checkout"
      fi
    done
  fi
}
_skill="$ROOT/.claude/skills/merge-pr/SKILL.md"
_why=$(step5 "$_skill")
if [ -z "$_why" ]; then
  ok "merge-pr pulls the main checkout only in step 5, from the main checkout itself, and only with catch-up off (skills/merge-pr/SKILL.md, read whole)"
else
  while IFS= read -r _l; do fail "merge-pr: $_l (skills/merge-pr/SKILL.md)"; done <<EOF
$_why
EOF
fi
# Each rule turns red, alone, on a copy of the skill that breaks it and nothing else: a rule
# no copy turns red checks nothing.
_d=$(scratch)
_n=0
_red() {  # _red WHAT SED-SCRIPT OBJECTION: step5 on the edited copy says OBJECTION
  _n=$((_n + 1))
  sed "$2" "$_skill" > "$_d/step5-$_n.md"
  if cmp -s "$_skill" "$_d/step5-$_n.md"; then fail "step 5's falsification '$1' edits nothing: re-anchor it"
  elif step5 "$_d/step5-$_n.md" | grep -qF -- "$3"; then ok "step 5's check catches $1"
  else fail "step 5's check passes $1 (looked for: $3)"; fi
}
_red "a root not from the porcelain list" 's/git worktree list --porcelain/git worktree list/g' \
  "no longer finds the main checkout"
_red "a catch-up question without the project" \
  's/clauductor panel catch-up --project/clauductor panel catch-up/g' "no longer asks clauductor panel"
_red "a looser test of on" 's/exits 0 and prints `on`/prints `on`/' "exits 0 and prints on"
_red "a branch gone" 's/^- \*\*Catch-up off, from a lane:\*\*/- **From a lane:**/' \
  "has no branch '- **Catch-up off, from a lane:**'"
_red "no pull from the main checkout" 's/: `git pull --ff-only origin main`, then confirm/: confirm/' \
  "it no longer pulls"
_red "a pull before the test of the main checkout" \
  's/^- \*\*Catch-up off, run from the main checkout itself\*\* /&`git pull --ff-only origin main` first, /' \
  "comes before the --git-common-dir test"
_red "a pull while catch-up is on" 's/^- \*\*Catch-up on:\*\* /&run `git pull origin main`, then /' \
  "branch 'Catch-up on:' names \`git pull origin main\`"
_red "a pull from a lane" 's/^- \*\*Catch-up off, from a lane:\*\* /&`git -C "$main" pull`, then /' \
  "branch 'Catch-up off, from a lane:' names \`git -C \"\$main\" pull\`"
# The spellings the list of pull commands this replaced let through: they fast-forward the main
# checkout before the panel's poll, so the merged panel.json is never trusted on arrival.
_red "the old placeholder pull while catch-up is on" \
  's/^- \*\*Catch-up on:\*\* /&run `git -C <main checkout> pull`, then /' \
  "branch 'Catch-up on:' names \`git -C <main checkout> pull\`"
_red "a fast-forward by merge while catch-up is on" \
  's/^- \*\*Catch-up on:\*\* /&run `git -C "$main" merge --ff-only origin\/main`, then /' \
  "branch 'Catch-up on:' names \`git -C \"\$main\" merge --ff-only origin/main\`"
_red "the allowed pull after the branches" '/^\*\*The owner.s step, in your report\.\*\*/i\
Then, whatever the branch, `git pull --ff-only origin main`.
' "between or after the branches names \`git pull --ff-only origin main\`"
_red "a pull named outside a code span" \
  's/^- \*\*Catch-up off, from a lane:\*\* /&run git pull in the main checkout, then /' \
  "branch 'Catch-up off, from a lane:' names git or gh outside a code span"
# A part is where a line sits (review round 2): these copies start a line like a part's heading
# somewhere that part does not open, and a part chosen by how a line starts let each one through.
_red "a bullet shaped like the owner's pull before the branches" '/^- \*\*Catch-up on:\*\*/i\
- `git pull --ff-only`, unless step 5 pulled already: run it first, whatever the branch.
' "text before the branches names \`git pull --ff-only\`"
_red "a bullet shaped like the owner's pull between the branches" \
  '/^- \*\*Catch-up off, run from the main checkout itself\*\*/i\
- `git pull --ff-only`, unless step 5 pulled: with catch-up on, run it in the main checkout too.
' "between or after the branches names \`git pull --ff-only\`"
_red "the main-checkout branch again after the owner's list" \
  '/^- after a change to `\.clauductor\/panel\.json`/a\
- **Catch-up off, run from the main checkout itself** again: `git pull --ff-only origin main`.
' "owner's step outside its pull bullet names \`git pull --ff-only origin main\`"
_red "a branch heading twice, naming nothing" '/^\*\*The owner.s step, in your report\.\*\*/i\
- **Catch-up on:** leave the main checkout to the panel.
' "branch 'Catch-up on:' comes again, or out of order"
_red "the owner's step heading twice" '/^With catch-up on, the report names none/i\
**The owner'\''s step, in your report.** Once more:
' "a second '**The owner's step, in your report.**'"
_red "a bullet before the owner's pull" '/^- `git pull --ff-only`, unless step 5 pulled/i\
- what merged, and where;
' "starts like the owner's pull bullet but is not the first bullet"
# The whole skill is read (review run 3): each of these passed a guard that read only its own cut
# of step 5, from the line after the heading to the next line starting '##'.
_red "a pull in step 5's heading" 's/^### 5\. Sync$/& (`git pull --ff-only origin main`)/' \
  "text before the branches names \`git pull --ff-only origin main\`"
_red "a pull after a #### inside step 5" '/^## After the merge/i\
#### By hand\
Then `git pull --ff-only origin main` from the main checkout.
' "owner's step outside its pull bullet names \`git pull --ff-only origin main\`"
_red "a pull in the step before step 5" '/^### 5\. Sync/i\
Then `git pull --ff-only origin main` in the main checkout.
' "text before step 5 names \`git pull --ff-only origin main\`"
_red "a pull in a step of its own after step 5" '/^## After the merge/i\
### 6. Pull\
Run `git -C "$main" pull --ff-only` once the merge lands.
' "text after step 5 names \`git -C \"\$main\" pull --ff-only\`"
_red "a fast-forward by merge after the merge" '/^- Leave the roadmap row/i\
- Fast-forward the main checkout: `git -C "$main" merge origin/main`.
' "text after step 5 names \`git -C \"\$main\" merge origin/main\`"
_red "a reset with no git before it" '/^- Leave the roadmap row/i\
- Then `reset --hard origin/main` in the main checkout.
' "text after step 5 names \`reset --hard origin/main\`"
_red "a sync by gh" '/^- Leave the roadmap row/i\
- Bring the main checkout up to date with `gh repo sync`.
' "text after step 5 names \`gh repo sync\`"
_red "a pull named outside a code span after step 5" '/^- Leave the roadmap row/i\
- Then run git pull in the main checkout.
' "text after step 5 names \`git pull\` outside a code span"
_red "a second step 5" '/^## After the merge/i\
### 5. Sync again
' "a second '### 5.' heading"
# A legitimate edit passes (OPS-124, #214): a rule that refuses every edit to merge-pr is no
# guard either. Catching the lane's own branch up to origin/main, by merge_pr_catchup's line in its
# place, is not a write to the main checkout; the same merge aimed at the main checkout stays red.
_green() {  # _green WHAT SED-SCRIPT: step5 on the edited copy objects to nothing
  _n=$((_n + 1))
  sed "$2" "$_skill" > "$_d/step5-$_n.md"
  _o=$(step5 "$_d/step5-$_n.md")
  if cmp -s "$_skill" "$_d/step5-$_n.md"; then fail "step 5's green case '$1' edits nothing: re-anchor it"
  elif [ -z "$_o" ]; then ok "step 5's check passes $1"
  else fail "step 5's check refuses $1: $(printf '%s' "$_o" | head -n 1)"; fi
}
_green "catching the lane branch up before the gate" '/^Run the full gate from the branch.s checkout/i\
In the branch'\''s checkout, first catch it up: `git fetch origin main && git merge origin/main`.
'
# Review rounds 1 and 2 (OPS-124): a rule that read how a merge was aimed, then a list of catch-up
# spans allowed anywhere before step 5, let each of these through. The catch-up is one line in
# one place; worded otherwise or placed otherwise it is refused, and so is a cd or an export
# before it in its place.
_catch='/^Run the full gate from the branch.s checkout/i\
In the branch'\''s checkout, first catch it up: `git fetch origin main && git merge origin/main`.
'
_red "the catch-up line after the squash-merge" '/^### 5\. Sync/i\
In the branch'\''s checkout, first catch it up: `git fetch origin main && git merge origin/main`.
' "text before step 5 names \`git fetch origin main\`"
_red "the catch-up line aimed by the sentence before it" '/^Run the full gate from the branch.s checkout/i\
In the main checkout, catch main up.\
In the branch'\''s checkout, first catch it up: `git fetch origin main && git merge origin/main`.
' "text before step 5 names \`git fetch origin main\`"
_red "the catch-up line aimed by the sentence after it" '/^Run the full gate from the branch.s checkout/i\
In the branch'\''s checkout, first catch it up: `git fetch origin main && git merge origin/main`.\
Do that in the main checkout.
' "text before step 5 names \`git fetch origin main\`"
_red "the catch-up line under a second step 2 heading after the squash-merge" '/^### 5\. Sync/i\
### 2. Gate evidence for the head commit (blocking)\
In the branch'\''s checkout, first catch it up: `git fetch origin main && git merge origin/main`.\
Run the full gate from the branch'\''s checkout again.
' "text before step 5 names \`git fetch origin main\`"
_red "the catch-up in a fenced block" '/^Run the full gate from the branch.s checkout/i\
```bash\
git fetch origin main\
git merge --no-edit origin/main\
```
' "text before step 5 names \`git fetch origin main\`"
_red "an export of GIT_WORK_TREE before the catch-up line" "$_catch"'/^Use the argument if given/i\
```bash\
export GIT_WORK_TREE="$main" GIT_DIR="$main/.git"\
```
' "text before step 5 names \`export GIT_WORK_TREE=\"\$main\" GIT_DIR=\"\$main/.git\"\`"
_red "a quoted cd before the catch-up line" "$_catch"'/^Use the argument if given/i\
Run `"cd" "$main"`.
' "text before step 5 names \`\"cd\" \"\$main\"\`"
_red "an escaped cd before the catch-up line" "$_catch"'/^Use the argument if given/i\
Run `\\cd "$main"`.
' 'cd "$main"`, which merge_pr_cmds does not list'
_red "a cd by a variable before the catch-up line" "$_catch"'/^Use the argument if given/i\
Run `c=cd; $c "$main"`.
' "text before step 5 names \`c=cd\`"
_red "a command named by a variable before the catch-up line" "$_catch"'/^Use the argument if given/i\
Run `"$c" "$main"`.
' "text before step 5 names \`\"\$c\" \"\$main\"\`"
# Merge-pr runs after the squash-merge from the main checkout too, so a bare merge there
# fast-forwards it.
_red "a bare merge after the merge" '/^- Leave the roadmap row/i\
- Then `git merge origin/main`.
' "text after step 5 names \`git merge origin/main\`"
_red "a merge into the main checkout by cd" '/^- Leave the roadmap row/i\
- Then `cd "$main" && git merge origin/main`.
' "text after step 5 names \`cd \"\$main\"\`"
_red "a cd on its own line of a fenced block before the catch-up merge" '/^Run the full gate from the branch.s checkout/i\
```bash\
cd "$main"\
git merge --no-edit origin/main\
```
' "text before step 5 names \`cd \"\$main\"\`"
_red "a cd in a span of its own before the catch-up merge" '/^Run the full gate from the branch.s checkout/i\
Run `cd "$main"`, then `git merge origin/main`.
' "text before step 5 names \`cd \"\$main\"\`"
_red "a cd inside a then" '/^Run the full gate from the branch.s checkout/i\
Run `if true; then cd "$main"; fi`, then `git merge origin/main`.
' "text before step 5 names \`then cd \"\$main\"\`"
_red "a merge into the main checkout by sh -c" '/^Run the full gate from the branch.s checkout/i\
Run `sh -c '\''cd "$main" \&\& git merge origin/main'\''`.
' "text before step 5 names \`sh -c 'cd \"\$main\"\`"
_red "a merge into the main checkout by --work-tree" '/^Run the full gate from the branch.s checkout/i\
Run `git --work-tree="$main" merge origin/main`.
' "text before step 5 names \`git --work-tree=\"\$main\" merge origin/main\`"
_red "a merge into the main checkout by GIT_WORK_TREE" '/^Run the full gate from the branch.s checkout/i\
Run `GIT_WORK_TREE="$main" git merge origin/main`.
' "text before step 5 names \`GIT_WORK_TREE=\"\$main\" git merge origin/main\`"
_red "a merge into the main checkout by -c core.worktree" '/^Run the full gate from the branch.s checkout/i\
Run `git -c core.worktree="$main" merge origin/main`.
' "text before step 5 names \`git -c core.worktree=\"\$main\" merge origin/main\`"
_red "a merge into the main checkout by env -C" '/^Run the full gate from the branch.s checkout/i\
Run `env -C "$main" git merge origin/main`.
' "text before step 5 names \`env -C \"\$main\" git merge origin/main\`"
_red "a fetch that moves main" '/^Run the full gate from the branch.s checkout/i\
Run `git fetch --update-head-ok origin main:main`.
' "text before step 5 names \`git fetch --update-head-ok origin main:main\`"

# Budget, outcome and provenance at archive and merge (items 12–14).
has .claude/skills/archive-change/SKILL.md 'change-cost.sh' "archive records the actual cost"
has .claude/skills/archive-change/SKILL.md 'check-outcome-<id>' "archive queues the outcome check"
has .claude/skills/merge-pr/SKILL.md 'Agent-Role:' "merge-pr writes the provenance trailers"
# A row's PR opens as a draft (session-start/marker.sh), and GitHub refuses to squash a draft.
has .claude/skills/merge-pr/SKILL.md 'run `gh pr ready <n>` first' "merge-pr readies a draft PR before the squash"
has .claude/skills/merge-pr/context.sh 'isDraft' "merge-pr's context says when the PR is a draft"
has .claude/workflows/build-change.js "stop('budget'" "build-change stops over budget"

# The reviewer checks that a citing test asserts the THEN (D4).
has .claude/agents/reviewer.md 'assert the THEN' "the reviewer checks that each citing test asserts the THEN"
# The reviewer names each finding's class in its own field (PRAC-0010, OPS-97); with no schema to
# fill, this paragraph is the only place the instruction lives.
has .claude/agents/reviewer.md '`class` field, never in its `summary`' "the reviewer names each finding's class in its class field"
finish
