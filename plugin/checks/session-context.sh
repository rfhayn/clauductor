#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The GitHub-reading context sections (.claude/lib/context.sh): remote branches with no PR, open PRs
# whose roadmap row still reads queued on origin/<main>, and living specs still carrying TBD. Driven
# through a stub `gh` in a throwaway repo whose origin is a local bare repo. Ported from the
# meta-tests of the project these were upstreamed from (P2.4); each case says what it holds, and a
# sentence that must be absent is asserted next to a case where it is present.
. "$(dirname "$0")/lib.sh"
need git jq awk

d=$(scratch)
R="$d/p"; new_repo "$R"
mkdir -p "$R/.claude/lib" "$R/docs" "$R/specs/a" "$R/specs/b" "$d/bin"
cp "$CLAUDUCTOR_FW/lib/conf.sh" "$CLAUDUCTOR_FW/lib/context.sh" "$R/.claude/lib/"
cp "$CLAUDUCTOR_FW/roadmap-queue.sh" "$R/.claude/"
roadmap() {  # roadmap [2C.9-STATUS]
  cat <<EOF
## Phase 2 — the build
**Owner:** Alice
### Gate 2B — done
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 2B.1 | \`add-done\` — finished | scope | — | ✅ merged (#1) |
### Gate 2C-L — league
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 2C.8 | \`add-alpha\` — the alpha thing | scope | — | ⬜ queued |
| 2C.9 | \`add-beta\` — the beta thing | scope | — | ${1:-⬜ queued} |
| 2C.10 | \`add-gamma\` — the gamma thing | scope | — | ⬜ queued — after the beta, which this status clips at forty |
| 2C.11 | \`ops/tidy\` — tidy | scope | — | ⬜ queued |
| 2C.12 | \`add-delta\` — delta | scope | — | ⬜ placeholder — owner: designer |
EOF
}
roadmap > "$R/docs/roadmap.md"
printf '# A\n\n## Purpose\nTBD - created by archiving change add-a.\n' > "$R/specs/a/spec.md"
printf '# B\n\n## Purpose\nWhat b does, in full.\n' > "$R/specs/b/spec.md"
git -C "$R" add -A && git -C "$R" commit -qm base
git init -q --bare "$d/origin.git"
git -C "$R" remote add origin "$d/origin.git"
git -C "$R" push -q origin HEAD:main 2>/dev/null
for b in lane-x old-y; do git -C "$R" push -q origin "HEAD:refs/heads/$b" 2>/dev/null; done
git -C "$R" fetch -q origin

cat > "$d/bin/gh" <<'EOF'
#!/bin/sh
case "$* " in
  *"api --paginate"*"branches"*) [ -n "${GH_BRANCHES_FAIL:-}" ] && exit 1; printf '%s\n' $GH_BRANCHES ;;
  *) exit 1 ;;
esac
EOF
chmod +x "$d/bin/gh"
# ctx FUNCTION ARGS...: one context.sh function in the fixture, online, with the stub gh first.
ctx() {
  (cd "$R" && env PATH="$d/bin:$PATH" CONTEXT_OFFLINE= ROOT="$R" sh -c '. .claude/lib/conf.sh; . .claude/lib/context.sh; "$@"' ctx "$@" 2>&1)
}
pr() { printf '{"number":%s,"headRefName":"%s","title":"%s","author":{"login":"a"}}' "$1" "$2" "${3:-t}"; }
prs() { printf '['; sep=""; for p in "$@"; do printf '%s%s' "$sep" "$p"; sep=,; done; printf ']'; }
has() { case "$2" in *"$3"*) ok "$1" ;; *) fail "$1: no \"$3\" in: $2" ;; esac; }
hasnt() { case "$2" in *"$3"*) fail "$1: \"$3\" in: $2" ;; *) ok "$1" ;; esac; }

# ── queued while a PR is open ──────────────────────────────────────────────────────────────────
out=$(ctx ctx_queued_open "$(prs "$(pr 41 change/add-beta)")")
has "queued-open: flags a change/ PR whose row reads queued, naming the row" "$out" "QUEUED ON MAIN: #41 change/add-beta → 2C.9 add-beta reads \"⬜ queued\""
has "queued-open: ...and the status to set, in the roadmap" "$out" "set its status to \"⬜ in flight (#41)\" in docs/roadmap.md in this close"
# MIRROR: main's row in flight, an ops/ PR named like a capability row, a merged row: none flagged.
roadmap "⬜ in flight (#41)" > "$R/docs/roadmap.md"; git -C "$R" commit -qam "2C.9 in flight"; git -C "$R" push -q origin HEAD:main 2>/dev/null; git -C "$R" fetch -q origin
out=$(ctx ctx_queued_open "$(prs "$(pr 41 change/add-beta)" "$(pr 42 ops/add-alpha)" "$(pr 43 change/add-done)")")
hasnt "queued-open: MIRROR: an in-flight row, an ops/ PR named like a capability row, a merged row are not flagged" "$out" "QUEUED ON"
has "queued-open: ...and it says none, by name" "$out" "none: no open PR's row reads queued on main"
# The roadmap read is origin/main's, never this branch's: the branch says in flight, main says queued.
roadmap > "$R/docs/roadmap.md"; git -C "$R" commit -qam "back to queued"; git -C "$R" push -q origin HEAD:main 2>/dev/null; git -C "$R" fetch -q origin
roadmap "⬜ in flight (#41)" > "$R/docs/roadmap.md"
out=$(ctx ctx_queued_open "$(prs "$(pr 41 change/add-beta)")")
has "queued-open: reads origin/main's roadmap, not the working tree's (in flight here, queued on main)" "$out" "QUEUED ON MAIN: #41"
git -C "$R" checkout -q -- docs/roadmap.md
out=$(ctx ctx_queued_open "$(prs "$(pr 44 change/add-beta-2)")")
hasnt "queued-open: a change id inside a longer one is not that row (add-beta-2 is not add-beta)" "$out" "QUEUED ON"
has "queued-open: ...a change/ PR that names no row is noted, not dropped" "$out" "note: #44 change/add-beta-2 names no roadmap row"
out=$(ctx ctx_queued_open "$(prs "$(pr 45 change/continue-gamma "2C.10: the gamma, continued")")")
has "queued-open: a PR title led by a row id names that row" "$out" "QUEUED ON MAIN: #45 change/continue-gamma → 2C.10 add-gamma reads \"⬜ queued — after the beta, which this s…\""
has "queued-open: ...its status clipped to 40 characters, never mid-glyph" "$out" "this s…\" — set"
out=$(ctx ctx_queued_open "$(prs "$(pr 46 ops/tidy)")")
has "queued-open: an ops/ PR whose ops row reads queued is flagged too" "$out" "QUEUED ON MAIN: #46 ops/tidy → 2C.11 ops/tidy"
out=$(ctx ctx_queued_open "$(prs "$(pr 47 change/add-delta)")")
hasnt "queued-open: a row open in its own word (⬜ placeholder) is not queued" "$out" "QUEUED ON"
out=$(ctx ctx_queued_open "")
has "queued-open: an unreadable PR list is CANNOT CHECK, never none" "$out" "CANNOT CHECK"
hasnt "queued-open: ...and never none" "$out" "none:"
out=$(cd "$R" && env PATH="$d/bin:$PATH" CONTEXT_OFFLINE=1 sh -c '. .claude/lib/conf.sh; . .claude/lib/context.sh; ctx_queued_open "[]"' 2>&1)
has "queued-open: offline is CANNOT CHECK" "$out" "CANNOT CHECK — offline"
printf '## Phase 1 — x\n| # | Change | Scope | Deps | Status |\n|---|---|---|---|---|\n| 1.1 | `add-a` — a | s | — | 🔨 doing |\n' > "$R/docs/roadmap.md"
git -C "$R" commit -qam broken; git -C "$R" push -q origin HEAD:main 2>/dev/null; git -C "$R" fetch -q origin
out=$(ctx ctx_queued_open "$(prs "$(pr 41 change/add-a)")")
has "queued-open: a roadmap on origin/main that does not parse is CANNOT CHECK, with the parser's error" "$out" "CANNOT CHECK — the roadmap on origin/main could not be parsed: ERROR"
# Through ROADMAP_PARSER: a project's own parser answers, not the template's grammar.
mkdir -p "$R/infra"
printf '#!/bin/sh\nprintf "9\\t1\\t\\tZ.1\\tadd-zeta\\tchange\\tqueued\\t\\t\\tzeta\\t\\t\\n"\n' > "$R/infra/own.sh"
echo 'ROADMAP_PARSER="sh infra/own.sh"' > "$R/.claude/project.conf"
out=$(ctx ctx_queued_open "$(prs "$(pr 48 change/add-zeta)")")
has "queued-open: reads the rows through ROADMAP_PARSER (roadmap_queue), not a second parser" "$out" "QUEUED ON MAIN: #48 change/add-zeta → Z.1 add-zeta"
rm -f "$R/.claude/project.conf"
roadmap > "$R/docs/roadmap.md"; git -C "$R" commit -qam fixed; git -C "$R" push -q origin HEAD:main 2>/dev/null; git -C "$R" fetch -q origin

# ── remote branches with no open PR ────────────────────────────────────────────────────────────
out=$(cd "$R" && env PATH="$d/bin:$PATH" CONTEXT_OFFLINE= GH_BRANCHES="main lane-x old-y gone-z" sh -c '. .claude/lib/conf.sh; . .claude/lib/context.sh; ctx_loose_branches "$1"' ctx "$(prs "$(pr 50 lane-x)")" 2>&1)
has "branches: a branch with no open PR is listed with its last commit's author and date" "$out" "old-y · $(git -C "$R" log -1 --format='%an · %ad' --date=short origin/old-y)"
hasnt "branches: a branch with an open PR is not" "$out" "lane-x"
hasnt "branches: the main branch is not" "$out" "main ·"
has "branches: a branch GitHub has and this clone has not fetched says so" "$out" "gone-z · (not fetched)"
out=$(cd "$R" && env PATH="$d/bin:$PATH" CONTEXT_OFFLINE= GH_BRANCHES="main lane-x" sh -c '. .claude/lib/conf.sh; . .claude/lib/context.sh; ctx_loose_branches "$1"' ctx "$(prs "$(pr 50 lane-x)")" 2>&1)
[ "$out" = none ] && ok "branches: none says none" || fail "branches: none read as: $out"
out=$(cd "$R" && env PATH="$d/bin:$PATH" CONTEXT_OFFLINE= GH_BRANCHES="main old-y" sh -c '. .claude/lib/conf.sh; . .claude/lib/context.sh; ctx_loose_branches ""' 2>&1)
has "branches: an unreadable PR list is CANNOT CHECK, not every branch PR-less" "$out" "CANNOT CHECK — the open-PR list could not be read"
hasnt "branches: ...and lists no branch" "$out" "old-y"
out=$(cd "$R" && env PATH="$d/bin:$PATH" CONTEXT_OFFLINE= GH_BRANCHES_FAIL=1 sh -c '. .claude/lib/conf.sh; . .claude/lib/context.sh; ctx_loose_branches "[]"' 2>&1)
has "branches: gh failing to list branches is CANNOT CHECK" "$out" "CANNOT CHECK — gh could not list the remote branches"

# ── living specs still carrying TBD ────────────────────────────────────────────────────────────
out=$(ctx ctx_tbd_specs)
[ "$out" = "specs/a/spec.md" ] && ok "TBD: names each living spec that still says TBD, and only those" || fail "TBD specs: $out"
printf '# A\n\n## Purpose\nWhat a does.\n' > "$R/specs/a/spec.md"
[ "$(ctx ctx_tbd_specs)" = none ] && ok "TBD: none says none" || fail "TBD none: $(ctx ctx_tbd_specs)"
rm -rf "$R/specs"
has "TBD: no specs directory is not a finding, and says why" "$(ctx ctx_tbd_specs)" "none: no specs/ here"

# ── the context scripts print every section, CANNOT CHECK where it cannot read ─────────────────
out=$(sh "$CLAUDUCTOR_FW/skills/session-close/context.sh" 2>&1)
for h in "Remote branches with no open PR" "roadmap row still reads queued on origin/" "Living specs still carrying TBD"; do
  sec=$(printf '%s\n' "$out" | awk -v h="$h" 'index($0, h) { on = 1; next } on && /^- / { exit } on { print }')
  [ -n "$sec" ] && ok "session-close context: \"$h\" prints a line offline ($(printf '%s' "$sec" | head -1 | sed 's/^ *//'))" || fail "session-close context: \"$h\" printed nothing"
done
out=$(sh "$CLAUDUCTOR_FW/skills/session-start/context.sh" 2>&1)
sec=$(printf '%s\n' "$out" | awk 'index($0, "Remote branches with no open PR") { on = 1; next } on && /^- / { exit } on { print }')
case "$sec" in *"CANNOT CHECK"*) ok "session-start context: remote branches says CANNOT CHECK offline, never none" ;; *) fail "session-start remote branches offline: $sec" ;; esac
# Without lib/context.sh the sections say so, and the script still reaches its last section. The
# model's files are this project's .claude, or the plugin root when this check runs from the plugin.
F="$d/full"; mkdir -p "$F"; cp -R "${CLAUDUCTOR_FW:-$ROOT/.claude}" "$F/.claude"; rm -f "$F/.claude/lib/context.sh"; new_repo "$F"
out=$(sh "$F/.claude/skills/session-close/context.sh" 2>&1)
has "session-close context: lib/context.sh missing is CANNOT CHECK by name" "$out" "lib/context.sh is missing"
has "session-close context: ...and the sections after it still print" "$out" "This session's cost"
finish
