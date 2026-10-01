#!/bin/sh
# The artifacts module (.claude/modules/artifacts), tested whether a project turns it on or not:
# its currency tool, its guard rule through the real pr-merge-guard.sh, its publishing half and its
# wiring, each against a throwaway repository, never against its own source. Ported from Standing
# Tee's vitest cases (artifact-currency, pr-merge-guard-currency, open-artifacts, artifact-health),
# and falsified in BOTH directions: each red case has its mirror that must stay green for the
# control to be usable at all, and the comment above a case names the mutation that turned it red.
. "$(dirname "$0")/lib.sh"
need git jq

MOD="$ROOT/.claude/modules/artifacts"
d=$(scratch)
R="$d/app"; new_repo "$R"
mkdir -p "$R/.claude/lib" "$R/.claude/modules"
cp "$ROOT/.claude/lib/conf.sh" "$ROOT/.claude/lib/modules.sh" "$R/.claude/lib/"
cp -R "$MOD" "$R/.claude/modules/artifacts"
CUR="$R/.claude/modules/artifacts/bin/currency.sh"
cur() { (cd "$R" && sh "$CUR" "$@") 2>&1; }
w() { mkdir -p "$(dirname "$R/$1")"; printf '%s\n' "$2" > "$R/$1"; }
commit() { git -C "$R" add -A && git -C "$R" commit -qm "$1"; }
line_for() { printf '%s\n' "$1" | grep -F " $2 " | head -n 1; }
has() {  # has WANT(yes|no) TEXT HAYSTACK LABEL
  case "$3" in *"$2"*) got=yes ;; *) got=no ;; esac
  [ "$got" = "$1" ] && ok "$4" || fail "$4 (looked for '$2'): $(printf '%s' "$3" | head -c 600)"
}
rc_of() { (cd "$R" && sh "$CUR" "$@") >/dev/null 2>&1; echo $?; }
roadmap() {  # roadmap STATUS1 STATUS2
  printf '## Phase 2 — Domain core\n**Owner:** Rich\n#### Gate 2D — the app\n| # | Change | Scope | Deps | Status |\n|---|---|---|---|---|\n| 2D.1 | **`add-shell`** — the shell | scope | — | %s |\n| 2D.2 | **`add-push`** — push | scope | 2D.1 | %s |\n' "$1" "$2" > "$R/docs/roadmap.md"
}

# ── currency: BEHIND when an authority moves, OK when anything else does ──────────────────────
w docs/page.html '<p>page</p>'
w docs/list.html '<p>lists the artifacts</p>'
w src/a.md 'a'
w src/sub/b.md 'b'
w other/unrelated.txt 'x'
roadmap '⬜ queued' '⬜ queued'
cat > "$R/docs/artifacts.json" <<'EOF'
{
  "pages": {
    "docs/page.html": {
      "url": "https://claude.ai/artifact/AAAAAAAAAAAAAAAAAAAAAA",
      "published": "0000000000000000000000000000000000000000",
      "authorities": ["src/**/*.md", "row:2D.1"]
    },
    "docs/list.html": {
      "url": "https://claude.ai/artifact/BBBBBBBBBBBBBBBBBBBBBB",
      "published": "1111111111111111111111111111111111111111",
      "authorities": ["registry:entries"]
    }
  }
}
EOF
git -C "$R" add -A   # a stamp counts TRACKED files only, as a commit does
out=$(cur --stamp docs/page.html docs/list.html --note "first review")
has yes "stamped docs/page.html = " "$out" "stamp: records the review and says what it hashed"
commit start

# Falsified by comparing against a constant instead of the stamp: red here, and every BEHIND case.
out=$(cur --ref HEAD --check); rc=$?
expect_rc 0 "$rc" "currency: OK right after a stamp (--check exits 0)"
has yes "Core artifacts vs HEAD @ $(git -C "$R" rev-parse --short=9 HEAD)" "$out" "currency: names the tree it read (subject and commit)"
case $(line_for "$out" docs/page.html) in "OK      docs/page.html — reviewed $(date +%Y-%m-%d) (first review)") ok "currency: the OK line carries the review's date and note" ;; *) fail "OK line: $(line_for "$out" docs/page.html)" ;; esac
has yes "All 2 core artifacts are current" "$out" "currency: the summary counts every artifact"

# Falsified by hashing only the first glob match: the edit to sub/b.md went unseen (red).
w src/sub/b.md 'b, edited'; commit "Edit the b runbook"
out=$(cur --ref HEAD --check); rc=$?
expect_rc 1 "$rc" "currency: an authority file changing fails --check"
l=$(line_for "$out" docs/page.html)
has yes "BEHIND  docs/page.html" "$l" "currency: ...and reads BEHIND"
has yes "src/**/*.md (sub/b.md)" "$l" "currency: ...naming the glob and the file, relative to the glob's base"
has yes '1 commit(s) since the review, latest' "$l" "currency: ...counting the commits since the review"
has yes '"Edit the b runbook"' "$l" "currency: ...naming the commit that moved it"
has yes "--stamp docs/page.html --note" "$out" "currency: ...and the command that clears it"
git -C "$R" reset -q --hard HEAD~1

