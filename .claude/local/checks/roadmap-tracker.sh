#!/bin/sh
# The roadmap tracker's data (OPS-83): roadmap-tracker.sh maps every roadmap state to the
# tracker's, reads the Session plan without the queue parser taking it for rows, and the
# session-start line says NEVER RAN, OK or STALE from the stamp. Run on a fixture project, a
# scratch git repository holding a copy of the model's parser and the two local scripts.
. "${CHECKS_LIB:?}"
need jq git

F="$(scratch)/proj"
new_repo "$F"
mkdir -p "$F/.claude/lib" "$F/.claude/local/context.d/session-start" "$F/docs" "$F/changes/b-1-approved" "$F/changes/b-2-plain"
cp "$ROOT/.claude/lib/conf.sh" "$F/.claude/lib/"
cp "$ROOT/.claude/roadmap-queue.sh" "$ROOT/.claude/roadmap-plan.sh" "$F/.claude/"
cp "$ROOT/.claude/local/roadmap-tracker.sh" "$F/.claude/local/"
cp "$ROOT/.claude/local/context.d/session-start/roadmap-tracker.sh" "$ROOT/.claude/local/context.d/session-start/session-plan.sh" "$F/.claude/local/context.d/session-start/"
cat > "$F/docs/roadmap.md" <<'EOF'
# Roadmap

