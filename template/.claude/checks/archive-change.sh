#!/bin/sh
# archive-change.sh promotes a change's spec deltas without losing what the living specs hold.
# Each case runs the real script in a throwaway project built from the example change
# (.claude/examples), and asserts what the living spec BECOMES, not only the exit code:
#   - the example change: a MODIFIED replace and a new capability created with its Purpose, in the
#     byte layout OpenSpec 1.2 writes (so moving between the CLI and this script churns nothing);
#   - NOT-SYNCED.md holds a capability back; RENAMED keeps the body under the new heading;
#   - a MODIFIED delta with FEWER scenarios than the living requirement is merged, keeping the ones
#     it did not restate (OpenSpec's replace deletes them), unless design.md names each one dropped;
#     a shorter body in such a merge, a rename or a MODIFIED of a missing requirement, an ADDED
#     duplicate and a new capability with no Purpose are each a STOP that writes nothing;
#   - superseded wording: a hit in a delta is a STOP, a hit in design.md a CHECK line only;
#   - after --apply, a requirement whose scenario count fell is a FAIL (a broken merge is planted).
# Behaviour parity with Standing Tee's archive (its skill, run over its own archive history) is in
# the PR that added this; the cases below are its decisions, each falsified against a mutation.
. "$(dirname "$0")/lib.sh"
. "$ROOT/.claude/lib/change.sh"

EX="$ROOT/.claude/examples"
C=changes/add-greeting-name
mk() {  # mk NAME: a project holding the example change and the greeting living spec; in $F
  F="$(scratch)/$1"; mkdir -p "$F/.claude/lib" "$F/changes" "$F/specs"
  cp "$ROOT/.claude/lib/conf.sh" "$ROOT/.claude/lib/change.sh" "$F/.claude/lib/"
  cp "$ROOT/.claude/archive-change.sh" "$F/.claude/"
  cp -R "$EX/changes/add-greeting-name" "$F/changes/"; cp -R "$EX/specs/greeting" "$F/specs/"
}
arc() { (cd "$F" && sh .claude/archive-change.sh "$@") > "$F/out" 2>&1; }
has() { grep -qF -- "$1" "$2"; }
# delta CAP BODY: replace the example change's delta for CAP.
delta() { mkdir -p "$F/$C/specs/$1"; printf '%s' "$2" > "$F/$C/specs/$1/spec.md"; }
living() { mkdir -p "$F/specs/$1"; printf '%s' "$2" > "$F/specs/$1/spec.md"; }

# ── The example change ────────────────────────────────────────────────────────────────────────
mk example
arc add-greeting-name; rc=$?
expect_rc 0 "$rc" "the example change's plan is clean"
has 'REPLACE greeting: "Greet every visitor"' "$F/out" && has 'NEW     farewell' "$F/out" && ok "...a REPLACE and a NEW capability" || fail "plan: $(tr '\n' '|' < "$F/out")"
cmp -s "$EX/specs/greeting/spec.md" "$F/specs/greeting/spec.md" && ok "...and the plan alone writes nothing" || fail "the plan (no --apply) changed specs/greeting"
arc add-greeting-name --apply; rc=$?
expect_rc 0 "$rc" "--apply promotes the example change"
[ "$(scenario_counts "$F/specs/greeting/spec.md")" = "$(printf '2\tGreet every visitor')" ] && ok "...greeting now holds the MODIFIED requirement's 2 scenarios" || fail "greeting after apply: $(scenario_counts "$F/specs/greeting/spec.md")"
grep -q '^Says goodbye to a member who signs out' "$F/specs/farewell/spec.md" && ! grep -q TBD "$F/specs/farewell/spec.md" \
  && ok "...farewell is created with the delta's Purpose, not a placeholder" || fail "farewell spec: $(head -5 "$F/specs/farewell/spec.md" | tr '\n' '|')"
