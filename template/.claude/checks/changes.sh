#!/bin/sh
# Every open change in CHANGES_DIR has the shape changes/README.md gives, so build-change, the
# trace and the merge guard can read it, and the living specs keep the format specs/README.md gives.
# The set is the directory, so a new change is checked the moment it exists.
#
# proposal.md: the owner's approval, recorded over THIS design (`· design <hash>`, D8), or
#   `**Status:** awaiting approval` (which fails: nothing merges unapproved); the review-page line
#   when REVIEW_PAGE=artifact; `**Risk:** low|normal|high` (item 15); `**Budget:** $N` repeating the
#   roadmap row's budget when the row has one (item 12); `## How we'll know` with a signal and a
#   date or a delay (item 13).
# tasks.md: `## N.` groups, `## Progress` and `## Decision log` (D6), a Slice line.
# spec deltas: every requirement has a SHALL or MUST line and a scenario; every scenario is
#   `#### Scenario: [CAP-n-Sn] title` with a THEN bullet (D3); an ID is never taken twice, in a file,
#   across the living specs, across open changes, or from the ID history (the living specs and every
#   archived delta: a retired ID is never reused); one requirement's scenarios share `CAP-n`; a
#   MODIFIED requirement exists and copies every current scenario header word for word (OpenSpec
#   1.2.0 silently deletes one left out); a REMOVED one exists; an ADDED one does not already; a new
#   capability's delta has a `## Purpose` of 50 characters or more; a change with no delta says so
#   in `.openspec.yaml` (`skip_specs: true`).
. "$(dirname "$0")/lib.sh"
. "$ROOT/.claude/lib/change.sh"