## Phase 1 — First

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| A-1 | `ops/a-1-old` — merged long ago | x | — | ✅ merged (#10) |
| A-2 | `ops/a-2-new` — merged this session | x | — | ✅ merged (#11) |
| A-3 | `ops/a-3-flight` — in flight with its PR | x | — | ⬜ in flight (#12) |

### Second (a track)

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| B-1 | `b-1-approved` — a change the owner approved | x | — | ⬜ queued |
| B-2 | `b-2-plain` — a change only proposed | x | A-3 | ⬜ queued |
| B-3 | `ops/b-3-park` — parked | x | — | ⬜ parked — until issue (#55) recurs |
| B-4 | `ops/b-4-defer` — deferred | x | — | ⬜ deferred — on a trigger |
| B-5 | `ops/b-5-gone` — cancelled and planned | x | — | ❌ cancelled — no |
| B-6 | `ops/b-6-gone` — cancelled and not planned | x | — | ❌ cancelled — no |
| B-7 | `b-7-build` — a change being built in its worktree | x | — | ⬜ queued |

## Phase 2 — Later

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| E-1 | `ops/e-1-later` — a row of the second phase | x | — | ⬜ queued |

## Session plan

| Session | When | Focus | Rows | The owner decides |
|---|---|---|---|---|
| S1 | today | First things | A-2, A-1, B-1, B-5 | B-1: approve its design; nothing else |
| S2–3 | | Then the rest | B-2, B-7, Z-9 | — |

## Done before

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| F-1 | `ops/f-1-outside` — a row outside every phase | x | — | ⬜ queued |
EOF
cp "$F/docs/roadmap.md" "$(scratch)/fixture.md"
printf '**Approved:** 2026-10-01 by Rich · design 0123456789ab\n\n# b-1\n' > "$F/changes/b-1-approved/proposal.md"
printf '**Status:** proposed\n\n# b-2\n' > "$F/changes/b-2-plain/proposal.md"
git -C "$F" add -A && GIT_COMMITTER_DATE='2020-01-01T12:00:00' git -C "$F" commit -qm 'A-1: Merge the old row (#10)'
git -C "$F" commit -q --allow-empty -m 'A-2: Merge the new row (#11)'
# B-7's branch is checked out with a ticked task (a build under way); B-2's with none ticked.
git -C "$F" worktree add -q -b change/b-7-build "$(scratch)/wt7" && mkdir -p "$(scratch)/wt7/changes/b-7-build"
printf '## 1. First\n\n- [x] 1.1 done\n- [ ] 1.2 next\n' > "$(scratch)/wt7/changes/b-7-build/tasks.md"
git -C "$F" worktree add -q -b change/b-2-plain "$(scratch)/wt2"
printf '## 1. First\n\n- [ ] 1.1 not yet\n' > "$(scratch)/wt2/changes/b-2-plain/tasks.md"

O="$(scratch)/out"
gen() { sh "$F/.claude/local/roadmap-tracker.sh" "$@"; }
err=$(gen "$O" 2>&1 >/dev/null); rc=$?
expect_rc 0 "$rc" "the generator builds the fixture's datasets ($err)"
row() { jq -r --arg id "$1" '[.[] | select(.id == $id)] | if length == 0 then "absent" else .[0].state end' "$O/roadmap.json" 2>/dev/null; }
is() { got=$(row "$1"); [ "$got" = "$2" ] && ok "$3: $1 reads $2" || fail "$3: $1 reads $got, want $2"; }
is A-2 merged "merged since SINCE -> merged"
is A-1 absent "merged before SINCE -> left out"
is A-3 merging "in flight with a PR -> merging"
is B-1 approved "queued with an **Approved:** line -> approved"
is B-2 queued "queued without one, its worktree with nothing ticked -> queued"
is B-7 running "queued, its change branch in a worktree with a ticked task -> running"
is B-3 parked "an open word (parked) -> parked"
is B-4 parked "an open word (deferred) -> parked"
is B-5 cancelled "cancelled and planned -> cancelled"
is B-6 absent "cancelled and not planned -> left out"
[ "$(jq -r '.[] | select(.id == "A-3") | .pr' "$O/roadmap.json")" = "#12" ] && ok "pr comes from the queue's PR field on a row in flight" || fail "A-3's pr is not #12"
[ "$(jq -r '.[] | select(.id == "B-3") | .pr' "$O/roadmap.json")" = "" ] && ok "an open row's (#N) (an issue) is no pr" || fail "B-3's issue (#55) read as its pr"
[ "$(jq -r '.[] | select(.id == "B-1") | [.session, .order, .owner, .track] | join("|")' "$O/roadmap.json")" = "S1|1|Approve its design|Second" ] \
  && ok "a planned row takes its session, order, the owner's decision and its section's track" \
  || fail "B-1 reads $(jq -c '.[] | select(.id == "B-1")' "$O/roadmap.json")"
# The phase number groups the tracker's sessions by phase; a row outside every phase is 0.
[ "$(jq -r '[.[] | select(.id == "B-1" or .id == "E-1" or .id == "F-1") | "\(.id):\(.phase):\(.track)"] | sort | join(" ")' "$O/roadmap.json")" = "B-1:1:Second E-1:2:Phase 2 F-1:0:Outside the phases" ] \
  && ok "each row carries its phase's number (0 outside every phase), beside its track" \
  || fail "phases read $(jq -c '[.[] | select(.id == "B-1" or .id == "E-1" or .id == "F-1") | {id, phase, track}]' "$O/roadmap.json")"
[ "$(jq -r '[.[] | "\(.session)/\(.order)/\(.owner)"] | join(" ")' "$O/sessions.json")" = "S1/1/B-1: approve its design; nothing else S2–3/2/" ] \
  && ok "sessions.json holds each planned session in order" || fail "sessions.json: $(jq -c . "$O/sessions.json")"
case $err in *"names Z-9, which is no roadmap row"*) ok "a planned id that is no row is named, not dropped silently" ;; *) fail "no warning for the unknown Z-9: $err" ;; esac

# The Session plan is not queue rows: the parser's rows (through roadmap_queue) are exactly the nine
# queue rows, none is a session, and in this repository's own roadmap no row sits in the section.
# (Reading the section's lines is the one roadmap read here; the rows come from roadmap_queue.)
tsv=$(cd "$F" && ROOT="$F" sh -c '. .claude/lib/conf.sh && roadmap_queue --tsv')
[ "$(printf '%s\n' "$tsv" | cut -f4 | tr '\n' ' ')" = "A-1 A-2 A-3 B-1 B-2 B-3 B-4 B-5 B-6 B-7 E-1 F-1 " ] \
  && ok "the fixture's Session plan is not parsed as queue rows" || fail "the parser read: $(printf '%s\n' "$tsv" | cut -f4 | tr '\n' ' ')"
jq -e '[.[].id] | any(. == "S1" or . == "S2–3")' "$O/roadmap.json" >/dev/null && fail "a session became a tracker row" || ok "no session is a tracker row"
range=$(cd "$ROOT" && awk '/^## Session plan/ { s = NR; next } s && !e && /^## / { e = NR } END { if (s) print s, (e ? e : NR) }' "$ROADMAP")
if [ -z "$range" ]; then fail "this repository's $ROADMAP has no Session plan section"
else
  if ! own=$(roadmap_queue --tsv); then fail "this repository's roadmap does not parse, so its Session plan cannot be shown apart from the queue: $own"
  else
    inplan=$(printf '%s\n' "$own" | awk -F'\t' -v r="$range" 'BEGIN { split(r, x, " ") } $1 > x[1] && $1 < x[2] { print $4 }')
    [ -z "$inplan" ] && ok "this repository's Session plan (lines $range) holds no queue row" || fail "the parser read Session plan lines as rows: $inplan"
  fi
fi

# A plugged parser's in-flight row with no PR -> running (the template's parser requires a PR).
printf 'ROADMAP_PARSER="sh stub.sh"\n' > "$F/.claude/project.conf"
printf '#!/bin/sh\nprintf "7\\t1\\t\\tR-1\\tops/r-1\\tops\\tinflight\\t\\t\\tbuilding\\t\\t\\t\\t\\t\\n"\n' > "$F/stub.sh"
gen "$O" >/dev/null 2>&1; is R-1 running "in flight without a PR -> running"
rm -f "$F/.claude/project.conf" "$F/stub.sh"

# The tracker's address lives only in the machine's uncommitted tracker-url (the repository is
# public): --url reads it, fails plainly without it, and no committed local-layer file or the
# roadmap names an artifact address.
e=$(gen --url 2>&1) && fail "--url printed an address with no tracker-url: $e" \
  || case $e in *"no tracker configured on this machine (tracker-url)"*) ok "--url with no tracker-url says no tracker is configured" ;; *) fail "--url with no tracker-url: $e" ;; esac