# MIRROR. Falsified by hashing every tracked file: red (every close blocked by noise).
w other/unrelated.txt 'y'; w docs/page.html '<p>the page itself, edited</p>'; commit "Unrelated work"
expect_rc 0 "$(rc_of --ref HEAD --check)" "currency MIRROR: an unrelated file (and the page itself) changing leaves it OK"

# Falsified by hashing the whole roadmap file for row:: red on the 2D.2 edit.
roadmap '⬜ queued' '✅ merged (#9)'; commit "Merge the push row"
expect_rc 0 "$(rc_of --ref HEAD --check)" "currency MIRROR: a DIFFERENT roadmap row changing leaves it OK"
roadmap '⬜ in flight (#8)' '✅ merged (#9)'; commit "The shell row goes in flight"
out=$(cur --ref HEAD --check); rc=$?
expect_rc 1 "$rc" "currency: a cited roadmap row changing reads BEHIND"
l=$(line_for "$out" docs/page.html)
has yes "row:2D.1" "$l" "currency: ...naming the row"
has yes "1 commit(s) since the review" "$l" "currency: ...counting only the commit that changed THIS row"
has yes '"The shell row goes in flight"' "$l" "currency: ...and naming it"
out=$(cur --stamp docs/page.html --note "refreshed: the shell row"); commit "Stamp the page"

# registry:entries: moves on a new artifact, never on a publish hash or a stamp (else stamping one
# page would un-stamp the page that lists them, forever). Falsified by hashing the whole registry: red.
jq --indent 2 '.pages["docs/page.html"].published = "2222222222222222222222222222222222222222"' "$R/docs/artifacts.json" > "$d/r" && cat "$d/r" > "$R/docs/artifacts.json"
commit "Record a publish"
cur --stamp docs/page.html --note "restamped" >/dev/null; commit "Restamp the page"
case $(line_for "$(cur --ref HEAD)" docs/list.html) in OK*) ok "currency MIRROR: registry:entries holds through a publish hash and a stamp" ;; *) fail "registry:entries moved on a stamp: $(line_for "$(cur --ref HEAD)" docs/list.html)" ;; esac
jq --indent 2 '.walkthroughs = {"go-live": {url: "https://claude.ai/artifact/CCCCCCCCCCCCCCCCCCCCCC", authorities: ["src/a.md"]}}' "$R/docs/artifacts.json" > "$d/r" && cat "$d/r" > "$R/docs/artifacts.json"
commit "Register a walkthrough"
out=$(cur --ref HEAD)
case $(line_for "$out" docs/list.html) in BEHIND*registry:entries*) ok "currency: registry:entries moves when an artifact is registered" ;; *) fail "list.html: $(line_for "$out" docs/list.html)" ;; esac
case $(line_for "$out" go-live) in "CANNOT CHECK go-live — has no 40-hex \`reviewedAt\`"*) ok "currency: a new, unstamped entry is CANNOT CHECK, never quietly OK" ;; *) fail "go-live: $(line_for "$out" go-live)" ;; esac
cur --stamp docs/list.html go-live --note "seed" >/dev/null; commit "Stamp the new entries"

# A stamp written on a branch survives the SQUASH merge, the only way anything lands. Falsified by
# stamping the commit id instead of the content: red.
git -C "$R" checkout -q -b ops/session-9-close
w src/a.md 'a, refreshed'
cur --stamp docs/page.html go-live --note "refreshed for a" >/dev/null
commit "close: refresh and stamp"
git -C "$R" checkout -q main
git -C "$R" merge --squash -q ops/session-9-close >/dev/null 2>&1
commit "Session 9 close (#99)"
git -C "$R" branch -qD ops/session-9-close
expect_rc 0 "$(rc_of --ref main --check)" "currency: a stamp written on a branch still matches after a squash merge"

# --worktree: the close's own tree, uncommitted edits first. Falsified by reading HEAD: red.
w src/a.md 'a, being edited'
out=$(cur --worktree --check); rc=$?
expect_rc 1 "$rc" "currency --worktree: an uncommitted authority edit fails --check"
has yes "uncommitted: src/**/*.md (a.md)" "$(line_for "$out" docs/page.html)" "currency --worktree: ...named as uncommitted, with the file"
has yes "Core artifacts vs the working tree" "$out" "currency --worktree: ...naming the working tree as its subject"
cur --stamp docs/page.html go-live --note "reviewed, no change: a typo fix" >/dev/null
expect_rc 0 "$(rc_of --worktree --check)" "currency --worktree: clears once stamped"
git -C "$R" add -A && git -C "$R" commit -qm "typo fix, stamped"