# ── Self-test: a well-formed change passes, and each broken shape fails ──────────────────────────
# Built from the example change (.claude/examples), so the example is held to every rule too.
if [ -z "${CHANGES_SELFTEST:-}" ]; then
  me="$ROOT/.claude/checks/changes.sh"
  EX="$ROOT/.claude/examples"
  C=changes/add-greeting-name
  mk() {  # mk NAME: a fixture project holding the example change, approved; returned in $F
    F="$(scratch)/$1"; mkdir -p "$F/.claude/lib" "$F/.claude/checks" "$F/changes" "$F/specs" "$F/docs"
    cp "$ROOT/.claude/lib/conf.sh" "$ROOT/.claude/lib/change.sh" "$F/.claude/lib/"
    cp "$ROOT/.claude/checks/lib.sh" "$F/.claude/checks/"
    cp "$ROOT/.claude/roadmap-queue.sh" "$ROOT/.claude/change-approval.sh" "$F/.claude/"
    cp -R "$EX/changes/add-greeting-name" "$F/changes/"; cp -R "$EX/specs/greeting" "$F/specs/"
    printf '## Phase 1 — x\n| # | Change | Scope | Deps | Status |\n|---|---|---|---|---|\n| 1.1 | `add-greeting-name` — a member sees their name | s · Budget: $15 | — | ⬜ queued |\n' > "$F/docs/roadmap.md"
    (ROOT="$F" APPROVAL_DATE=2026-01-02 sh "$F/.claude/change-approval.sh" add-greeting-name --record Ana >/dev/null)
  }
  run_on() { (ROOT="$F" CHANGES_SELFTEST=1 sh "$me" > "$(scratch)/selftest.out" 2>&1); }
  # case_ok LABEL / case_bad LABEL: run on $F; the label says what the fixture is.
  case_ok() { if run_on; then ok "self-test: $1 passes"; else fail "self-test: $1 FAILED:"; grep '^FAIL' "$(scratch)/selftest.out" | sed 's/^/       /'; fi; }
  # case_bad LABEL WORDS: it must fail, and for the reason WORDS (a fixed string in a FAIL line), not
  # for some other breakage the edit caused (*A falsification can fail at any link in its chain*).
  case_bad() {
    if run_on; then fail "self-test: $1 passed"
    elif grep '^FAIL' "$(scratch)/selftest.out" | grep -qF -- "$2"; then ok "self-test: $1 fails ($2)"
    else fail "self-test: $1 fails, but not for '$2':"; grep '^FAIL' "$(scratch)/selftest.out" | sed 's/^/       /'; fi
  }
  edit() { f=$1; shift; sed "$@" "$F/$f" > "$F/$f.new" && mv "$F/$f.new" "$F/$f"; }

  mk good; case_ok "the example change, approved"
  mk awaiting; (ROOT="$F" sh "$F/.claude/change-approval.sh" add-greeting-name --revoke >/dev/null); case_bad "a proposal awaiting approval" "awaiting the"
  mk noapproval; edit $C/proposal.md '/^\*\*Approved:\*\*/d'; case_bad "a proposal with no approval line" "proposal.md needs '**Approved:**"
  mk nohash; edit $C/proposal.md 's/ · design [0-9a-f]*$//'; case_bad "an approval with no design hash" "does not end"
  mk design; printf '\nA later thought.\n' >> "$F/$C/design.md"; case_bad "a design edited after approval" "CHANGED since the owner approved"
  mk risk; edit $C/proposal.md 's/^\*\*Risk:\*\* low/**Risk:** normal/'; case_bad "a risk tier changed after approval" "CHANGED since the owner approved"
  mk norisk; edit $C/proposal.md '/^\*\*Risk:\*\*/d'; (ROOT="$F" APPROVAL_DATE=2026-01-02 sh "$F/.claude/change-approval.sh" add-greeting-name --record Ana >/dev/null); case_bad "a proposal with no Risk line" "needs '**Risk:**"
  mk badrisk; edit $C/proposal.md 's/^\*\*Risk:\*\* low/**Risk:** medium/'; (ROOT="$F" APPROVAL_DATE=2026-01-02 sh "$F/.claude/change-approval.sh" add-greeting-name --record Ana >/dev/null); case_bad "a Risk tier that is not low, normal or high" "is not low, normal or high"
  mk budget; edit $C/proposal.md 's/^\*\*Budget:\*\* \$15/**Budget:** $25/'; case_bad "a proposal whose budget differs from its roadmap row's" "has 'Budget: \$15'"
  mk nobudget; edit $C/proposal.md '/^\*\*Budget:\*\*/d'; case_bad "a proposal that drops its roadmap row's budget" "proposal.md says 'nothing'"
  mk noknow; edit $C/proposal.md '/^## How we.ll know/,$d'; case_bad "a proposal with no How we'll know section" "no '## How we'll know' section"
  mk nowhen; edit $C/proposal.md '/Check after/d'; case_bad "a How we'll know with no date or delay" "Check on:"
  mk noprogress; edit $C/tasks.md '/^## Progress/d'; case_bad "tasks.md with no Progress section" "no '## Progress' section"
  mk nolog; edit $C/tasks.md '/^## Decision log/d'; case_bad "tasks.md with no Decision log section" "no '## Decision log' section"
  mk noslice; edit $C/tasks.md '/Slice:/d'; case_bad "tasks.md with no Slice line" "no Slice: line"
  mk nogroups; edit $C/tasks.md 's/^## \([0-9]\)\. /## Group \1 /'; case_bad "tasks.md with no task group heading" "no '## <n>. <title>' group heading"
  mk noscenario; printf '\n### Requirement: Wave\nThe system SHALL wave.\n' >> "$F/$C/specs/farewell/spec.md"; case_bad "a requirement with no scenario" "requirement without a scenario"
  mk noshall; edit $C/specs/farewell/spec.md 's/THE SYSTEM SHALL show/the page shows/'; case_bad "a requirement with no SHALL or MUST" "no SHALL or MUST line"
  mk noid; edit $C/specs/farewell/spec.md 's/^#### Scenario: \[FAREWELL-1-S1\] /#### Scenario: /'; case_bad "a scenario with no ID" "needs '[CAP-n-Sn] title'"
  mk badid; edit $C/specs/farewell/spec.md 's/\[FAREWELL-1-S1\]/[FAREWELL-1-1]/'; case_bad "a malformed scenario ID" "needs '[CAP-n-Sn] title'"
  mk dupid; edit $C/specs/farewell/spec.md 's/\[FAREWELL-1-S2\]/[FAREWELL-1-S1]/'; case_bad "a duplicate scenario ID" "is used twice"
  mk mixed; edit $C/specs/farewell/spec.md 's/\[FAREWELL-1-S2\]/[FAREWELL-2-S2]/'; case_bad "one requirement's scenarios under two requirement numbers" "mixes FAREWELL-1 and FAREWELL-2"
  mk reused; edit $C/specs/greeting/spec.md 's/\[GREETING-1-S2\]/[GREETING-1-S9]/'
  mkdir -p "$F/changes/archive/2025-01-01-old/specs/greeting"; printf '## REMOVED Requirements\n### Requirement: Old\n#### Scenario: [GREETING-1-S9] retired\n- **THEN** x\n' > "$F/changes/archive/2025-01-01-old/specs/greeting/spec.md"
  case_bad "a retired scenario ID reused" "GREETING-1-S9 is already taken"
  mk taken; edit $C/specs/farewell/spec.md 's/\[FAREWELL-1-S1\]/[GREETING-1-S1]/; s/\[FAREWELL-1-S2\]/[GREETING-1-S5]/'; case_bad "an ADDED scenario taking an ID the living specs hold" "GREETING-1-S1 is already taken"
  mk nothen; edit $C/specs/farewell/spec.md '/THEN\*\* the page shows "Goodbye, Ana"/d'; case_bad "a scenario with no THEN" "scenario without a THEN"
  mk dropped; edit $C/specs/greeting/spec.md '/GREETING-1-S1/,/THEN/d'; case_bad "a MODIFIED requirement that drops a current scenario" "leaves out or rewords"
  mk retitled; edit $C/specs/greeting/spec.md 's/\[GREETING-1-S1\] An anonymous visitor is greeted/[GREETING-1-S1] A stranger is greeted/'; case_bad "a MODIFIED requirement that rewords a current scenario header" "leaves out or rewords"
  mk nosuch; edit $C/specs/greeting/spec.md 's/^### Requirement: Greet every visitor/### Requirement: Greet nobody/'; case_bad "a MODIFIED requirement the living spec does not have" "MODIFIED requirement \"Greet nobody\" is not in"
  mk readded; edit $C/specs/greeting/spec.md 's/^## MODIFIED Requirements/## ADDED Requirements/'; case_bad "an ADDED requirement the living spec already has" "is already in"
  mk purpose; edit $C/specs/farewell/spec.md 's/^Says goodbye.*/Says goodbye./'; case_bad "a new capability whose Purpose is under 50 characters" "at least 50 characters"
  mk nopurpose; edit $C/specs/farewell/spec.md '/^## Purpose/,/^## ADDED/{/^## ADDED/!d;}'; case_bad "a new capability's delta with no Purpose" "at least 50 characters (it has 0)"
  mk nodelta; rm -rf "$F/$C/specs"; case_bad "a change with no spec delta and no skip_specs" "skip_specs: true"
  printf 'schema: spec-driven\nskip_specs: true\n' > "$F/$C/.openspec.yaml"; case_ok "a change with no spec delta that declares skip_specs: true"
  mk legacy; printf '\n### Requirement: Old behaviour\nThe system SHALL do the old thing.\n\n#### Scenario: written before IDs\n- **THEN** it works\n' >> "$F/specs/greeting/spec.md"
  case_bad "a living scenario with no ID (SCENARIO_IDS=required)" "'#### Scenario: written before IDs' needs"
  printf 'SCENARIO_IDS="new-only"\n' > "$F/.claude/project.conf"; case_ok "a living scenario with no ID under SCENARIO_IDS=new-only"