c="$(git -C "$F" rev-parse --path-format=absolute --git-common-dir)/clauductor"
mkdir -p "$c" && printf 'https://example.invalid/tracker\n' > "$c/tracker-url"
[ "$(gen --url 2>&1)" = "https://example.invalid/tracker" ] && ok "--url prints the machine's tracker-url" || fail "--url: $(gen --url 2>&1)"
rm -f "$c/tracker-url"

# leaks REPO: every tracked file that names the tracker. With the machine's tracker-url, its exact
# value (git grep -F); without one, any artifact address but the module docs' public examples: a
# single repeated character, or the artifacts module README's sample id (listed here, so allowing
# another is an edit to this check).
leaks() {
  _u=$(head -n 1 "$(git -C "$1" rev-parse --path-format=absolute --git-common-dir)/clauductor/tracker-url" 2>/dev/null)
  if [ -n "$_u" ]; then git -C "$1" grep -nF -e "$_u"
  # Per address, not per line: -o prints each match whole (the id runs to its last [A-Za-z0-9], so
  # the id compares whole), and only an exempt id is dropped, never its line's neighbours.
  else git -C "$1" grep -noE 'claude\.ai/artifact/[A-Za-z0-9]{8,}' \
    | awk -F'artifact/' '{ id = $NF; r = id; gsub(substr(id, 1, 1), "", r); if (r != "" && id != "XvoZvCgkJJVbiXrWabnVS7") print }'
  fi
}
printf 'see https://example.invalid/tracker\n' > "$F/notes.md"
git -C "$F" add notes.md && git -C "$F" commit -qm 'plant: a committed tracker address'
mkdir -p "$c" && printf 'https://example.invalid/tracker\n' > "$c/tracker-url"
case $(leaks "$F") in "notes.md:1:"*) ok "a committed copy of tracker-url's exact value is found in any tracked file" ;; *) fail "the planted tracker address was missed: $(leaks "$F")" ;; esac
rm -f "$c/tracker-url"
# The planted id is built at run time, so no committed line of this check is itself a leak. It
# shares a line with an exempt sample id, which must not hide it.
id="Zq81$(printf mVbT0pLx)"
printf 'see https://claude.ai/artifact/%s and https://claude.ai/artifact/%s\n' XvoZvCgkJJVbiXrWabnVS7 "$id" > "$F/notes.md"
git -C "$F" commit -qam 'plant: an artifact address'
[ "$(leaks "$F")" = "notes.md:1:claude.ai/artifact/$id" ] && ok "with no tracker-url, an artifact address is found, even on a line with a sample id" || fail "the planted artifact address was missed: $(leaks "$F")"
printf 'see https://claude.ai/artifact/%s, https://claude.ai/artifact/%s and https://claude.ai/artifact/%s7\n' XvoZvCgkJJVbiXrWabnVS7 "$(printf 'A%.0s' 1 2 3 4 5 6 7 8 9 10)" XvoZvCgkJJVbiXrWabnVS7 > "$F/notes.md"
git -C "$F" commit -qam 'plant: sample ids, and one that only starts like one'
sample=XvoZvCgkJJVbiXrWabnVS7
[ "$(leaks "$F")" = "notes.md:1:claude.ai/artifact/${sample}7" ] && ok "...the sample ids are exempt whole, never a longer id that starts like one" || fail "the sample-id exemption: [$(leaks "$F")]"
git -C "$F" rm -q notes.md && git -C "$F" commit -qm 'plant: removed'
if [ -s "$(git -C "$ROOT" rev-parse --path-format=absolute --git-common-dir)/clauductor/tracker-url" ]; then how="its tracker-url's exact value"; else how="an artifact address (no tracker-url here)"; fi
leak=$(leaks "$ROOT")
[ -z "$leak" ] && ok "no tracked file in this repository names $how" || fail "a tracked file names the tracker: $(printf '%s\n' "$leak" | cut -d: -f1-2 | tr '\n' ' ')"