# Untracked files are named, never hashed: rule 8 reads a COMMIT, which holds none, and a stamp
# that hashed one would never match any commit (the close loops). Other agents' worktrees and build
# output are never listed (git ls-files, not the directory). Falsified by hashing the raw walk: red.
before=$(jq -r '.pages["docs/page.html"].reviewedAt' "$R/docs/artifacts.json")
w src/stray.md 'an untracked scratch note'
w src/ignored.md 'ignored'
printf 'src/ignored.md\n' > "$R/.gitignore"; git -C "$R" add .gitignore; git -C "$R" commit -qm ignore
out=$(cur --worktree --check); rc=$?
expect_rc 0 "$rc" "currency --worktree: an untracked file under a glob changes neither the verdict..."
has yes "untracked, so not counted" "$out" "currency --worktree: ...and is named"
has yes "src/stray.md" "$out" "currency --worktree: ...by path"
has no "src/ignored.md" "$out" "currency --worktree: an IGNORED file is not even named"
s=$(cur --stamp docs/page.html --note "restamp with a stray file present")
has yes "src/stray.md is untracked and NOT in this stamp" "$s" "stamp: names the untracked file it left out"
[ "$(jq -r '.pages["docs/page.html"].reviewedAt' "$R/docs/artifacts.json")" = "$before" ] && ok "stamp: ...and the stamp is unchanged by it" || fail "stamp: an untracked file changed the stamp"
rm -f "$R/src/stray.md" "$R/src/ignored.md"
git -C "$R" checkout -q docs/artifacts.json

# Every way to learn nothing is loud. Falsified by skipping an authority that matches nothing: red.
jq --indent 2 '.pages["docs/page.html"].authorities = ["src/*.mdx", "row:2D.9"]' "$R/docs/artifacts.json" > "$d/r" && cat "$d/r" > "$R/docs/artifacts.json"
commit "Typo the authorities"
out=$(cur --ref HEAD --check); rc=$?
expect_rc 1 "$rc" "currency: a glob matching nothing and a missing row fail --check"
l=$(line_for "$out" docs/page.html)
has yes "CANNOT CHECK" "$l" "currency: ...as CANNOT CHECK, never OK"
has yes "src/*.mdx matches no file" "$l" "currency: ...naming the glob"
has yes "row:2D.9 names no row" "$l" "currency: ...and the row"
expect_rc 1 "$(rc_of --lint)" "currency --lint: fails on the same declarations"
o=$(cur --stamp docs/page.html); has yes "needs --note" "$o" "stamp: refuses with no note"
expect_rc 2 "$(rc_of --stamp docs/page.html --note x)" "stamp: refuses a glob that matches nothing (exit 2)"
has yes "src/*.mdx matches no file" "$(cur --stamp docs/page.html --note x)" "stamp: ...and says which"
git -C "$R" reset -q --hard HEAD~1
expect_rc 0 "$(rc_of --lint)" "currency --lint MIRROR: passes on good declarations"
o=$(cur --ref no-such-ref); has yes "cannot resolve no-such-ref" "$o" "currency: an unknown revision is an error, never an empty report"
expect_rc 2 "$(rc_of --worktree --ref HEAD)" "currency: --worktree with --ref is refused, not guessed at"

# The health line (session-start's reader): no origin/main is CANNOT CHECK, exit 0, never OK.
H="$R/.claude/modules/artifacts/health/currency.sh"
o=$(cd "$R" && sh "$H"); rc=$?
expect_rc 0 "$rc" "health: exits 0 even when it cannot check"
has yes "CANNOT CHECK — core-artifact currency" "$o" "health: no origin/main reads CANNOT CHECK"
has yes "UNKNOWN, not current" "$o" "health: ...and says so"
git -C "$R" update-ref refs/remotes/origin/main HEAD
o=$(cd "$R" && sh "$H")
has yes "OK — all 3 core artifacts" "$o" "health MIRROR: one OK line naming the count and the commit"
w src/a.md 'a, moved on main'; commit "Move a"; git -C "$R" update-ref refs/remotes/origin/main HEAD
o=$(cd "$R" && sh "$H")
has yes "STALE — docs/page.html is BEHIND its authorities" "$o" "health: a BEHIND artifact is a STALE verdict line"
has no "OK —" "$o" "health: ...and no OK line beside it"
cur --stamp docs/page.html go-live --note "refreshed: a" >/dev/null; commit "stamp a"; git -C "$R" update-ref refs/remotes/origin/main HEAD