# OpenSpec 1.2's layout, byte for byte (its `openspec archive` on this example writes exactly this):
# no blank line before or after the "## Requirements" line, one between blocks, one at the end.
printf '# Greeting\n\n## Purpose\nGreets a visitor on the home page, so a first visit feels addressed rather than anonymous.\n## Requirements\n### Requirement: Greet every visitor\n' > "$F/want.head"
head -6 "$F/specs/greeting/spec.md" | cmp -s - "$F/want.head" && [ "$(tail -c 2 "$F/specs/greeting/spec.md" | od -An -c | tr -d ' ')" = '\n\n' ] \
  && awk 'NR > 1 && prev == "" && $0 == "" { bad = 1 } { prev = $0 } END { exit bad }' "$F/specs/greeting/spec.md" \
  && ok "...in the byte layout OpenSpec 1.2 writes" || fail "layout differs from OpenSpec's: $(head -8 "$F/specs/greeting/spec.md" | tr '\n' '|')"

# ── NOT-SYNCED.md holds a capability back ─────────────────────────────────────────────────────
mk held
printf '# Deliberately not synced\n\nThe farewell route does not exist yet; sync when it ships.\n' > "$F/$C/specs/farewell/NOT-SYNCED.md"
arc add-greeting-name --apply; rc=$?
expect_rc 0 "$rc" "a held-back capability does not stop the archive"
[ ! -f "$F/specs/farewell/spec.md" ] && has 'HELD    farewell: NOT-SYNCED.md holds it back' "$F/out" && has 'The farewell route does not exist yet' "$F/out" \
  && ok "...farewell is not promoted, and the plan says why, quoting the file" || fail "held: farewell promoted or not reported: $(tr '\n' '|' < "$F/out")"
[ "$(scenario_counts "$F/specs/greeting/spec.md" | cut -f1)" = 2 ] && ok "...while greeting is still promoted" || fail "held: greeting not promoted"

# ── RENAMED ─────────────────────────────────────────────────────────────────────────────────────
mk renamed; rm -rf "$F/$C/specs/farewell"
delta greeting '## RENAMED Requirements

- FROM: `### Requirement: Greet every visitor`
- TO: `### Requirement: Greet each visitor`
'
arc add-greeting-name --apply; rc=$?
expect_rc 0 "$rc" "a RENAMED requirement is promoted"
if grep -q '^### Requirement: Greet each visitor$' "$F/specs/greeting/spec.md" && ! grep -q 'Greet every visitor' "$F/specs/greeting/spec.md" \
   && grep -q 'An anonymous visitor is greeted' "$F/specs/greeting/spec.md"; then ok "...under the new heading, with its body and scenario"
else fail "renamed: $(grep '^###\|^####' "$F/specs/greeting/spec.md" | tr '\n' '|')"; fi
mk renamedmissing; rm -rf "$F/$C/specs/farewell"
delta greeting '## RENAMED Requirements

- FROM: `### Requirement: Greet nobody`
- TO: `### Requirement: Greet each visitor`
'
arc add-greeting-name --apply; rc=$?
expect_rc 1 "$rc" "a RENAMED FROM the living spec lacks is refused"
has 'STOP    greeting: "Greet nobody": RENAMED, but the living spec has no such requirement' "$F/out" && cmp -s "$EX/specs/greeting/spec.md" "$F/specs/greeting/spec.md" \
  && ok "...naming it, and writing nothing" || fail "renamed missing: $(tr '\n' '|' < "$F/out")"

# ── A MODIFIED delta with FEWER scenarios than the living requirement ───────────────────────────
# The living requirement has two scenarios without IDs (a project adopted with SCENARIO_IDS=new-only);
# the delta restates only the one it changes.
two='# Greeting

## Purpose
Greets a visitor on the home page, so a first visit feels addressed rather than anonymous.

## Requirements
### Requirement: Greet every visitor
The system SHALL show a greeting on the home page to every visitor.

#### Scenario: An anonymous visitor is greeted
- **WHEN** a visitor opens the home page
- **THEN** the page shows "Hello!"

#### Scenario: A returning visitor is greeted
- **WHEN** a visitor returns
- **THEN** the page shows "Welcome back!"
'
short='## MODIFIED Requirements

### Requirement: Greet every visitor
The system SHALL show a greeting on the home page to every visitor, in their language.