fi

dir="$ROOT/$CHANGES_DIR"
[ -d "$dir" ] || { fail "CHANGES_DIR $CHANGES_DIR does not exist"; finish; }
w="$(scratch)/work"; mkdir -p "$w"

# spec_ok FILE: every requirement has a scenario; a delta's added and modified ones a SHALL or MUST.
spec_ok() {
  awk '/^## (ADDED|MODIFIED|REMOVED|RENAMED) Requirements/ { if (open) { bad = 1; print "requirement without a scenario: " name } open = 0; sec = $2; next }
       /^### Requirement:/{ if (open) { bad = 1; print "requirement without a scenario: " name } open = 1; name = $0; if (shallneed && !shall) { bad = 1; print "no SHALL or MUST line: " prev } prev = $0; shall = 0; shallneed = (sec == "ADDED" || sec == "MODIFIED"); next }
       /SHALL|MUST/ { shall = 1 }
       /^#### Scenario:/{ open = 0 }
       /^## /{ if (open) { bad = 1; print "requirement without a scenario: " name } open = 0 }
       END { if (open) { bad = 1; print "requirement without a scenario: " name }
             if (shallneed && !shall) { bad = 1; print "no SHALL or MUST line: " prev } exit bad }' "$1"
}
# then_ok FILE: every scenario has a THEN bullet before the next heading.
then_ok() {
  awk '/^#### Scenario:/ { if (s && !t) { bad = 1; print "scenario without a THEN: " s } s = $0; t = 0; next }
       /^#/ { if (s && !t) { bad = 1; print "scenario without a THEN: " s } s = "" }
       /^[[:space:]]*- \*\*THEN\*\*/ { t = 1 }
       END { if (s && !t) { bad = 1; print "scenario without a THEN: " s } exit bad }' "$1"
}
# ids_ok FILE LABEL STRICT: IDs present (STRICT=1: on every scenario) and well formed, unique in the
# file, one CAP-n per requirement and one CAP per file.
ids_ok() {
  spec_malformed "$1" | while IFS="$(printf '\t')" read -r kind h; do
    [ "$kind" = none ] && [ "$3" != 1 ] && continue
    echo "FAIL $2: scenario header '$h' needs '[CAP-n-Sn] title' (docs: specs/README.md)"
  done
  spec_scenarios "$1" | awk -F'\t' -v l="$2" '$3 != "-" {
      if ($3 in seen) printf "FAIL %s: scenario ID %s is used twice\n", l, $3; seen[$3] = 1
      p = $3; sub(/-S[0-9]+$/, "", p); cap = p; sub(/-[0-9]+$/, "", cap)
      if (($2 in reqp) && reqp[$2] != p) printf "FAIL %s: requirement \"%s\" mixes %s and %s scenario IDs\n", l, $2, reqp[$2], p
      reqp[$2] = p
      if (fcap != "" && cap != fcap) printf "FAIL %s: scenario IDs name two capabilities, %s and %s\n", l, fcap, cap
      if (fcap == "") fcap = cap
    }'
}

