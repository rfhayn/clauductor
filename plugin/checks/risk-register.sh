#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The risk-register module's parts, whether it is on here or not, in a throwaway project:
#   - the session-start section lists every live row as `R<n> <title> — gate <Gate> (<Issue>)`, and
#     leaves out a row whose bold title holds ✅ (a ✅ elsewhere in the row keeps it live);
#   - it reads RISK_REGISTER, says `none: …` for a register with no live risk and CANNOT CHECK for
#     one it cannot find, never nothing;
#   - it prints only while MODULES names the module, as does the session-close review fragment;
#   - enable.sh makes the register from the stub and never overwrites one;
#   - the module's own check passes the stub and fails a duplicate id, a row with no bold title, an
#     empty Gate or Issue cell, and a row the listing would skip.
# With the module on, this project's own register is the module's check (risk-register:register).
. "$(dirname "$0")/lib.sh"
need git

d=$(scratch)
R="$d/p"; new_repo "$R"
mkdir -p "$R/.claude/lib" "$R/.claude/checks" "$R/docs"
cp "$CLAUDUCTOR_FW/lib/conf.sh" "$CLAUDUCTOR_FW/lib/modules.sh" "$R/.claude/lib/"
cp "$CLAUDUCTOR_FW/extensions.sh" "$R/.claude/"
cp "$CLAUDUCTOR_FW/checks/run.sh" "$CLAUDUCTOR_FW/checks/lib.sh" "$R/.claude/checks/"
cp -R "$CLAUDUCTOR_FW/modules" "$R/.claude/modules"
conf() { printf '%s\n' "$@" > "$R/.claude/project.conf"; }
ctx() { (cd "$R" && sh .claude/extensions.sh context session-start) 2>&1; }
has() {  # has WANT(yes|no) TEXT HAYSTACK LABEL
  case "$3" in *"$2"*) got=yes ;; *) got=no ;; esac
  [ "$got" = "$1" ] && ok "$4" || fail "$4 (looked for '$2'): $(printf '%s' "$3" | head -c 600)"
}

cat > "$R/docs/risk-register.md" <<'EOF'
# Risk register

| # | Risk | L | I | Mitigation | Gate | Issue |
|---|------|---|---|-----------|------|-------|
| R1 | **No delivery pipeline** — merged code reaches the box by hand | M | H | a deploy script | Phase 1 | #10 |
| R2 | **✅ MITIGATED — No audit trail** — disputes had no history | M | M | shipped | Phase 1 | #16 (closed) |
| R3 | **Legal exposure** — no policy | M | H→M | half done ✅, half open | Phase 1 → **2B.7c** | #14 #15 |
| R4 | **A pipe in a cell** — shifts the middle columns | L | M | `a | b` | Phase 2 | — |
EOF
cat > "$d/want" <<'EOF'
    R1 No delivery pipeline — gate Phase 1 (#10)
    R3 Legal exposure — gate Phase 1 → **2B.7c** (#14 #15)
    R4 A pipe in a cell — gate Phase 2 (—)
EOF

conf 'MODULES=""'
c=$(ctx); has no "live-risks" "$c" "off: session-start prints no risk section while MODULES does not name the module"
f=$(cd "$R" && sh .claude/extensions.sh fragments session-close 2>&1)
has no "Review the risk register" "$f" "off: session-close has no review step"

conf 'MODULES="risk-register"'
c=$(ctx)
has yes "- live-risks (module risk-register):" "$c" "on: session-start prints the live-risks section under the module's heading"
printf '%s\n' "$c" | sed -n '/live-risks (module risk-register)/,$p' | sed 1d > "$d/got"
if cmp -s "$d/want" "$d/got"; then ok "it lists exactly the live rows, title, gate and issue (a ✅ title is closed; a ✅ elsewhere is not)"
else fail "the listing differs from the expected one:"; diff "$d/want" "$d/got" | sed 's/^/     /'; fi
f=$(cd "$R" && sh .claude/extensions.sh fragments session-close 2>&1)
has yes "Review the risk register" "$f" "on: session-close includes the review step"
r=$(cd "$R" && sh .claude/checks/run.sh risk-register:register 2>&1); rc=$?
expect_rc 0 "$rc" "the module's check passes a register in its grammar"
# A row outside the grammar (no bold title) whose text holds a ✅ is still LIVE: only a ✅ inside a
# bold title closes a risk (the module's check fails such a row; the listing must not hide it).
cp "$R/docs/risk-register.md" "$d/keep"
echo "| R5 | Plain title, the first half done ✅, the second open | L | M | x | Phase 3 | #5 |" >> "$R/docs/risk-register.md"
c=$(ctx); cp "$d/keep" "$R/docs/risk-register.md"
has yes "R5 Plain title, the first half done ✅, the second open — gate Phase 3 (#5)" "$c" "a row with no bold title and a ✅ in its text is listed as live"

conf 'MODULES="risk-register"' 'RISK_REGISTER="docs/other.md"'
c=$(ctx); has yes "CANNOT CHECK — RISK_REGISTER docs/other.md does not exist" "$c" "a register it cannot find says CANNOT CHECK, naming RISK_REGISTER"
(cd "$R" && sh .claude/checks/run.sh risk-register:register >/dev/null 2>&1); expect_rc 1 $? "...and fails the module's check"
o=$(cd "$R" && sh .claude/modules/risk-register/enable.sh 2>&1)
has yes "made docs/other.md from the stub" "$o" "enable.sh makes the register from the stub"
c=$(ctx); has yes "none: docs/other.md has no" "$c" "the stub lists no risk, and says so (its example row is not read as one)"
(cd "$R" && sh .claude/checks/run.sh risk-register:register >/dev/null 2>&1); expect_rc 0 $? "the stub passes the module's check"
echo "| R9 | **Mine** — kept | L | L | x | g | i |" >> "$R/docs/other.md"
(cd "$R" && sh .claude/modules/risk-register/enable.sh >/dev/null 2>&1)
grep -q '^| R9 |' "$R/docs/other.md" && ok "enable.sh never overwrites an existing register" || fail "enable.sh overwrote docs/other.md"
sed -i.bak 's/^| R9 | \*\*Mine\*\* — kept |/| R9 | **✅ CLOSED — Mine** — kept |/' "$R/docs/other.md"
c=$(ctx); has yes "none: all 1 rows of docs/other.md are ✅ closed" "$c" "a register whose every row is closed says so"

# The module's check, falsified one defect at a time.
bad() {  # bad LABEL ROW
  cp "$R/docs/risk-register.md" "$d/keep"; printf '%s\n' "$2" >> "$R/docs/risk-register.md"
  out=$(cd "$R" && sh .claude/checks/run.sh risk-register:register 2>&1); rc=$?
  cp "$d/keep" "$R/docs/risk-register.md"
  [ "$rc" -ne 0 ] && ok "the module's check fails $1" || fail "the module's check passed $1: $out"
}
conf 'MODULES="risk-register"'
bad "a duplicate id" "| R1 | **Again** — twice | L | L | x | g | i |"
bad "a Risk cell with no bold title" "| R7 | Plain title | L | L | x | g | i |"
bad "an empty Gate cell" "| R7 | **T** — x | L | L | x |  | i |"
bad "an empty Issue cell" "| R7 | **T** — x | L | L | x | g |  |"
bad "a row spaced so the listing skips it" "|R7| **T** — x | L | L | x | g | i |"
finish