# ── the template's own roadmap grammar: rows under no Gate heading, ids like 1.1 and 1.10 ─────
K="$d/core"; new_repo "$K"
mkdir -p "$K/.claude/lib" "$K/.claude/modules" "$K/docs"
cp "$ROOT/.claude/lib/conf.sh" "$ROOT/.claude/lib/modules.sh" "$K/.claude/lib/"
cp -R "$MOD" "$K/.claude/modules/artifacts"
kc() { (cd "$K" && sh "$K/.claude/modules/artifacts/bin/currency.sh" "$@") 2>&1; }
krc() { (cd "$K" && sh "$K/.claude/modules/artifacts/bin/currency.sh" "$@") >/dev/null 2>&1; echo $?; }
krow() {  # krow ROW-1.1-STATUS ROW-1.10-STATUS [GATE-HEADING]
  { printf '## Phase 1 — First slice\n**Owner:** Rich\n\n'; [ -z "${3:-}" ] || printf '%s\n' "$3"
    printf '| # | Change | Scope | Deps | Status |\n|---|--------|-------|------|--------|\n'
    printf '| 1.1 | `add-first` — a user can start | `src/` | — | %s |\n' "$1"
    printf '| 1.10 | `ops/gate` — the gate runs | `scripts/` | — | %s |\n' "$2"; } > "$K/docs/roadmap.md"
}
krow '⬜ queued' '⬜ queued'
printf '{"pages":{"docs/p.html":{"url":"https://claude.ai/artifact/PPPPPPPPPPPP","authorities":["row:1.1"]}}}\n' > "$K/docs/artifacts.json"
git -C "$K" add -A; kc --stamp docs/p.html --note seed >/dev/null; git -C "$K" add -A; git -C "$K" commit -qm seed
# Falsified by splitting the row lines with read on a TAB IFS (an empty gate merged away, every
# row's value empty): red. A row with no Gate heading is the template roadmap's default shape.
krow '⬜ in flight (#3)' '⬜ queued'
o=$(kc --worktree --check); rc=$?
expect_rc 1 "$rc" "currency: a row under NO Gate heading (the template's grammar) reads BEHIND when it changes"
has yes "row:1.1" "$(line_for "$o" docs/p.html)" "currency: ...naming the row"
# Falsified by comparing ids as numbers in awk (1.1 == 1.10): red.
krow '⬜ queued' '✅ merged (#4)'
expect_rc 0 "$(krc --worktree --check)" "currency MIRROR: row 1.10 changing leaves row:1.1 alone (ids compared as strings)"
# gate:<id>: every row under the gate, and the gate's list of rows. Falsified by dropping the gate
# items from the state: red.
krow '⬜ queued' '⬜ queued' '### Gate A — the first gate'
printf '{"pages":{"docs/p.html":{"url":"https://claude.ai/artifact/PPPPPPPPPPPP","authorities":["gate:A"]}}}\n' > "$K/docs/artifacts.json"
kc --stamp docs/p.html --note seed >/dev/null
expect_rc 0 "$(krc --worktree --check)" "currency: a gate:<id> authority stamps clean"
git -C "$K" add -A; git -C "$K" commit -qm "a gate"
krow '⬜ queued' '✅ merged (#4)' '### Gate A — the first gate'
git -C "$K" commit -qam "1.10 merged"
o=$(kc --ref HEAD --check); rc=$?
expect_rc 1 "$rc" "currency: a row under the gate changing reads BEHIND"
has yes "gate:A (row:1.10)" "$(line_for "$o" docs/p.html)" "currency: ...naming the gate and the row"
# The gate's own list of rows: reordering them changes nothing a row says, only the gate. Falsified
# by dropping the gate:<id> item from the state: red.
kc --stamp docs/p.html --note "1.10 merged" >/dev/null; git -C "$K" commit -qam "stamp"
awk 'NR == FNR { if ($0 ~ /^\| 1\.1 \|/) a = $0; if ($0 ~ /^\| 1\.10 \|/) b = $0; next }
     /^\| 1\.1 \|/ { print b; next } /^\| 1\.10 \|/ { print a; next } { print }' "$K/docs/roadmap.md" "$K/docs/roadmap.md" > "$d/rm" && cat "$d/rm" > "$K/docs/roadmap.md"
o=$(kc --worktree --check); rc=$?
expect_rc 1 "$rc" "currency: reordering a gate's rows reads BEHIND (the gate's list of rows is part of the state)"
has yes "uncommitted: gate:A" "$(line_for "$o" docs/p.html)" "currency: ...naming the gate"
git -C "$K" checkout -q docs/roadmap.md
# Falsified by deleting the refusal: red.
o=$(cd "$K" && sh "$K/.claude/modules/artifacts/bin/currency.sh" --root "$K/docs" --worktree 2>&1); rc=$?
expect_rc 2 "$rc" "currency: a --root inside a repository (not its top) is refused"
has yes "is inside a repository" "$o" "currency: ...and says so"
# Rows under no Gate heading, so an empty gate: id would match them all. Falsified by dropping the
# empty-id refusal: red.
krow '⬜ queued' '⬜ queued'
printf '{"pages":{"docs/p.html":{"url":"https://claude.ai/artifact/PPPPPPPPPPPP","authorities":["gate:", "row:"],"reviewedAt":"0000000000000000000000000000000000000000","reviewNote":"2026-01-01: x"}}}\n' > "$K/docs/artifacts.json"
l=$(line_for "$(kc --worktree)" docs/p.html)
has yes "gate: names no row in docs/roadmap.md; row: names no row" "$l" "currency: an authority with no id is CANNOT CHECK, never every row outside a gate"
printf '{"pages":{"docs/p.html":{"url":"https://claude.ai/artifact/PPPPPPPPPPPP","authorities":["gate:Z", "bogus:x", 5],"reviewedAt":"0000000000000000000000000000000000000000","reviewNote":"2026-01-01: x"}}}\n' > "$K/docs/artifacts.json"
l=$(line_for "$(kc --worktree)" docs/p.html)
has yes "gate:Z names no row" "$l" "currency: a gate with no rows is CANNOT CHECK"
has yes "bogus:x is not an authority kind" "$l" "currency: ...an unknown kind too"
has yes "an authority is not a non-empty string: 5" "$l" "currency: ...and an authority that is not a string"
printf '{"pages":{"docs/p.html":{"published":"x"}}}\n' > "$K/docs/artifacts.json"
o=$(kc --worktree --check); rc=$?
expect_rc 1 "$rc" "currency: a registry with no artifact fails --check (never 'all 0 are current')"
has yes "registers no artifact" "$o" "currency: ...and says so"
git -C "$K" commit -qam "an empty registry"; git -C "$K" update-ref refs/remotes/origin/main HEAD
o=$(cd "$K" && sh "$K/.claude/modules/artifacts/health/currency.sh")
has yes "CANNOT CHECK — docs/artifacts.json registers no artifact" "$o" "health: an empty registry is one CANNOT CHECK line"
has no "— —" "$o" "health: ...passed through, not re-prefixed"
has no "OK —" "$o" "health: ...and never OK"
# A head and main that both lack the registry: nothing to hold current (the MIRROR of the deleted-
# registry block below).
git -C "$K" rm -q docs/artifacts.json; git -C "$K" commit -qm "no registry"; git -C "$K" update-ref refs/remotes/origin/main HEAD
o=$(cd "$K" && ROOT="$K" GUARD_BRANCH=ops/session-2-close GUARD_HEAD="$(git -C "$K" rev-parse HEAD)" GUARD_PR=2 sh "$K/.claude/modules/artifacts/guard.d/currency.sh" 2>&1); rc=$?
expect_rc 0 "$rc" "guard rule MIRROR: a close where neither the head nor main has a registry is allowed"
has yes "no docs/artifacts.json at the head" "$o" "guard rule: ...saying so"