#### Scenario: An anonymous visitor is greeted
- **WHEN** a visitor opens the home page
- **THEN** the page shows "Hello!" in the browser language
'
mk partial; rm -rf "$F/$C/specs/farewell"; living greeting "$two"; delta greeting "$short"
arc add-greeting-name --apply; rc=$?
expect_rc 0 "$rc" "a partial MODIFIED is promoted"
if has 'MERGE   greeting: "Greet every visitor": a partial delta restates 1 of 2 scenario(s); kept from the living spec: A returning visitor is greeted' "$F/out" \
   && grep -q 'Welcome back' "$F/specs/greeting/spec.md" && grep -q 'in the browser language' "$F/specs/greeting/spec.md" && grep -q 'in their language' "$F/specs/greeting/spec.md"; then
  ok "...MERGED: the delta's body and changed scenario, and the living scenario it did not restate"
else fail "partial: $(tr '\n' '|' < "$F/out") / $(grep -c '^#### Scenario' "$F/specs/greeting/spec.md") scenario(s)"; fi
mk named; rm -rf "$F/$C/specs/farewell"; living greeting "$two"; delta greeting "$short"
printf '\n## D9 — the returning greeting goes\nWe drop the scenario "A returning visitor is greeted": returning is not tracked.\n' >> "$F/$C/design.md"
arc add-greeting-name --apply; rc=$?
expect_rc 0 "$rc" "a partial MODIFIED whose dropped scenario design.md names is promoted"
has "design.md names each living one dropped" "$F/out" && ! grep -q 'Welcome back' "$F/specs/greeting/spec.md" \
  && ok "...as a REPLACE: the removal the owner approved goes" || fail "named: $(tr '\n' '|' < "$F/out")"
mk prose; rm -rf "$F/$C/specs/farewell"; living greeting "$two"; delta greeting "$short"
printf '\nA returning visitor is greeted the way they are today.\n' >> "$F/$C/design.md"
arc add-greeting-name --apply; rc=$?
has 'MERGE   greeting' "$F/out" && grep -q 'Welcome back' "$F/specs/greeting/spec.md" \
  && ok "a dropped scenario's title in unquoted prose is not naming its removal: still a MERGE" || fail "prose naming: $(tr '\n' '|' < "$F/out")"
# A delta that changes one scenario and adds one (as many as the living requirement) but leaves the
# other out: a reword or a drop, so neither a silent replace nor a merge.
mk swap; rm -rf "$F/$C/specs/farewell"; living greeting "$two"
delta greeting "$short
#### Scenario: A member is greeted by name
- **WHEN** a member opens the home page
- **THEN** the page shows their name
"
arc add-greeting-name --apply; rc=$?
expect_rc 1 "$rc" "a MODIFIED with as many scenarios that leaves a living one out is refused"
has 'does not restate 1 living scenario(s), and design.md does not name them as removed: A returning visitor is greeted' "$F/out" && grep -q 'Welcome back' "$F/specs/greeting/spec.md" \
  && ok "...naming it, and writing nothing" || fail "swap: $(tr '\n' '|' < "$F/out")"
# SCENARIO_IDS=new-only: the living scenarios have no ID, the restated delta copy must carry one.
mk newonly; rm -rf "$F/$C/specs/farewell"; living greeting "$two"
delta greeting "$(printf '%s' "$short" | sed 's/^#### Scenario: An anonymous/#### Scenario: [GREETING-1-S1] An anonymous/')
"
arc add-greeting-name --apply; rc=$?
expect_rc 0 "$rc" "a partial MODIFIED whose restated scenario gained an ID is promoted"
[ "$(grep -c '^#### Scenario:' "$F/specs/greeting/spec.md")" = 2 ] && grep -q 'in the browser language' "$F/specs/greeting/spec.md" && ! grep -q '"Hello!"$' "$F/specs/greeting/spec.md" \
  && ok "...the restated scenario replaces its ID-less original (matched by title), not sits beside it" || fail "new-only: $(grep '^####' "$F/specs/greeting/spec.md" | tr '\n' '|')"
mk renamedamp; rm -rf "$F/$C/specs/farewell"
delta greeting '## RENAMED Requirements