# ── The ID history: every ID the living specs and archived deltas hold (a retired one included) ──
: > "$w/history"
for s in "$ROOT/$SPECS_DIR"/*/spec.md "$dir"/archive/*/specs/*/spec.md; do
  [ -f "$s" ] && spec_scenarios "$s" | awk -F'\t' '$3 != "-" { print $3 }' >> "$w/history"
done
sort -u "$w/history" -o "$w/history"

roadmap_tsv=$(sh "$ROOT/.claude/roadmap-queue.sh" --tsv 2>/dev/null) || roadmap_tsv=""
: > "$w/added-all"
n=0
for c in "$dir"/*/; do
  [ -d "$c" ] || continue
  c=${c%/}; id=$(basename "$c"); [ "$id" = archive ] && continue
  n=$((n + 1))
  for f in proposal.md design.md tasks.md; do [ -f "$c/$f" ] || fail "$id: no $f"; done

  # ── proposal.md ───────────────────────────────────────────────────────────────────────────────
  if [ -f "$c/proposal.md" ]; then
    p="$c/proposal.md"
    if [ "$REVIEW_PAGE" = artifact ]; then
      head -1 "$p" | grep -qE '^\*\*Review page:\*\* https://claude\.ai/artifact/[A-Za-z0-9_-]+$' \
        && ok "$id: proposal.md opens with its review page" || fail "$id: proposal.md line 1 must be '**Review page:** https://claude.ai/artifact/<id>' (REVIEW_PAGE=artifact)"
    fi
    # An unapproved proposal FAILS: this check is a gate step, and the merge guard wants a gate
    # receipt, so a proposal cannot reach main before the owner approves it. The approval covers
    # the design as written: an edit to design.md (or the Risk line) after it is a new design.
    if grep -qE '^\*\*Approved:\*\* ' "$p"; then
      out=$(ROOT="$ROOT" sh "$ROOT/.claude/change-approval.sh" "$id" 2>&1) \
        && ok "$id: approved by the $OWNER_ROLE, and the design is unchanged since ($out)" \
        || fail "$id: $out. Put it back in front of the $OWNER_ROLE: 'sh .claude/change-approval.sh $id --revoke', then --record once they approve"
    elif grep -qE '^\*\*Status:\*\* awaiting approval' "$p"; then fail "$id: awaiting the $OWNER_ROLE's approval; it cannot merge until they approve ('sh .claude/change-approval.sh $id --record <name>')"
    else fail "$id: proposal.md needs '**Approved:** YYYY-MM-DD by <owner> · design <hash>' (or '**Status:** awaiting approval' while it waits)"; fi
    risk=$(proposal_field "$p" Risk)
    case "$risk" in
      low|normal|high) ok "$id: risk tier $risk" ;;
      '') fail "$id: proposal.md needs '**Risk:** low | normal | high' (the tier picks the build's models)" ;;
      *) fail "$id: '**Risk:** $risk' is not low, normal or high" ;;
    esac
    budget=$(proposal_field "$p" Budget)
    [ -z "$budget" ] || printf '%s' "$budget" | grep -Eq '^\$[0-9]+(\.[0-9][0-9]?)?$' || fail "$id: '**Budget:** $budget' must read '\$<dollars>'"
    row=$(proposal_field "$p" "Roadmap row")
    if [ -n "$row" ] && [ -n "$roadmap_tsv" ]; then
      rb=$(printf '%s\n' "$roadmap_tsv" | awk -F'\t' -v r="$row" '$4 == r { print $11; exit }')
      if [ -n "$rb" ] && [ "\$$rb" != "$budget" ]; then fail "$id: roadmap row $row has 'Budget: \$$rb', proposal.md says '${budget:-nothing}' (repeat it as '**Budget:** \$$rb')"
      elif [ -n "$rb" ]; then ok "$id: budget $budget, as roadmap row $row says"; fi
    fi
    if grep -qE "^## How we.ll know" "$p"; then
      know=$(sed -n "/^## How we.ll know/,/^## /p" "$p")
      printf '%s\n' "$know" | grep -qE '^[[:space:]]*- \*\*Signal:\*\* .+' || fail "$id: How we'll know needs '- **Signal:** <what you will observe>'"
      printf '%s\n' "$know" | grep -qE '^[[:space:]]*- \*\*(Check on:\*\* [0-9]{4}-[0-9]{2}-[0-9]{2}|Check after:\*\* [0-9]+ days?)[[:space:]]*$' \
        && ok "$id: states how we'll know, and when to look" || fail "$id: How we'll know needs '- **Check on:** YYYY-MM-DD' or '- **Check after:** <n> days' (archive queues the outcome check then)"
    else
      fail "$id: proposal.md has no '## How we'll know' section (the signal, and when to look)"
    fi
  fi

  # ── tasks.md ──────────────────────────────────────────────────────────────────────────────────
  if [ -f "$c/tasks.md" ]; then
    grep -qE '^## [0-9]+\. ' "$c/tasks.md" && ok "$id: tasks.md has task groups" || fail "$id: tasks.md has no '## <n>. <title>' group heading"
    grep -qE "$_is_slice" "$c/tasks.md" \
      && ok "$id: tasks.md states its slice" || fail "$id: tasks.md has no Slice: line (pr-merge-guard rule 3 will refuse the PR)"
    for sec in "Progress" "Decision log"; do
      grep -qx "## $sec" "$c/tasks.md" && ok "$id: tasks.md keeps its $sec" || fail "$id: tasks.md has no '## $sec' section (the builder keeps it current, so a resumed lane continues from the file)"
    done
  fi

  # ── spec deltas ───────────────────────────────────────────────────────────────────────────────
  have_delta=""
  for s in "$c"/specs/*/spec.md; do
    [ -f "$s" ] || continue
    have_delta=1; cap=$(basename "$(dirname "$s")"); rel="$id: specs/$cap/spec.md"; living="$ROOT/$SPECS_DIR/$cap/spec.md"
    out=$(spec_ok "$s") && ok "$rel: every requirement has a scenario and a SHALL" || fail "$rel: $out"
    out=$(then_ok "$s") || fail "$rel: $out"
    ids_ok "$s" "$rel" 1 > "$w/ids"; cat "$w/ids"; _fails=$((_fails + $(grep -c '^FAIL' "$w/ids")))
    spec_scenarios "$s" > "$w/sc"
    # New IDs: every ADDED one, and every MODIFIED one the living requirement does not already hold.
    : > "$w/cur"; [ -f "$living" ] && spec_scenarios "$living" | awk -F'\t' '$3 != "-" { print $3 }' > "$w/cur"
    awk -F'\t' '$1 == "ADDED" && $3 != "-" { print $3 }' "$w/sc" > "$w/new"
    awk -F'\t' '$1 == "MODIFIED" && $3 != "-" { print $3 }' "$w/sc" | { grep -vxF -f "$w/cur" 2>/dev/null || true; } >> "$w/new"
    for i in $(cat "$w/new"); do
      if grep -qxF "$i" "$w/history"; then fail "$rel: scenario ID $i is already taken (a living spec or an archived change holds it; IDs are never reused)"; fi
      echo "$i $id" >> "$w/added-all"
    done
    # Requirements by name, per section.
    awk '/^## (ADDED|MODIFIED|REMOVED|RENAMED) Requirements/ { sec = $2; next } /^## / { sec = "" }
         /^### Requirement:/ { n = $0; sub(/^### Requirement:[ \t]*/, "", n); sub(/[ \t]+$/, "", n); print sec "\t" n }' "$s" > "$w/reqs"
    while IFS="$(printf '\t')" read -r sec name; do
      has=""; [ -f "$living" ] && requirement_block "$living" "$name" | grep -q . && has=1
      case "$sec" in
        ADDED) [ -z "$has" ] || fail "$rel: ADDED requirement \"$name\" is already in $SPECS_DIR/$cap/spec.md (MODIFY it instead)" ;;
        REMOVED) [ -n "$has" ] || fail "$rel: REMOVED requirement \"$name\" is not in $SPECS_DIR/$cap/spec.md" ;;
        MODIFIED)
          if [ -z "$has" ]; then fail "$rel: MODIFIED requirement \"$name\" is not in $SPECS_DIR/$cap/spec.md"; continue; fi
          requirement_block "$living" "$name" | grep '^#### Scenario:' | sed 's/[[:space:]]*$//' > "$w/want"
          requirement_block "$s" "$name" | grep '^#### Scenario:' | sed 's/[[:space:]]*$//' > "$w/got"
          lost=$(grep -vxF -f "$w/got" "$w/want")
          if [ -n "$lost" ]; then fail "$rel: MODIFIED \"$name\" leaves out or rewords current scenario(s), which archiving would delete: $(printf '%s' "$lost" | tr '\n' ';'). Copy the current block word for word, then edit"
          else ok "$rel: MODIFIED \"$name\" keeps every current scenario header"; fi
          ;;
      esac
    done < "$w/reqs"
    if [ ! -f "$living" ]; then
      purpose=$(sed -n '/^## Purpose/,/^## /{/^## /d;p;}' "$s" | tr -s ' \n\t' '   ' | sed 's/^ *//; s/ *$//')
      if [ "${#purpose}" -ge 50 ]; then ok "$rel: a new capability, with a Purpose (${#purpose} characters)"
      else fail "$rel: a new capability's delta needs '## Purpose' of at least 50 characters (it has ${#purpose}); archiving otherwise writes a placeholder"; fi
    fi
  done
  if [ -z "$have_delta" ]; then
    if grep -qE '^skip_specs:[[:space:]]*true[[:space:]]*$' "$c/.openspec.yaml" 2>/dev/null; then ok "$id: no spec delta, and .openspec.yaml says skip_specs: true"
    else fail "$id: no spec delta. A change with none (a refactor, tooling) says so in $CHANGES_DIR/$id/.openspec.yaml: 'skip_specs: true'"; fi
  fi