# ── the guard rule, through the real pr-merge-guard.sh (Standing Tee's rule 8) ───────────────
mkdir -p "$R/.claude/hooks/lib" "$d/bin" "$R/docs"
cp "$ROOT/.claude/hooks/pr-merge-guard.sh" "$R/.claude/hooks/"
cp "$ROOT/.claude/hooks/lib/"* "$R/.claude/hooks/lib/"
cp "$ROOT/.claude/lib/change.sh" "$ROOT/.claude/lib/evals.sh" "$R/.claude/lib/"
cp "$ROOT/.claude/scenario-trace.sh" "$R/.claude/"
printf 'GATE_DISPLAY_CONTEXTS="ci/local"\nMODULES="artifacts"\n' > "$R/.claude/project.conf"
commit "the model"
git init -q --bare "$R/acme/app.git"
echo "acme/" >> "$R/.git/info/exclude"
git -C "$R" remote add origin acme/app.git
(cd "$R" && git push -q origin HEAD:main 2>/dev/null && git fetch -q origin)
cat > "$d/bin/gh" <<'EOF'
#!/bin/sh
case "$1 $2" in
  "pr checks") echo '[]' ;;
  "pr view") case "$*" in
      *headRefName*) printf '{"headRefName":"%s","headRefOid":"%s"}\n' "$GH_BRANCH" "$GH_HEAD" ;;
      *headRefOid*) echo "$GH_HEAD" ;;
    esac ;;
  "run list") echo 0 ;;
  *) exit 1 ;;