- FROM: `### Requirement: Greet every visitor`
- TO: `### Requirement: Greet & welcome every visitor`
'
arc add-greeting-name --apply; rc=$?
expect_rc 0 "$rc" "a RENAMED TO name with an & is promoted"
grep -qx '### Requirement: Greet & welcome every visitor' "$F/specs/greeting/spec.md" && ok "...with the & as text, not the matched heading" || fail "renamed &: $(grep '^###' "$F/specs/greeting/spec.md")"
mk shortbody; rm -rf "$F/$C/specs/farewell"
living greeting "$(printf '%s' "$two" | sed 's/^The system SHALL show a greeting on the home page to every visitor\.$/The system SHALL show a greeting on the home page to every visitor.\
\
The greeting SHALL be visible without scrolling./')
"
delta greeting "$short"
arc add-greeting-name --apply; rc=$?
expect_rc 1 "$rc" "a partial MODIFIED whose body has fewer paragraphs is refused"
has 'whose body leaves out living paragraph(s): ' "$F/out" && has 'The greeting SHALL be visible without scrolling' "$F/out" && grep -q 'Welcome back' "$F/specs/greeting/spec.md" \
  && ok "...naming the paragraph, and writing nothing" || fail "short body: $(tr '\n' '|' < "$F/out")"

# ── The other refusals ──────────────────────────────────────────────────────────────────────────
mk dupadd; rm -rf "$F/$C/specs/farewell"
delta greeting "$(sed 's/^## MODIFIED Requirements/## ADDED Requirements/' "$EX/$C/specs/greeting/spec.md")
"
arc add-greeting-name --apply; rc=$?
expect_rc 1 "$rc" "an ADDED requirement the living spec already has is refused"
has 'ADDED, but the living spec already has it' "$F/out" && ok "...saying so" || fail "dup add: $(tr '\n' '|' < "$F/out")"
mk nopurpose
sed '/^## Purpose/,/^## ADDED/{/^## ADDED/!d;}' "$EX/$C/specs/farewell/spec.md" > "$F/$C/specs/farewell/spec.md"
arc add-greeting-name --apply; rc=$?
expect_rc 1 "$rc" "a new capability with no Purpose is refused"
has 'no ## Purpose to create it with' "$F/out" && [ ! -f "$F/specs/farewell/spec.md" ] && cmp -s "$EX/specs/greeting/spec.md" "$F/specs/greeting/spec.md" \
  && ok "...and nothing is written, the other capability included" || fail "no purpose: $(tr '\n' '|' < "$F/out")"
mk removed; rm -rf "$F/$C/specs/farewell"
delta greeting '## REMOVED Requirements

### Requirement: Greet every visitor
**Reason:** the home page is gone.
'
arc add-greeting-name --apply; rc=$?
expect_rc 0 "$rc" "a REMOVED requirement is promoted"
! grep -q 'Greet every visitor' "$F/specs/greeting/spec.md" && ok "...and is gone from the living spec" || fail "removed: still there"

# ── Superseded wording ──────────────────────────────────────────────────────────────────────────
mk supdelta
arc add-greeting-name --superseded 'Goodbye, Ana' --apply; rc=$?
expect_rc 1 "$rc" "superseded wording in a delta is refused"
has 'STOP    superseded wording "Goodbye, Ana" in a delta' "$F/out" && [ ! -f "$F/specs/farewell/spec.md" ] && ok "...naming the line, writing nothing" || fail "superseded in delta: $(tr '\n' '|' < "$F/out")"
mk suprename; rm -rf "$F/$C/specs/farewell"
delta greeting '## RENAMED Requirements

- FROM: `### Requirement: Greet every visitor`
- TO: `### Requirement: Greet each visitor`
'
arc add-greeting-name --superseded 'Greet every visitor' --apply; rc=$?
expect_rc 0 "$rc" "superseded wording only in a RENAMED FROM line does not stop the archive (it names the old heading to retire it)"
has 'CHECK   superseded wording "Greet every visitor" in specs/greeting/spec.md:3' "$F/out" && has "RENAMED section" "$F/out" && ok "...it is listed to confirm, naming the section" || fail "superseded in a FROM line: $(tr '\n' '|' < "$F/out")"
mk supdesign
printf '\nRejected: a farewell banner that slides in.\n' >> "$F/$C/design.md"
arc add-greeting-name --superseded 'farewell banner that slides' --apply; rc=$?
expect_rc 0 "$rc" "superseded wording only in design.md does not stop the archive"
has 'CHECK   superseded wording "farewell banner that slides" in design.md' "$F/out" && has 'design  ' "$F/out" \
  && ok "...it is listed to confirm, with the design lines that record a reversal" || fail "superseded in design: $(tr '\n' '|' < "$F/out")"
