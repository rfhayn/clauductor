#!/bin/sh
# The people module (.claude/modules/people), whether it is on here or not, in a throwaway project
# with the module on and fixture people, roadmap and GitHub reads:
#   - who is on what (people.sh --activity): a block per person in the registry and only them;
#     Last = the most recently MERGED PR whatever order gh lists; Now = open PRs and PR-less
#     branches by login, with the row each names; Next = the first unfinished row the person owns
#     that NOBODY has a PR or branch for, never a deferred one; ownership from the roadmap's
#     **Owner:** lines through the one parser (a malformed row is the parser's own error); a read
#     that failed says CANNOT CHECK, never none; unmapped work is named, bots left out;
#   - the registry fails loudly (a missing key, a name or login twice, malformed lanes), a flag
#     with no value is refused, and the lane table is drawn from the registry;
#   - the roadmap owner rule (roadmap.d/owners.sh) makes the queue UNKNOWN for an owner who is not
#     a person, on every mode, and PEOPLE_OWNERS=off turns it off;
#   - the production path: session-start's context (extensions.sh) runs who.sh, which reads GitHub
#     (a stub gh here) and hands the reads to people.sh, and says CANNOT CHECK offline.
# Ported from Standing Tee's vitest people-activity cases; the fixtures name nobody real (Alice
# and Bob), so a script that hard-coded a person or read a real registry would fail here.
. "$(dirname "$0")/lib.sh"
need git jq

M="$ROOT/.claude/modules/people"
d=$(scratch)
R="$d/p"; new_repo "$R"
mkdir -p "$R/.claude/lib" "$R/.claude/checks" "$R/docs" "$R/.claude/modules" "$d/bin" "$d/fix"
cp "$ROOT/.claude/lib/conf.sh" "$ROOT/.claude/lib/modules.sh" "$R/.claude/lib/"
cp "$ROOT/.claude/extensions.sh" "$ROOT/.claude/roadmap-queue.sh" "$R/.claude/"
cp "$ROOT/.claude/checks/run.sh" "$ROOT/.claude/checks/lib.sh" "$R/.claude/checks/"
cp -R "$M" "$R/.claude/modules/people"
printf 'MODULES="people"\n' > "$R/.claude/project.conf"
PS="$R/.claude/modules/people/people.sh"