done
dups=$(awk '{ c[$1]++; w[$1] = w[$1] " " $2 } END { for (i in c) if (c[i] > 1) print i ":" w[i] }' "$w/added-all")
[ -z "$dups" ] || fail "scenario IDs added by more than one open change: $dups"
[ "$n" -le 1 ] && ok "$n change(s) open (at most one proposed ahead of the one building)" || ok "NOTE $n changes open: more than one proposed ahead is a finding for session-start"
[ "$n" -eq 0 ] && ok "no open changes in $CHANGES_DIR"

# ── The living specs ─────────────────────────────────────────────────────────────────────────────
strict=1; [ "${SCENARIO_IDS:-required}" = new-only ] && strict=0
: > "$w/living-ids"
for s in "$ROOT/$SPECS_DIR"/*/spec.md; do
  [ -f "$s" ] || continue
  rel="spec ${s#"$ROOT"/}"
  out=$(spec_ok "$s") && ok "$rel: every requirement has a scenario" || fail "$rel: $out"
  grep -q '^## Purpose' "$s" || fail "$rel has no ## Purpose"
  ids_ok "$s" "$rel" "$strict" > "$w/ids"; cat "$w/ids"; _fails=$((_fails + $(grep -c '^FAIL' "$w/ids")))
  spec_scenarios "$s" | awk -F'\t' -v f="$rel" '$3 != "-" { print $3 "\t" f }' >> "$w/living-ids"
done
dups=$(awk -F'\t' '{ c[$1]++; w[$1] = w[$1] " " $2 } END { for (i in c) if (c[i] > 1) print i " in" w[i] }' "$w/living-ids")
if [ -z "$dups" ]; then ok "no scenario ID appears in two living specs"; else fail "scenario IDs in more than one living spec: $dups"; fi
finish