mk supnone; arc add-greeting-name; has 'no --superseded phrase given' "$F/out" && ok "with no --superseded phrase the plan says the check did not run" || fail "no phrase: silent"

# ── The count guard after --apply catches a merge that loses a scenario ──────────────────────────
mk broken; rm -rf "$F/$C/specs/farewell"; living greeting "$two"; delta greeting "$short"
sed 's/lblk\[nme\] = out$/lblk[nme] = d/' "$F/.claude/lib/change.sh" > "$F/x" && mv "$F/x" "$F/.claude/lib/change.sh"
grep -q 'lblk\[nme\] = d$' "$F/.claude/lib/change.sh" && ok "fixture: a merge that drops the scenarios it should keep is planted" || fail "fixture: the broken merge was not planted; the next case learns nothing"
arc add-greeting-name --apply; rc=$?
expect_rc 1 "$rc" "a promotion that lost a scenario fails"
has 'FAIL    Greet every visitor: 2 scenario(s) before, 1 after' "$F/out" && ok "...naming the requirement and both counts" || fail "count guard: $(tr '\n' '|' < "$F/out")"
grep -q 'Welcome back' "$F/specs/greeting/spec.md" && ok "...and, like a STOP, writes nothing" || fail "count guard: the living spec was written before the FAIL"

# ── A chain of renames carries the count to the final name ───────────────────────────────────────
mk chain; rm -rf "$F/$C/specs/farewell"
delta greeting '## RENAMED Requirements

- FROM: `### Requirement: Greet every visitor`
- TO: `### Requirement: Greet each visitor`
- FROM: `### Requirement: Greet each visitor`
- TO: `### Requirement: Welcome each visitor`
'
arc add-greeting-name --apply; rc=$?
expect_rc 0 "$rc" "a chain of renames in one delta is promoted, with no false count FAIL"
grep -qx '### Requirement: Welcome each visitor' "$F/specs/greeting/spec.md" && ok "...under the last name" || fail "chain: $(tr '\n' '|' < "$F/out")"

# A shift (B->C, then A->B) and a swap through a temporary name: each count follows its requirement.
ab='# Greeting

## Purpose
Greets a visitor on the home page, so a first visit feels addressed rather than anonymous.

## Requirements
### Requirement: Alpha
The system SHALL alpha.

#### Scenario: a one
- **THEN** a

#### Scenario: a two
- **THEN** a

### Requirement: Beta
The system SHALL beta.

#### Scenario: b one
- **THEN** b
'
mk shift; rm -rf "$F/$C/specs/farewell"; living greeting "$ab"
delta greeting '## RENAMED Requirements

- FROM: `### Requirement: Beta`
- TO: `### Requirement: Gamma`
- FROM: `### Requirement: Alpha`
- TO: `### Requirement: Beta`
'
arc add-greeting-name --apply; rc=$?
expect_rc 0 "$rc" "a rename shift (Beta to Gamma, then Alpha to Beta) is promoted, with no false count FAIL"
[ "$(scenario_counts "$F/specs/greeting/spec.md" | tr '\t\n' ':;')" = "2:Beta;1:Gamma;" ] && ok "...Alpha's scenarios under Beta, Beta's under Gamma" || fail "shift: $(scenario_counts "$F/specs/greeting/spec.md" | tr '\t\n' ':;') / $(tr '\n' '|' < "$F/out")"
mk swap2; rm -rf "$F/$C/specs/farewell"; living greeting "$ab"
delta greeting '## RENAMED Requirements

- FROM: `### Requirement: Alpha`
- TO: `### Requirement: Tmp`
- FROM: `### Requirement: Beta`
- TO: `### Requirement: Alpha`
- FROM: `### Requirement: Tmp`
- TO: `### Requirement: Beta`
'
arc add-greeting-name --apply; rc=$?
expect_rc 0 "$rc" "a rename swap through a temporary name is promoted"
[ "$(scenario_counts "$F/specs/greeting/spec.md" | tr '\t\n' ':;')" = "2:Beta;1:Alpha;" ] && ok "...each requirement's scenarios under its new name" || fail "swap: $(scenario_counts "$F/specs/greeting/spec.md" | tr '\t\n' ':;') / $(tr '\n' '|' < "$F/out")"