esac
EOF
chmod +x "$d/bin/gh"
gd() {  # gd WANT BRANCH HEAD LABEL [env...]
  want=$1 br=$2 hd=$3 label=$4; shift 4
  printf '%s\tfull\tclean\tall\n' "$hd" > "$R/.git/ci-receipt"
  rc=0
  gout=$(payload "gh pr merge 999 --squash" "$R" | (cd "$R" && env PATH="$d/bin:$PATH" GH_HEAD="$hd" GH_BRANCH="$br" "$@" sh "$R/.claude/hooks/pr-merge-guard.sh" 2>"$d/err")) || rc=$?
  expect_rc "$want" "$rc" "guard rule: $label"
  [ "$rc" = "$want" ] || sed 's/^/       /' "$d/err" | head -4
}
git -C "$R" checkout -q -b ops/session-9-close
w src/sub/b.md 'b, rewritten by the deploy'; commit "Rewrite the runbook"
HD=$(git -C "$R" rev-parse HEAD)
# Falsified by deleting the rule's exit 2: green here (red as a check).
gd 2 ops/session-9-close "$HD" "BLOCKS a close whose head has a BEHIND artifact"
grep -q 'module artifacts guard.d/currency.sh' "$d/err" && ok "guard rule: ...the block names the module's rule" || fail "guard block: $(head -3 "$d/err")"
grep -qE 'BEHIND +docs/page.html' "$d/err" && grep -q 'src/\*\*/\*.md (sub/b.md)' "$d/err" && ok "guard rule: ...and carries the BEHIND line with the source that moved" || fail "guard block text: $(cat "$d/err" | head -5)"
# MIRROR: the rule is scoped to the one PR per session whose job is to leave the pages true.
gd 0 ops/other-tooling "$HD" "MIRROR: ALLOWS a branch that is not a session close, with the same BEHIND artifact"
case "$gout" in *"currency.sh"*) fail "a non-close branch got a currency advisory: $gout" ;; *) ok "guard rule MIRROR: ...and says nothing there" ;; esac
# Falsified by matching ops/session-*: the tooling branches go red.
# The branch scoping is the rule's own: asked of the rule directly (the hook around it is proven
# above), which keeps this check inside the plugin suite's time budget.
rule() { (cd "$R" && ROOT="$R" GUARD_BRANCH="$1" GUARD_HEAD="$2" GUARD_PR=999 sh "$R/.claude/modules/artifacts/guard.d/currency.sh") >/dev/null 2>&1; echo $?; }
for b in ops/session-start-x ops/session-plan-gate-fixes ops/session-71-cleanup; do expect_rc 0 "$(rule "$b" "$HD")" "guard rule MIRROR: ALLOWS the session TOOLING branch $b"; done
for b in ops/session-93-close-addendum ops/session-94-close-2; do expect_rc 2 "$(rule "$b" "$HD")" "guard rule: BLOCKS the close variant $b"; done
gd 2 ops/session-9-close "$HD" "FAILS CLOSED when the check cannot finish in time" ARTIFACT_RULE_SECONDS=0
grep -q 'did not finish within 0 s' "$d/err" && ok "guard rule: ...and says it was stopped" || fail "guard timeout text: $(head -3 "$d/err")"
# A real overrun, past currency.sh's start: a jq that takes 2 s, a 1 s budget. The rule is run on
# its own (the hook's own jq calls would be slow too). Falsified by a TERM trap that exits 2: the
# block then reads "could not run", not "did not finish".
# Slow ONCE: a parent shell holds a trapped TERM until its foreground child ends, so a jq slow on
# every call would let the KILL win (137) and never reach the trap this case is about.
mkdir -p "$d/slow"; printf '#!/bin/sh\n[ -e "$0.once" ] || { : > "$0.once"; sleep 2; }\nexec %s "$@"\n' "$(command -v jq)" > "$d/slow/jq"; chmod +x "$d/slow/jq"
o=$(cd "$R" && env PATH="$d/slow:$PATH" ARTIFACT_RULE_SECONDS=1 GUARD_BRANCH=ops/session-9-close GUARD_HEAD="$HD" GUARD_PR=999 sh "$R/.claude/modules/artifacts/guard.d/currency.sh" 2>&1); rc=$?
expect_rc 2 "$rc" "guard rule: a check that overruns its budget mid-run blocks"
has yes "did not finish within 1 s" "$o" "guard rule: ...and says it was stopped, not that it could not run"
# The verdict is the HEAD commit's, never the local tree's. Falsified by reading the working tree: red.
cur --stamp docs/page.html --note "reviewed" >/dev/null
gd 2 ops/session-9-close "$HD" "reads the HEAD commit: a stamp on disk alone does not clear it"
commit "Stamp the page: reviewed"
HD=$(git -C "$R" rev-parse HEAD)
gd 0 ops/session-9-close "$HD" "ALLOWS the close once the stamp is committed"
case "$gout" in *"every core artifact is current with its sources at the head"*) ok "guard rule: ...and says it ran" ;; *) fail "guard advisory: $gout" ;; esac
w src/sub/b.md 'b, rewritten again'; commit "Rewrite again"; HD=$(git -C "$R" rev-parse HEAD)
gd 2 ops/session-9-close "$HD" "BLOCKS again once a source moves after the stamp"
# Deleting the registry is not the way past the rule. Falsified by allowing a head with no registry: red.
git -C "$R" rm -q docs/artifacts.json; commit "Drop the registry"
gd 2 ops/session-9-close "$(git -C "$R" rev-parse HEAD)" "BLOCKS a close whose head deleted the registry main has"
grep -q 'which origin/main has' "$d/err" && ok "guard rule: ...and says the registry is gone from the head" || fail "deleted-registry text: $(head -3 "$d/err")"
git -C "$R" reset -q --hard HEAD~1
sed -i.bak 's/^MODULES=.*/MODULES=""/' "$R/.claude/project.conf" && rm -f "$R/.claude/project.conf.bak"
gd 0 ops/session-9-close "$HD" "MIRROR: the module OFF, the same BEHIND close is not this rule's to block"
sed -i.bak 's/^MODULES=.*/MODULES="artifacts"/' "$R/.claude/project.conf" && rm -f "$R/.claude/project.conf.bak"
git -C "$R" checkout -q main

