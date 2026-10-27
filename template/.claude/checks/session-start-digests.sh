#!/bin/sh
# session-start/digests.sh — the five long sections of the session-start context block, printed as
# digests so the block fits (ST-11, ported from Standing Tee's session-start-digests.test.ts).
#
#   - SIZE, against the REAL docs: the digests stay under DIGEST_BUDGET however the docs grow.
#   - CONTENT, against fixtures: each digest keeps what session-start's steps use — the journal's
#     "What's next" (when present), the Raw count, and every record not yet Accepted, by name.
#   - WIRING: context.sh must call digests.sh (not print the five sections whole again).
#
# Falsified: context.sh with no digests.sh call fails; index_digest with no NOT ACCEPTED line fails
# the fixture case; replacing digests output with a stub fails the size and heading cases.
. "$(dirname "$0")/lib.sh"

DIGEST_BUDGET=8000
SCRIPT="$ROOT/.claude/skills/session-start/digests.sh"
CTX="$ROOT/.claude/skills/session-start/context.sh"

[ -f "$SCRIPT" ] || { fail "missing $SCRIPT"; finish; }
[ -x "$SCRIPT" ] || chmod +x "$SCRIPT"

digests() {
  (cd "$ROOT" && env "$@" sh "$SCRIPT" 2>&1)
}

has() { case "$2" in *"$3"*) ok "$1" ;; *) fail "$1: no \"$3\" in: $(printf '%s' "$2" | head -5)" ;; esac; }
hasnt() { case "$2" in *"$3"*) fail "$1: \"$3\" in: $(printf '%s' "$2" | head -5)" ;; *) ok "$1" ;; esac; }

grep -q 'skills/session-start/digests.sh' "$CTX" \
  && ok "context.sh calls digests.sh" \
  || fail "context.sh does not call digests.sh (the five sections would print whole again)"

out=$(digests); rc=$?
[ "$rc" -eq 0 ] || fail "digests.sh on the real docs exits $rc: $(printf '%s' "$out" | tail -3)"
bytes=$(printf '%s' "$out" | wc -c | tr -d ' ')
[ "$bytes" -lt "$DIGEST_BUDGET" ] \
  && ok "real-docs digests stay under ${DIGEST_BUDGET} bytes (got $bytes)" \
  || fail "real-docs digests are ${bytes} bytes, over ${DIGEST_BUDGET}"
for h in "- Change queue" "- Latest journal" "- Recent insights" "- ADR index" "- Practice record index"; do
  has "real docs: heading present" "$out" "$h"
done
case "$out" in *unreadable*|*failed*) fail "real-docs digests report unreadable/failed: $(printf '%s' "$out" | grep -E 'unreadable|failed' | head -1)" ;; esac
ok "real-docs digests do not report unreadable or failed"

d=$(scratch)
JOURNAL=$d/journal.md
INSIGHTS=$d/insights.md
ADR_INDEX=$d/adr.md
PRAC_INDEX=$d/prac.md

cat > "$JOURNAL" <<'EOF'
# Journal
## Session 9 — today — **the newest**
**What happened.** The newest session did a thing.
**Surprises.** Not printed.
**What's next:**
1. **The next thing.**
## Session 8 — before — **older**
**What's next:**
1. An older plan, not printed.

EOF

cat > "$INSIGHTS" <<'EOF'
| Date | Area | Topic | Observation | How to verify | Status |
|---|---|---|---|---|---|
| 2026-10-14 | CI | ci/one | a long insight text | verify | Raw |
| 2026-10-13 | Process | p/two | text | verify | Promoted → ADR-0001 |
| 2026-10-12 | Docs | d/three | text | verify | Raw — owner #1 |

EOF

cat > "$ADR_INDEX" <<'EOF'
# Records

## Index

| ADR | Title | Status |
|---|---|---|
| [0001](0001-a.md) | First decision | Accepted |
| [0002](0002-b.md) | A decision under review | Proposed |
| [0003](0003-c.md) | An old one | Superseded by ADR-0004 |

EOF

cat > "$PRAC_INDEX" <<'EOF'
# Records

## Index

| PRAC | Title | Status |
|---|---|---|
| [0010](0010-p.md) | A practice | Accepted |

EOF

fo=$(JOURNAL=$JOURNAL INSIGHTS=$INSIGHTS ADR_INDEX=$ADR_INDEX PRAC_INDEX=$PRAC_INDEX digests)

has "fixture: newest session heading" "$fo" "## Session 9"
has "fixture: first paragraph" "$fo" "**What happened.** The newest session did a thing."
has "fixture: What's next item" "$fo" "1. **The next thing.**"
hasnt "fixture: no Surprises" "$fo" "Surprises"
hasnt "fixture: no older What's next" "$fo" "An older plan"
has "fixture: Raw count in whole log" "$fo" "    2 row(s) at Raw in the whole log"
has "fixture: recent insight with status" "$fo" "    2026-10-13 · Process · p/two · Promoted → ADR-0001"
hasnt "fixture: no long observation text" "$fo" "a long insight text"
has "fixture: NOT ACCEPTED ADR by name" "$fo" "    NOT ACCEPTED: 0002 A decision under review — Proposed"
hasnt "fixture: Accepted not listed as NOT ACCEPTED" "$fo" "NOT ACCEPTED: 0001"
hasnt "fixture: Superseded not listed as NOT ACCEPTED" "$fo" "NOT ACCEPTED: 0003"
has "fixture: ADR index counts Accepted" "$fo" "3 records:"
has "fixture: full index path" "$fo" "    full index: $ADR_INDEX"

bare=$d/bare.md
printf '## Session 1 — x\n**What happened.** Only this.\n' > "$bare"
fo2=$(JOURNAL=$bare INSIGHTS=$INSIGHTS ADR_INDEX=$ADR_INDEX PRAC_INDEX=$d/absent.md digests)
has "fixture: no What's next" "$fo2" "(this entry has no **What's next** paragraph)"
has "fixture: missing index" "$fo2" "($d/absent.md unreadable)"

finish