people() {  # people [extra JSON for the top level]
  printf '{"people":[{"name":"Alice","github":"alice-gh","role":"founder"},{"name":"Bob","github":"bob-gh","role":"designer"}]%s}\n' "${1:-}" > "$R/docs/people.json"
}
roadmap() {  # roadmap [owner of Gate 2U]
  cat > "$R/docs/roadmap.md" <<EOF
## Phase 2 — the build
**Owner:** Alice

### Gate 2B
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 2B.1 | \`add-done\` — finished | scope | — | ✅ merged (#1) |

### Gate 2C-L
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 2C.8 | \`add-alpha\` — the alpha thing | scope | — | ⬜ queued |
| 2C.9 | \`add-beta\` — the beta thing | scope | — | ⬜ queued |
| 2C.10 | \`add-gamma\` — the gamma thing | scope | — | ⬜ queued |

### Gate 2U
**Owner:** ${1:-Bob}
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 2U.1 | \`design/one\` — the first design | scope | — | ⬜ queued |
| 2U.2 | \`design/two\` — the second design | scope | — | ⬜ queued |

## Phase 3 — later
**Owner:** Alice
| # | Change | Scope | Deps | Status |
|---|---|---|---|---|
| 2C.5 | \`add-later\` — later work | scope | — | ⬜ queued |
EOF
}
people; roadmap

# The fixture world: Alice has a PR on 2C.8, Bob a PR-less branch on 2U.1; an unmapped login, a
# bot, a branch with no account and one whose lookup failed. Alice's merged PRs are listed out of
# mergedAt order on purpose: gh lists by creation (or update), and #7 merged after #8.
OPEN='[{"number":10,"title":"Build the alpha thing","headRefName":"change/add-alpha","author":{"login":"alice-gh","is_bot":false}},
 {"number":11,"title":"A fix by someone not in people.json","headRefName":"fix/stray","author":{"login":"carol-gh","is_bot":false}},
 {"number":12,"title":"Bump x","headRefName":"dependabot/npm/x","author":{"is_bot":true}}]'
BRANCHES=$(printf 'design/one\tbob-gh\t2026-09-26\nops/nobody\t\t2026-09-20\nops/unknown\t?\t2026-09-21')
MERGED_ALICE='[{"number":8,"title":"An older merge","headRefName":"ops/x","mergedAt":"2026-09-25T10:00:00Z"},
 {"number":7,"title":"Land the done thing","headRefName":"change/add-done","mergedAt":"2026-09-26T09:00:00Z"}]'

# world NAME [open|-] [branches|-] [merged-alice|-]: a state directory; "-" leaves that read
# failed (its file missing). Bob's merged list is always missing unless set by hand.
world() {
  w="$d/w-$1"; rm -rf "$w"; mkdir -p "$w"
  [ "${2:--}" = - ] || printf '%s\n' "$2" > "$w/open-prs.json"
  [ "${3:--}" = - ] || printf '%s\n' "$3" > "$w/branches.tsv"
  [ "${4:--}" = - ] || printf '%s\n' "$4" > "$w/merged-alice-gh.json"
  printf '%s' "$w"
}
act() { _st=$1; shift; (cd "$R" && sh "$PS" --activity --state "$_st" "$@") 2>&1; }
# inr SCRIPT: sh code run in the project with its own config read (ROOT is the check's otherwise).
# CLAUDUCTOR_FW names its .claude/ too: the plugin build's conf.sh finds its libraries through it.
inr() { (cd "$R" && ROOT="$R" CLAUDUCTOR_FW="$R/.claude" sh -c ". .claude/lib/conf.sh; $1") 2>&1; }
# blk NAME: from the output on stdin, NAME's block (header through the line before the next one).
blk() { awk -v n="$1 (" 'index($0, n) == 1 { on = 1; print; next } on && /^[^ ]/ { exit } on { print }'; }
has() {  # has yes|no NEEDLE HAYSTACK LABEL
  case "$3" in *"$2"*) got=yes ;; *) got=no ;; esac
  [ "$got" = "$1" ] && ok "$4" || fail "$4 (looked for '$2'): $(printf '%s' "$3" | head -c 600)"
}

W=$(world world "$OPEN" "$BRANCHES" "$MERGED_ALICE")
out=$(act "$W"); rc=$?
expect_rc 0 "$rc" "--activity runs on a full set of reads"
hdr=$(printf '%s\n' "$out" | grep -E '^[^ ].* \(.* · .*\)$' | tr '\n' '|')
[ "$hdr" = "Alice (alice-gh · founder)|Bob (bob-gh · designer)|" ] && ok "a block for every person in the registry, and only them" || fail "headers: $hdr"
alice=$(printf '%s\n' "$out" | blk Alice); bob=$(printf '%s\n' "$out" | blk Bob)
has yes "Last: #7, merged 2026-09-26 — Land the done thing → 2B.1 add-done" "$alice" "Last is the most recently MERGED PR, not the first gh lists, with its row"
has yes "#10 change/add-alpha — Build the alpha thing → 2C.8" "$alice" "Now lists a person's open PR with the row it names"
has yes "branch design/one (no PR, last commit 2026-09-26) → 2U.1" "$bob" "Now lists a person's PR-less branch with the row it names"
has yes "Next: 2C.9 add-beta — the beta thing (Gate 2C-L)" "$alice" "Next is the first unfinished row they own that is not under way (2C.8 has their PR)"
has yes "Next: 2U.2 design/two — the second design (Gate 2U)" "$bob" "Next follows the section's owner line (Bob's 2U.1 has his branch)"
tail=$(printf '%s\n' "$out" | sed -n '/^Not mapped to anyone/,$p')
has yes "#11 fix/stray by carol-gh" "$tail" "an open PR by a login nobody maps to is named, not dropped"
has yes "branch ops/nobody (no GitHub account)" "$tail" "a branch with no GitHub account is named"
has yes "branch ops/unknown (account lookup failed)" "$tail" "a branch whose account lookup failed is named"
has no "dependabot" "$tail" "bots are left out of the unmapped list"

# Last: an OLD PR merged most recently, listed LAST the way a creation-ordered list puts it.
W2=$(world oldmerge "$OPEN" "$BRANCHES" '[{"number":30,"title":"Recent open, early merge","headRefName":"ops/a","mergedAt":"2026-09-20T00:00:00Z"},{"number":29,"title":"Another","headRefName":"ops/b","mergedAt":"2026-09-21T00:00:00Z"},{"number":2,"title":"Opened in July","headRefName":"change/add-beta","mergedAt":"2026-09-27T12:00:00Z"}]')
has yes "Last: #2, merged 2026-09-27 — Opened in July → 2C.9 add-beta" "$(act "$W2" | blk Alice)" "Last finds an old PR that merged most recently, wherever gh lists it"

# Next skips a row somebody ELSE has a PR or branch for: Bob's PR on 2C.9, an unmapped login's
# branch on 2C.10.
OPEN3=$(printf '%s' "$OPEN" | jq -c '. + [{"number":13,"title":"Bob helps with beta","headRefName":"change/add-beta","author":{"login":"bob-gh","is_bot":false}}]')
W3=$(world someoneelse "$OPEN3" "$(printf '%s\nchange/add-gamma\tcarol-gh\t2026-09-27' "$BRANCHES")" "$MERGED_ALICE")
has yes "Next: 2C.5 add-later — later work (Phase 3)" "$(act "$W3" | blk Alice)" "Next skips rows somebody else already has a PR or branch for (2C.9 Bob's PR, 2C.10 an unmapped branch)"

# Ownership follows the roadmap's owner line, not a rule about roles.
roadmap Alice
W4=$(world owneralice '[]' '' "$MERGED_ALICE")
o4=$(act "$W4")
has yes "Next: no unfinished roadmap row names them as owner" "$(printf '%s\n' "$o4" | blk Bob)" "ownership follows the owner line: Bob, the designer, owns nothing once 2U is Alice's"
has yes "Next: 2C.8 add-alpha" "$(printf '%s\n' "$o4" | blk Alice)" "...and Alice's Next is her first row"
roadmap

# A read that failed says CANNOT CHECK, never none.
W5=$(world unreadable - - '[]')
o5=$(act "$W5"); bob=$(printf '%s\n' "$o5" | blk Bob)
has yes "Last: CANNOT CHECK" "$bob" "a missing merged-PR list says CANNOT CHECK"
has yes "CANNOT CHECK — the open-PR list could not be read" "$bob" "a missing open-PR list says CANNOT CHECK"
has yes "CANNOT CHECK — the remote branches with no PR could not be read" "$bob" "a missing branch list says CANNOT CHECK"
has yes "may be under way" "$bob" "Next says it may be under way when Now could not be read"
has no "nothing open" "$bob" "an unreadable Now never reads as nothing open"
has yes "Last: nothing merged yet" "$(printf '%s\n' "$o5" | blk Alice)" "an empty merged list (a read that succeeded) is nothing merged yet"

# A malformed roadmap row reaches the output as the parser's own error, not a shorter queue.
sed -i.bak 's/^| 2C.9 | `add-beta` — the beta thing | scope | — | ⬜ queued |$/| 2C.9 | `add-beta` — the beta thing | scope | ⬜ queued |/' "$R/docs/roadmap.md" && rm -f "$R/docs/roadmap.md.bak"
grep -q '^| 2C.9 .* scope | ⬜ queued |$' "$R/docs/roadmap.md" || fail "fixture: the malformed row was not written"
o6=$(act "$W"); rc=$?
expect_rc 1 "$rc" "a malformed roadmap row fails --activity"
has yes "CANNOT CHECK" "$o6" "...saying CANNOT CHECK"
has yes "a queue row needs exactly 5 cells, this has 4" "$o6" "...with the parser's own error"
roadmap

# Deferred: a plugged parser (ROADMAP_PARSER) emitting 14 columns, the 14th the raw status, keeps a
# deferred row from anyone's Next. Column 13 is another field (the builtin parser's own): a status
# there is NOT read as one.
inr 'roadmap_queue --tsv' > "$d/rows.tsv"
cut -f1-12 "$d/rows.tsv" > "$d/rows12.tsv"
door=.claude/roadmap-queue
cat > "$R/.claude/parser14.sh" <<EOF
#!/bin/sh
# A project's own parser: the builtin's rows, then column 13 empty and column 14 the raw status.
case "\${1:-}" in
  --tsv) shift; ROADMAP_BUILTIN=1 sh $door.sh --tsv "\$@" | cut -f1-12 | awk -F'\t' -v OFS='\t' -v c="\${COL:-14}" '{ s = (\$4 == "2C.9") ? "⬜ deferred — trigger: a league asks" : "⬜ queued"; if (c == 13) print \$0, s; else if (c == "date") print \$0, "2026-07-21", s; else print \$0, "", s }' ;;
  *) ROADMAP_BUILTIN=1 sh $door.sh "\$@" ;;
esac
EOF
printf 'MODULES="people"\nROADMAP_PARSER="sh .claude/parser14.sh"\n' > "$R/.claude/project.conf"
[ "$(inr 'roadmap_queue --tsv' | awk -F'\t' '$4 == "2C.9" { print NF }')" = 14 ] && ok "fixture: the plugged parser emits 14 columns" || fail "fixture: the plugged parser's rows: $(inr 'roadmap_queue --tsv' | head -3)"
has yes "Next: 2C.10 add-gamma" "$(act "$W" | blk Alice)" "a 14-column parser's deferred row (column 14) is skipped for Next"
# Column 13 is the builtin's `started` date (docs/roadmap.md): with a valid date there and the
# deferred status in column 14, the module still reads column 14; a status in column 13 is refused
# by the contract before the module runs.
has yes "Next: 2C.10 add-gamma" "$(cd "$R" && COL=date sh "$PS" --activity --state "$W" 2>&1 | blk Alice)" "...with a started date in column 13, the status is still read from column 14"
o13=$(cd "$R" && COL=13 sh "$PS" --activity --state "$W" 2>&1)
has yes "column 13 (started)" "$o13" "...and a status in column 13 is refused by the contract, never read as the row's status"
has yes "CANNOT CHECK" "$o13" "...so the activity says CANNOT CHECK"
printf 'MODULES="people"\n' > "$R/.claude/project.conf"
has yes "Next: 2C.9 add-beta" "$(act "$W" --rows "$d/rows12.tsv" | blk Alice)" "...and a 12-column row has no status: Next as before"

# --logins, the registry and the flags.
[ "$(cd "$R" && sh "$PS" --logins | tr '\n' ' ')" = "alice-gh bob-gh " ] && ok "--logins prints the registry's logins, which who.sh loops over" || fail "--logins: $(cd "$R" && sh "$PS" --logins 2>&1)"
for argv in "--logins --people" "--people --logins" "--activity --state"; do
  # shellcheck disable=SC2086
  o=$(cd "$R" && sh "$PS" $argv 2>&1); rc=$?
  [ "$rc" -eq 1 ] && case "$o" in *"needs a value"*) true ;; *) false ;; esac && ok "refuses '$argv': a flag with no value" || fail "'$argv' (exit $rc): $o"
done
reg() {  # reg JSON NEEDLE LABEL: the registry JSON must fail --check, naming NEEDLE
  printf '%s\n' "$1" > "$d/bad.json"
  o=$(sh "$PS" --check --people "$d/bad.json" 2>&1); rc=$?
  [ "$rc" -eq 1 ] && case "$o" in *"$2"*) true ;; *) false ;; esac && ok "the registry refuses $3" || fail "registry: $3 (exit $rc): $o"
}
reg '{"people":[]}' 'expected a non-empty "people" array' "an empty people list"
reg '{"people":[{"name":"A","github":"a","role":" "}]}' 'a person has no "role"' "a person with a blank role"
reg '{"people":[{"name":"A","github":"Ab","role":"x"},{"name":"B","github":"aB","role":"y"}]}' 'github:ab is listed twice' "one login twice, whatever its case"
reg '{"people":[{"name":"A","github":"a","role":"x"},{"name":"A","github":"b","role":"y"}]}' 'name:A is listed twice' "one name twice"
reg '{"people":[{"name":"A","github":"a","role":"x"}],"lanes":[{"lane":"x"}]}' 'a lane has no "owns"' "a lane with nothing it owns"
reg 'not json' 'is not valid JSON' "a file that is not JSON"
o=$(sh "$PS" --activity --state "$W" --people "$d/missing.json" 2>&1); rc=$?
[ "$rc" -eq 1 ] && case "$o" in *"CANNOT CHECK — $d/missing.json is missing"*) true ;; *) false ;; esac && ok "a missing registry is CANNOT CHECK, never an empty team" || fail "missing registry (exit $rc): $o"
o=$(sh "$PS" --check --people "$M/people.example.json" 2>&1) && ok "the module's people.example.json passes --check" || fail "people.example.json: $o"
o=$(sh "$PS" --lanes --people "$M/people.example.json" 2>&1)
has yes "| **designer** | the design system" "$o" "--lanes draws the lane table from the registry's lanes"
has yes "none: docs/people.json lists no lanes" "$(cd "$R" && sh "$PS" --lanes 2>&1)" "--lanes says none, by name, when the registry has no lanes"

# The roadmap owner rule: on with the module, on every mode, and off by key.
roadmap Mallory
for m in --check --text --tsv; do
  o=$(inr "roadmap_queue $m; echo rc=\$?")
  has yes "rc=1" "$o" "an owner who is not in the registry makes the queue UNKNOWN (roadmap_queue $m)"
done
has yes 'owner "Mallory" is not in docs/people.json (Alice, Bob)' "$o" "...naming the row's owner and the people there are"
# The parser's front door, by path on purpose (the bypass scan in checks/roadmap.sh matches the
# literal path, which readers must not use).
door=.claude/roadmap-queue
o=$(cd "$R" && sh "$door.sh" --check 2>&1; echo "rc=$?")
has yes "rc=1" "$o" "...and on a direct call of the parser's front door"
o=$(cd "$R" && sh .claude/checks/run.sh people:registry 2>&1); rc=$?
expect_rc 1 "$rc" "the module's check (people:registry) fails on that roadmap"
printf 'MODULES="people"\nPEOPLE_OWNERS="off"\n' > "$R/.claude/project.conf"
o=$(inr 'roadmap_queue --check; echo rc=$?')
has yes "rc=0" "$o" "PEOPLE_OWNERS=off turns the owner rule off"
printf 'MODULES=""\n' > "$R/.claude/project.conf"
o=$(inr 'roadmap_queue --check; echo rc=$?')
has yes "rc=0" "$o" "with the module off, the owner rule does not run"
printf 'MODULES="people"\n' > "$R/.claude/project.conf"
roadmap
o=$(cd "$R" && sh .claude/checks/run.sh people:registry 2>&1); rc=$?
expect_rc 0 "$rc" "people:registry passes on a sound registry and roadmap"

# The production path: session-start's context runs who.sh (extensions.sh context session-start),
# which reads GitHub through gh (a stub over the fixture world) and prints the block.
printf '%s\n' "$OPEN" > "$d/fix/open.json"; printf '%s\n' "$MERGED_ALICE" > "$d/fix/merged-alice.json"
cat > "$d/bin/gh" <<EOF
#!/bin/sh
# GH_FAIL names one read to fail (prs | branches); GH_SLOW sleeps on design/one's lookup; GH_FULL
# answers the open-PR list with a full page of 100.
case "\${GH_FAIL:-}:\$*" in
  "prs:pr list --state open "*) exit 1 ;;
  "branches:api --paginate "*) exit 1 ;;
esac
case "\$*" in
  "pr list --state open "*) if [ -n "\${GH_FULL:-}" ]; then jq -n '[range(100) | {number: (. + 1000), title: "t", headRefName: "ops/n\(.)", author: {login: "alice-gh", is_bot: false}}]'; else cat "$d/fix/open.json"; fi ;;
  "api --paginate repos/{owner}/{repo}/branches"*) printf 'main\nchange/add-alpha\ndesign/one\nops/nobody\n' ;;
  "api repos/{owner}/{repo}/commits/design/one "*) [ -z "\${GH_SLOW:-}" ] || sleep 8 >/dev/null; printf 'bob-gh\t2026-09-26\n' ;;
  "api repos/{owner}/{repo}/commits/ops/nobody "*) printf '\t2026-09-20\n' ;;
  "pr list --state merged --author alice-gh --search sort:updated-desc "*) cat "$d/fix/merged-alice.json" ;;
  *) exit 1 ;;
esac
EOF
chmod +x "$d/bin/gh"
c=$(cd "$R" && CONTEXT_OFFLINE= PATH="$d/bin:$PATH" sh .claude/extensions.sh context session-start 2>&1)
has yes "- who (module people):" "$c" "session-start's context prints the who section while the module is on"
has yes "Last: #7, merged 2026-09-26 — Land the done thing → 2B.1 add-done" "$c" "...from GitHub's merged list, sorted by mergedAt"
has yes "branch design/one (no PR, last commit 2026-09-26) → 2U.1" "$c" "...with a PR-less branch tied to its person by GitHub's account"
has no "branch change/add-alpha" "$c" "...and a branch with an open PR is not listed as PR-less"
has yes "Last: CANNOT CHECK" "$c" "...and Bob's failed merged read (the stub has none) says CANNOT CHECK"
has yes "- lanes (module people):" "$c" "session-start's context prints the lanes section"
has no "hit its limit" "$c" "a short open-PR list says nothing about the limit"
# A read that FAILS writes no file, so it says CANNOT CHECK; it never becomes an empty list.
ctx() { (cd "$R" && CONTEXT_OFFLINE= PATH="$d/bin:$PATH" env "$@" sh .claude/extensions.sh context session-start 2>&1); }
c=$(ctx GH_FAIL=prs)
has yes "CANNOT CHECK — the open-PR list could not be read" "$c" "who.sh: a failed open-PR read says CANNOT CHECK, never nothing open"
has yes "CANNOT CHECK — the remote branches with no PR could not be read" "$c" "who.sh: ...and skips the branches (each would look PR-less)"
c=$(ctx GH_FAIL=branches)
has yes "CANNOT CHECK — the remote branches with no PR could not be read" "$c" "who.sh: a failed branch read says CANNOT CHECK, never no branches"
has no "branch design/one" "$c" "who.sh: ...and lists no branch"
has yes "#10 change/add-alpha" "$c" "who.sh: ...while the open PRs that were read still show"
# L1: a full page of open PRs is named, not silently truncated.
c=$(ctx GH_FULL=1)
has yes "CANNOT CHECK — the open-PR list hit its limit of 100" "$c" "who.sh: a full page of open PRs says the rest are unread"
# L2: each gh call is bounded; a slow lookup is a named timeout, not a stalled session-start.
c=$(ctx GH_SLOW=1 PEOPLE_GH_TIMEOUT=1)
has yes "CANNOT CHECK — gh timed out after 1s reading branch design/one's last commit" "$c" "who.sh: a gh call past PEOPLE_GH_TIMEOUT is cut off and named"
has yes "branch design/one (account lookup failed)" "$c" "who.sh: ...and that branch reads as a failed lookup"
# The watchdog of a call that returned in time leaves nothing running: no sleep survives it. The
# timeout is a value nothing else on the machine sleeps for, so the count is ours: it carries this
# run's PID, because go test runs this check twice at once (the template's and the plugin's), and
# a shared value counted, then killed, the other run's live watchdog (#63). Under macOS sleep's
# INT_MAX limit for any PID.
# A decoy stands in for the other run's watchdog, so a shared value fails here on every run, not
# only when go test happens to overlap the two: it must be neither counted nor killed. It sleeps
# for the value this check shared before #63, the one a regression would most likely bring back.
# lib.sh's traps know only the scratch directory, and an async sleep ignores the Ctrl-C that ends
# this check, so until the decoy is gone the traps take it too, or an interrupt leaves it for 2h.
# The wait reaps it, so bash 3.2 prints no "Terminated" notice for it.
wd=$((7000000 + $$))
sleep 7373 </dev/null >/dev/null 2>&1 & decoy=$!
trap '{ kill "$decoy"; wait "$decoy"; } 2>/dev/null; rm -rf "$_scratch"' EXIT
trap '{ kill "$decoy"; wait "$decoy"; } 2>/dev/null; rm -rf "$_scratch"; exit 1' INT TERM
c=$(ctx PEOPLE_GH_TIMEOUT="$wd")
sleep 1
if ! psout=$(ps -A -o pid= -o args= 2>/dev/null) || [ -z "$psout" ]; then
  fail "cannot list processes (ps), so the watchdog count and cleanup are unchecked"
else
  # This run's watchdog sleeps, by PID: the count and the cleanup read the one list.
  mine=$(printf '%s\n' "$psout" | awk -v wd="$wd" '$2 == "sleep" && $3 == wd && NF == 3 { print $1 }')
  n=$(printf '%s\n' "$mine" | grep -cvx -e '' -e "$decoy")
  [ "$n" = 0 ] && ok "who.sh: a gh call that returns in time leaves no watchdog sleep behind" || fail "who.sh: $n watchdog sleep(s) outlived their gh calls"
  printf '%s\n' "$mine" | grep -qx "$decoy" \
    && fail "the watchdog count and cleanup reach another run's sleep (the decoy was counted): the value is shared, not this run's own"
  for p in $mine; do kill "$p" 2>/dev/null; done
  # Not kill -0: a killed decoy is this shell's zombie until reaped, and kill -0 reaches a zombie.
  case $(ps -o stat= -p "$decoy" 2>/dev/null | tr -d ' ') in
    '' | Z*) fail "the watchdog count and cleanup reach another run's sleep (the decoy was killed): the value is shared, not this run's own" ;;
    *) ok "...and the count and cleanup are this run's own: another run's watchdog survives them" ;;
  esac
fi
{ kill "$decoy"; wait "$decoy"; } 2>/dev/null
# lib.sh's own traps, restored: these two lines must stay word for word what lib.sh sets.
trap 'rm -rf "$_scratch"' EXIT
trap 'rm -rf "$_scratch"; exit 1' INT TERM
c=$(ctx PEOPLE_WHO_BUDGET=0)
has yes "CANNOT CHECK — the branch lookups ran past 0s" "$c" "who.sh: the branch lookups stop at PEOPLE_WHO_BUDGET..."
has yes "CANNOT CHECK — the remote branches with no PR could not be read" "$c" "who.sh: ...and the partial branch list is not handed over"
c=$(cd "$R" && sh .claude/extensions.sh context session-start 2>&1)
has yes "CANNOT CHECK — offline" "$c" "who.sh honours CONTEXT_OFFLINE: no network, says so"
f=$(cd "$R" && sh .claude/extensions.sh fragments session-start 2>&1)
has yes "Who is on what" "$f" "session-start's Project steps include the module's fragment"
t=$(cd "$R" && sh .claude/extensions.sh conflicts 2>&1)
has yes "the people registry" "$t" "session-close's conflict table carries the registry row"
printf 'MODULES=""\n' > "$R/.claude/project.conf"
c=$(cd "$R" && CONTEXT_OFFLINE= PATH="$d/bin:$PATH" sh .claude/extensions.sh context session-start 2>&1)
has no "who (module people)" "$c" "with the module off, session-start prints no who section"

# who.sh's own wiring, comments stripped (a claim in a comment satisfies nothing).
code=$(sed 's/^[[:space:]]*#.*$//; s/[[:space:]]#.*$//' "$M/context.d/session-start/who.sh")
has yes 'pr list --state merged --author "$login" --search "sort:updated-desc"' "$code" "who.sh reads merged PRs by most recent update, so an old PR merged today is found"
has yes 'state=$(mktemp ' "$code" "who.sh makes its state directory with mktemp..."
case "$code" in *'state=$(mktemp '*') || state='*) ok "...and survives a failed mktemp instead of ending the briefing" ;; *) fail "who.sh: a bare mktemp ends the briefing on failure" ;; esac
has yes 'CANNOT CHECK — mktemp failed' "$code" "...saying CANNOT CHECK when it fails"
has yes '--activity --state "$state" 2>&1' "$code" "who.sh runs people.sh with stderr merged, so its CANNOT CHECK is visible"
for login in $(sh "$PS" --logins --people "$M/people.example.json") alice-gh bob-gh; do
  has no "$login" "$code" "who.sh names no login ($login): every one comes from the registry"
done
finish