# ── the publishing half ──────────────────────────────────────────────────────────────────────
PUB="$R/.claude/modules/artifacts/bin/publish.sh"
o=$(cd "$R" && sh "$PUB" --status); has yes "publishing is off" "$o" "publish: ARTIFACT_PUBLISH=off (the default) says so, and does nothing"
o=$(cd "$R" && sh "$R/.claude/modules/artifacts/health/shared-copies.sh")
has yes "OK — not applicable: ARTIFACT_PUBLISH=off" "$o" "publish: the health line names the switch when it is off"
# --recorded-in: what merge-pr publishes, from the commit. Falsified by listing every page: red.
P="$d/pub"; new_repo "$P"; mkdir -p "$P/docs"
printf '{"pages":{"docs/a.html":{"url":"https://claude.ai/artifact/AAAAAAAAAAAA","published":"0000000000000000000000000000000000000000"},"docs/b.html":{"url":"https://claude.ai/artifact/BBBBBBBBBBBB","published":"1111111111111111111111111111111111111111"}}}\n' > "$P/docs/artifacts.json"
git -C "$P" add -A && git -C "$P" commit -qm seed
jq '.pages["docs/b.html"].published = "2222222222222222222222222222222222222222"' "$P/docs/artifacts.json" > "$d/r" && cat "$d/r" > "$P/docs/artifacts.json"
git -C "$P" commit -qam "record b"
o=$(ARTIFACT_PUBLISH=claude.ai sh "$PUB" --root "$P" --recorded-in HEAD 2>"$d/err")
[ "$o" = "docs/b.html https://claude.ai/artifact/BBBBBBBBBBBB" ] && ok "publish --recorded-in: lists exactly the page whose published hash the commit changed" || fail "--recorded-in: $o"
o=$(ARTIFACT_PUBLISH=claude.ai sh "$PUB" --root "$P" --recorded-in HEAD~1 2>&1)
has yes "its entries are seeds, nothing to publish" "$o" "publish --recorded-in: the commit that introduced the registry holds seeds, not records"
# open.sh iterates the registry, never a list of its own. Falsified by reading only "pages": red.
OPEN="$R/.claude/modules/artifacts/bin/open.sh"
jq '.extra = {"thing": {"url": "https://claude.ai/artifact/DDDDDDDDDDDD"}}' "$P/docs/artifacts.json" > "$d/r" && cat "$d/r" > "$P/docs/artifacts.json"
o=$(sh "$OPEN" --root "$P" --print 2>&1)
has yes "extra/thing → https://claude.ai/artifact/DDDDDDDDDDDD" "$o" "open: prints entries it has never seen, a new section included"
printf '#!/bin/sh\necho "$1" >> "%s/opened"\n' "$d" > "$d/bin/rec"; chmod +x "$d/bin/rec"
sh "$OPEN" --root "$P" --opener "$d/bin/rec" >/dev/null 2>&1
[ "$(wc -l < "$d/opened" | tr -d ' ')" = 3 ] && ok "open: hands each url to the opener exactly once" || fail "open: opened $(cat "$d/opened" 2>/dev/null)"
# An opener that reads stdin must not eat the rest of the list. Falsified by dropping </dev/null: red.
printf '#!/bin/sh\ncat >/dev/null\necho "$1" >> "%s/opened2"\n' "$d" > "$d/bin/greedy"; chmod +x "$d/bin/greedy"
sh "$OPEN" --root "$P" --opener "$d/bin/greedy" >/dev/null 2>&1
[ "$(wc -l < "$d/opened2" | tr -d ' ')" = 3 ] && ok "open: an opener that reads stdin still gets every url" || fail "open (greedy opener): $(cat "$d/opened2" 2>/dev/null)"
printf '#!/bin/sh\ncase "$1" in *BBBB*) echo nope >&2; exit 4 ;; esac\n' > "$d/bin/half"; chmod +x "$d/bin/half"
o=$(sh "$OPEN" --root "$P" --opener "$d/bin/half" 2>&1); rc=$?
expect_rc 1 "$rc" "open: an opener that fails exits 1..."
has yes "docs/b.html → https://claude.ai/artifact/BBBBBBBBBBBB  NOT OPENED" "$o" "open: ...naming the link it could not open"
has yes "extra/thing → https://claude.ai/artifact/DDDDDDDDDDDD" "$o" "open: ...and tries the rest"
jq '.extra.thing.url = "claude.ai/artifact/DDDD"' "$P/docs/artifacts.json" > "$d/r" && cat "$d/r" > "$P/docs/artifacts.json"
o=$(sh "$OPEN" --root "$P" --print 2>&1); rc=$?
expect_rc 2 "$rc" "open: an entry with no usable url is an error, never a skip"
has yes "extra/thing has no well-formed url" "$o" "open: ...naming the entry"
# The copy (Node): only checked where Node is, or where this project turned publishing on.
if command -v node >/dev/null 2>&1; then
  Q="$d/prep"; new_repo "$Q"; mkdir -p "$Q/docs"
  printf '<p>a <a href="b.html#x">b</a></p>\n<!-- a repo-only note -->\n' > "$Q/docs/a.html"
  printf '<p>b</p>\n' > "$Q/docs/b.html"
  printf '{"pages":{"docs/a.html":{"url":"https://claude.ai/artifact/AAAAAAAAAAAA","published":"0000000000000000000000000000000000000000"},"docs/b.html":{"url":"https://claude.ai/artifact/BBBBBBBBBBBB","published":"%s"}}}\n' "$(git hash-object "$Q/docs/b.html")" > "$Q/docs/artifacts.json"
  git -C "$Q" add -A && git -C "$Q" commit -qm pages && git -C "$Q" update-ref refs/remotes/origin/main HEAD
  o=$(ARTIFACT_PUBLISH=claude.ai sh "$PUB" --root "$Q" --status 2>&1)
  has yes "STALE docs/a.html → https://claude.ai/artifact/AAAAAAAAAAAA" "$o" "publish --status: a copy that differs from its record is STALE"
  has yes "OK    docs/b.html → https://claude.ai/artifact/BBBBBBBBBBBB" "$o" "publish --status MIRROR: a page equal to its record is OK"
  f=$(ARTIFACT_PUBLISH=claude.ai sh "$PUB" --root "$Q" docs/a.html --ref HEAD --out "$d/a.out" 2>&1)
  grep -q 'target="_blank" rel="noopener" href="https://claude.ai/artifact/BBBBBBBBBBBB#x"' "$d/a.out" && ok "publish: the copy links a registered page by its artifact url, fragment kept" || fail "copy: $(cat "$d/a.out" 2>/dev/null) $f"
  grep -q 'repo-only note' "$d/a.out" && fail "publish: the copy kept an HTML comment" || ok "publish: the copy drops HTML comments"
  (ARTIFACT_PUBLISH=claude.ai sh "$PUB" --root "$Q" --record docs/a.html >/dev/null 2>&1)
  git -C "$Q" commit -qam record && git -C "$Q" update-ref refs/remotes/origin/main HEAD
  o=$(ARTIFACT_PUBLISH=claude.ai sh "$PUB" --root "$Q" --status 2>&1)
  has yes "All 2 shared copies are current" "$o" "publish --record: recording the copy clears STALE"