# The session-start line, from the stamp: NOT CONFIGURED with no tracker-url, else by the blobs.
ctx() { CONTEXT_OFFLINE=1 sh "$F/.claude/local/context.d/session-start/roadmap-tracker.sh" 2>&1; }
case $(ctx) in "tracker: NOT CONFIGURED on this machine (no tracker-url)") ok "no tracker-url -> NOT CONFIGURED" ;; *) fail "no tracker-url: $(ctx)" ;; esac
printf 'https://example.invalid/tracker\n' > "$c/tracker-url"
case $(ctx) in "tracker: NEVER RAN, "*) ok "no stamp -> NEVER RAN" ;; *) fail "no stamp: $(ctx)" ;; esac
gen "$O" >/dev/null 2>&1
gen --mark "$O" >/dev/null 2>&1; expect_rc 0 $? "--mark stamps the blob the datasets were built from"
case $(ctx) in "tracker: OK, current with docs/roadmap.md") ok "a stamp matching the roadmap -> OK" ;; *) fail "after --mark: $(ctx)" ;; esac
# add_row ID: the roadmap gains a queued row after B-7, so its blob changes.
add_row() {
  echo "| $1 | \`ops/$1\` — added after the upload | x | — | ⬜ queued |" > "$(scratch)/row"
  sed '/^| B-7 /r '"$(scratch)/row" "$F/docs/roadmap.md" > "$(scratch)/rm" && cp "$(scratch)/rm" "$F/docs/roadmap.md"
}
was=$(git -C "$F" hash-object docs/roadmap.md | cut -c1-7)
add_row C-1
now=$(git -C "$F" hash-object docs/roadmap.md | cut -c1-7)
case $(ctx) in "tracker: STALE, built from $was, roadmap.md is now $now; refresh it (session-start step)") ok "a stamp from another roadmap -> STALE, naming both blobs" ;; *) fail "after an edit: $(ctx)" ;; esac
# The stamp holds the blob the data was BUILT from, not the roadmap at mark time: an edit between
# the build and the upload's --mark must still read STALE.
gen "$O" >/dev/null 2>&1
built=$(git -C "$F" hash-object docs/roadmap.md | cut -c1-7)
add_row C-2
gen --mark "$O" >/dev/null 2>&1
case $(ctx) in "tracker: STALE, built from $built, "*) ok "an edit between the build and --mark -> STALE (the stamp is the built blob)" ;; *) fail "an edit between the build and --mark: $(ctx)" ;; esac
s="$(git -C "$F" rev-parse --path-format=absolute --git-common-dir)/clauductor/tracker-refreshed"
cp "$s" "$(scratch)/stamp.before"
rm -rf "$(scratch)/none"
gen --mark "$(scratch)/none" >/dev/null 2>&1 && fail "--mark stamped with nothing built" || ok "--mark refuses a directory it did not build"
cmp -s "$s" "$(scratch)/stamp.before" && ok "...and leaves the stamp byte for byte as it was" || fail "a refused --mark changed the stamp"
# A build removes the blob a previous build left, so a build that fails is never marked.
gen "$O" >/dev/null 2>&1
printf 'not a roadmap\n' > "$F/docs/roadmap.md"
gen "$O" >/dev/null 2>&1 && fail "the generator built a roadmap with no Session plan"
gen --mark "$O" >/dev/null 2>&1 && fail "--mark stamped the blob of an earlier build after a failed one" || ok "a failed build leaves nothing to mark"

# The Session plan reader refuses what it cannot read, naming it, rather than planning less.
plan_err() { # NAME SED-EXPR WANT
  sed "$2" "$(scratch)/fixture.md" > "$F/docs/roadmap.md"
  e=$(gen "$O" 2>&1 >/dev/null); r=$?
  if [ "$r" -ne 0 ] && printf '%s' "$e" | grep -q "$3"; then ok "the plan reader refuses $1"; else fail "the plan reader accepted $1 (exit $r): $e"; fi
}
plan_err "a row with the wrong cell count" 's/^| S1 | today | First things |/| S1 | today |/' "exactly 5 cells"
plan_err "a session planned twice" 's/^| S2–3 |/| S1 |/' "S1 is planned twice"
plan_err "an id planned twice" 's/| B-2, B-7, Z-9 |/| B-2, B-7, A-2 |/' "A-2 is planned in S1 and S2–3"