# design.md names a dropped scenario by its ID only as a whole ID: GREETING-1-S1 is not in GREETING-1-S10.
twoid='# Greeting

## Purpose
Greets a visitor on the home page, so a first visit feels addressed rather than anonymous.

## Requirements
### Requirement: Greet every visitor
The system SHALL show a greeting on the home page to every visitor.

#### Scenario: [GREETING-1-S1] An anonymous visitor is greeted
- **THEN** the page shows "Hello!"

#### Scenario: [GREETING-1-S2] A returning visitor is greeted
- **THEN** the page shows "Welcome back!"
'
idshort='## MODIFIED Requirements

### Requirement: Greet every visitor
The system SHALL show a greeting on the home page to every visitor.

#### Scenario: [GREETING-1-S2] A returning visitor is greeted
- **THEN** the page shows "Welcome back, friend!"
'
mk idsub; rm -rf "$F/$C/specs/farewell"; living greeting "$twoid"; delta greeting "$idshort"
printf '\nA follow-up adds GREETING-1-S10 for single sign-on.\n' >> "$F/$C/design.md"
arc add-greeting-name --apply; rc=$?
has 'MERGE   greeting' "$F/out" && grep -q 'GREETING-1-S1\]' "$F/specs/greeting/spec.md" \
  && ok "an ID that only prefixes another (GREETING-1-S10) does not name GREETING-1-S1's removal: a MERGE keeps it" || fail "id substring: $(tr '\n' '|' < "$F/out")"
mk idnamed; rm -rf "$F/$C/specs/farewell"; living greeting "$twoid"; delta greeting "$idshort"
printf '\nGREETING-1-S1 goes: the anonymous greeting is retired.\n' >> "$F/$C/design.md"
arc add-greeting-name --apply; rc=$?
has 'design.md names each living one dropped' "$F/out" && ! grep -q 'GREETING-1-S1\]' "$F/specs/greeting/spec.md" \
  && ok "...and the whole ID does name it: a REPLACE" || fail "id named: $(tr '\n' '|' < "$F/out")"

# Superseded wording in the REMOVED section is a CHECK; in a RENAMED TO line it reaches the specs.
mk supremoved; rm -rf "$F/$C/specs/farewell"
delta greeting '## REMOVED Requirements

### Requirement: Greet every visitor
**Reason:** the home page is gone.
'
arc add-greeting-name --superseded 'Greet every visitor' --apply; rc=$?
expect_rc 0 "$rc" "superseded wording only in the REMOVED section does not stop the archive"
has "REMOVED section" "$F/out" && ok "...it is listed, naming the section" || fail "superseded removed: $(tr '\n' '|' < "$F/out")"
mk supto; rm -rf "$F/$C/specs/farewell"
delta greeting '## RENAMED Requirements

- FROM: `### Requirement: Greet every visitor`
- TO: `### Requirement: Greet each visitor warmly`
'
arc add-greeting-name --superseded 'warmly' --apply; rc=$?
expect_rc 1 "$rc" "superseded wording in a RENAMED TO line (the new heading) is refused"

# ── Superseded wording in a held-back capability's delta reaches nothing ─────────────────────────
mk supheld
printf '# Held\n\nNot shipped yet.\n' > "$F/$C/specs/farewell/NOT-SYNCED.md"
arc add-greeting-name --superseded 'Goodbye, Ana' --apply; rc=$?
expect_rc 0 "$rc" "superseded wording only in a held-back capability's delta does not stop the archive"
has 'in held-back farewell' "$F/out" && ok "...it is listed, naming the hold-back" || fail "superseded held: $(tr '\n' '|' < "$F/out")"

# ── scenario_counts prints a requirement with no scenario as 0 (Standing Tee's awk omitted it) ──
printf '### Requirement: A\n#### Scenario: a\n### Requirement: B\nThe system SHALL b.\n' > "$(scratch)/counts.md"
[ "$(scenario_counts "$(scratch)/counts.md" | tr '\t\n' ':;')" = "1:A;0:B;" ] && ok "scenario_counts lists a requirement with no scenario as 0" || fail "scenario_counts: $(scenario_counts "$(scratch)/counts.md" | tr '\t\n' ':;')"
finish