else
  if [ "${ARTIFACT_PUBLISH:-off}" = claude.ai ]; then fail "ARTIFACT_PUBLISH=claude.ai, but node is not installed, so the copies cannot be prepared"
  else ok "SKIPPED: node is not installed, so the claude.ai copies (ARTIFACT_PUBLISH=claude.ai) are not exercised here"; fi
fi

# ── wiring: each part has a real caller (AGENTS.md rule 3) ────────────────────────────────────
conf="$MOD/module.conf"
grep -q '^ARTIFACT_REGISTRY="docs/artifacts.json"' "$conf" && grep -q '^ARTIFACT_RULE_SECONDS=' "$conf" && grep -q '^ARTIFACT_PUBLISH="off"' "$conf" \
  && ok "module.conf: the registry, the rule's budget and the publishing switch (off) are default keys" || fail "module.conf keys"
F="$MOD/skills/session-close/artifacts.md"
c1=$(grep -n 'currency.sh --worktree 2>&1' "$F" | head -n 1 | cut -d: -f1)
c3=$(grep -n 'currency.sh --worktree --check' "$F" | head -n 1 | cut -d: -f1)
g3=$(grep -n 'Then the gate' "$F" | head -n 1 | cut -d: -f1)
m4=$(grep -n 'merge-pr' "$F" | tail -n 1 | cut -d: -f1)
# Falsified by moving the last check below the merge: red.
if [ -n "$c1" ] && [ -n "$c3" ] && [ -n "$g3" ] && [ -n "$m4" ] && [ "$c1" -lt "$c3" ] && [ "$c3" -le "$g3" ] && [ "$g3" -lt "$m4" ]; then
  ok "session-close fragment: the close's LAST currency check comes after its edits and before the gate and the merge"
else fail "session-close fragment order: worktree@$c1 last@$c3 gate@$g3 merge@$m4"; fi
grep -qE '[Aa]fter step 3.s roadmap edits' "$F" && ok "session-close fragment: stamps after the roadmap edits that move its sources" || fail "session-close fragment: no roadmap-first ordering"
grep -q 'publish.sh --recorded-in' "$MOD/skills/merge-pr/artifacts.md" && ok "merge-pr fragment: publishes what the merge commit recorded" || fail "merge-pr fragment: no --recorded-in"
grep -q 'bin/open.sh' "$MOD/skills/session-start/artifacts.md" && ok "session-start fragment: opens the shared copies through the tool" || fail "session-start fragment: no open.sh"
grep -q 'currency.sh" --root "$ROOT" --worktree' "$MOD/context.d/session-close/artifacts.sh" && ok "session-close's context section reads the close's own tree" || fail "session-close context: not --worktree"
grep -q -- '--ref "$hd" --check' "$MOD/guard.d/currency.sh" && ok "the guard rule judges the head with --check" || fail "guard rule: not --ref head --check"
for f in "$MOD"/bin/*.sh "$MOD"/guard.d/*.sh "$MOD"/health/*.sh "$MOD"/context.d/*/*.sh "$MOD"/checks/*.sh; do
  sh -n "$f" || fail "$(basename "$f") does not parse under sh"
done
nodeuse=""
for f in "$MOD"/bin/currency.sh "$MOD"/bin/open.sh "$MOD"/guard.d/*.sh "$MOD"/health/currency.sh "$MOD"/context.d/*/*.sh; do
  grep -v '^[[:space:]]*#' "$f" | grep -qw node && nodeuse="$nodeuse $(basename "$f")"
done
[ -z "$nodeuse" ] && ok "the currency half, the guard rule, the opener and the context sections run no node" || fail "these run node:$nodeuse (the currency half must be sh, git and jq)"
finish