# The session-plan section names the first session not "(done)", with its rows in the plan's
# order, not the queue's table order; a plan it cannot read is CANNOT CHECK, never a shorter plan.
plan() { CONTEXT_OFFLINE=1 sh "$F/.claude/local/context.d/session-start/session-plan.sh" 2>&1; }
cp "$(scratch)/fixture.md" "$F/docs/roadmap.md"
case $(plan) in "session plan: S1 (today) — First things"*) ok "the section names the first session not done" ;; *) fail "the first session: $(plan)" ;; esac
printf '# a comment\n\nB-1\tS1: B-1 routine\nA-2\tS1: A-2 routine\nB-2\nB-3 spaces not a tab\nZ-8\tS1: no such row\n' > "$F/.claude/local/cloud-lanes.tsv"
e=$(gen "$O" 2>&1 >/dev/null)
printf '%s\n' "$e" | grep -q "cloud-lanes.tsv line 5 is not <id> TAB <routine name>" && printf '%s\n' "$e" | grep -q "cloud-lanes.tsv line 6 is not" \
  && ok "a cloud-lanes line without a TAB-separated routine name is warned about, by line" || fail "malformed cloud-lanes lines: $e"
printf '%s\n' "$e" | grep -q "cloud-lanes.tsv names Z-8, which is no roadmap row" && ok "...and one naming no roadmap row" || fail "an unknown cloud-lanes id: $e"
[ -e "$O/.cloud" ] && fail "the generator left .cloud in its output" || ok "...and the generator leaves no .cloud behind"
case $(jq -r '.[] | select(.id == "B-1") | .note' "$O/roadmap.json") in *"cloud: S1: B-1 routine") ok "a listed open row is noted as a cloud lane" ;; *) fail "B-1's note: $(jq -c '.[] | select(.id == "B-1")' "$O/roadmap.json")" ;; esac
case $(jq -r '.[] | select(.id == "A-2") | .note' "$O/roadmap.json") in *cloud*) fail "a merged row is still noted as a cloud lane" ;; *) ok "...a merged one is not" ;; esac
case $(jq -r '.[] | select(.id == "B-2") | .note' "$O/roadmap.json") in *cloud*) fail "a line without a routine name made a cloud lane" ;; *) ok "...nor a line without a routine name" ;; esac
plan | grep -qF "cloud lanes, run their routines first (session-start step): B-1 (S1: B-1 routine)" && ok "the session-plan section names the session's cloud routines" || fail "the cloud line: $(plan)"
rm "$F/.claude/local/cloud-lanes.tsv"
plan | grep -q "cloud lanes" && fail "with no cloud lanes, the section still names some" || ok "...and none when there are none"
sed 's/^| S1 | today |/| S1 | before (done) |/; s/| B-2, B-7, Z-9 |/| B-7, B-2, Z-9 |/' "$(scratch)/fixture.md" > "$F/docs/roadmap.md"
p=$(plan)
case $p in "session plan: S2–3 — Then the rest"*) ok "...and skips a session marked (done)" ;; *) fail "S1 done: $p" ;; esac
# B-7 before B-2 is neither the table's order nor the ids' sort: only the plan puts it there.
printf '%s\n' "$p" | grep -qF "rows, in the plan's order: B-7 [running], B-2 [queued]" && ok "...listing its rows in the plan's order, with their states" || fail "the rows line: $p"
printf '%s\n' "$p" | grep -qF "warning: the Session plan names Z-9, which is no roadmap row" && ok "...and saying which planned row it left out" || fail "the left-out row is not said: $p"
sed 's/^| S1 | today |/| S1 | before (done) |/; s/^| S2–3 | |/| S2–3 | after (done) |/' "$(scratch)/fixture.md" > "$F/docs/roadmap.md"
case $(plan) in "session plan: every planned session is done"*) ok "every session done -> it says so" ;; *) fail "all done: $(plan)" ;; esac
sed 's/^| S1 | today | First things |/| S1 | today |/' "$(scratch)/fixture.md" > "$F/docs/roadmap.md"
case $(plan) in "session plan: CANNOT CHECK — "*) ok "an unreadable plan -> CANNOT CHECK" ;; *) fail "an unreadable plan: $(plan)" ;; esac
finish
