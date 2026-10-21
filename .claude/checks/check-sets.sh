#!/bin/sh
# The gate runs each shared check once, and never loses one (OPS-89 (c)). This repo's gate runs two
# check sets over one model: its own (.claude/checks, ROOT = this repo) and the template's
# (template/.claude/checks, ROOT = template/). scripts/ci/steps.sh leaves CHECKS_ONCE out of this
# repo's set, by CHECKS_SKIP, while the framework matches the template. This repo's own check: the
# template ships one check set and no CHECKS_ONCE.
#
# Three parts:
#  1. The gate's real steps, captured by a stand-in `step`: exactly two whole-set runs, one per set;
#     against the AUTHORITY, the two directories, every check runs in at least one set; a check left
#     out of one set runs in the other from a byte-identical copy; only CHECKS_ONCE is ever left out;
#     and nothing is left out once the framework differs from the template.
#  2. That judgement on doctored steps, each shown failing: a shared check left out of both sets, a
#     project check left out, a copy that differs, a set run by name, a set gone.
#  3. run.sh's CHECKS_SKIP on a fixture: it leaves out what it names and says so, a named run
#     ignores it, and no check inherits it.
. "$(dirname "$0")/lib.sh"
need git

STEPS="$ROOT/scripts/ci/steps.sh"
REPO_DIR="$ROOT/.claude/checks" TMPL_DIR="$ROOT/template/.claude/checks"
[ -f "$STEPS" ] && [ -d "$TMPL_DIR" ] || { fail "no $STEPS or $TMPL_DIR: nothing to check the two sets against"; finish; }
. "$STEPS"
command -v framework_matches_template >/dev/null 2>&1 && [ -n "${CHECKS_ONCE:-}" ] \
  || { fail "steps.sh defines no CHECKS_ONCE or framework_matches_template: the two sets are not deduplicated"; finish; }

S=$(scratch)
# names DIR: the checks run.sh finds in DIR (its own rule: every *.sh but run and lib).
names() { (cd "$1" && ls ./*.sh 2>/dev/null) | sed 's|^\./||; s|\.sh$||' | grep -vx 'run\|lib'; }

# capture MATCH: run gate_steps full with a `step` that records, and runs nothing but check_sets. One
# line per step that runs a check set: `<dir>\t<CHECKS_SKIP>\t<arguments after run.sh>`.
# framework_matches_template answers MATCH (0 or 1), so the capture needs no Go toolchain.
capture() {
  _match=$1
  (
    framework_matches_template() { return "$_match"; }
    step() {
      shift; _skip="" _set="" _args=""
      # The step that decides what this repo's set leaves out runs for real (it sets CHECKS_LEFT).
      [ "$1" = check_sets ] && { "$@" >/dev/null 2>&1; return 0; }
      for _a in "$@"; do
        if [ -n "$_set" ]; then _args="$_args $_a"; continue; fi
        case $_a in
          CHECKS_SKIP=*) _skip=${_a#CHECKS_SKIP=} ;;
          */checks/run.sh) _set=${_a%/run.sh} ;;
        esac
      done
      [ -z "$_set" ] || printf '%s\t%s\t%s\n' "$_set" "$_skip" "$_args" >&3
      return 0
    }
    cd "$ROOT" && gate_steps full 3>&1 >/dev/null 2>&1
  )
}

# judge CAPTURE REPO_DIR TMPL_DIR: every rule part 1 states; prints one line per breach.
judge() {
  _cap=$1 _rd=$2 _td=$3
  _rl=$(awk -F'\t' '$1 == ".claude/checks"' "$_cap") _tl=$(awk -F'\t' '$1 == "template/.claude/checks"' "$_cap")
  [ "$(printf '%s\n' "$_rl" | grep -c .)" = 1 ] || echo "this repo's set (.claude/checks/run.sh) is run by $(printf '%s\n' "$_rl" | grep -c .) gate steps, not one"
  [ "$(printf '%s\n' "$_tl" | grep -c .)" = 1 ] || echo "the template's set (template/.claude/checks/run.sh) is run by $(printf '%s\n' "$_tl" | grep -c .) gate steps, not one"
  for _l in "$_rl" "$_tl"; do
    [ -z "$(printf '%s' "$_l" | cut -f3 | tr -d ' ')" ] || echo "$(printf '%s' "$_l" | cut -f1)/run.sh runs by name ($(printf '%s' "$_l" | cut -f3)): every check it does not name goes unrun"
  done
  _rs=" $(printf '%s' "$_rl" | cut -f2) " _ts=" $(printf '%s' "$_tl" | cut -f2) "
  for _n in $( { names "$_rd"; names "$_td"; } | sort -u); do
    _inr="" _int=""
    [ -f "$_rd/$_n.sh" ] && case $_rs in *" $_n "*) ;; *) _inr=1 ;; esac
    [ -f "$_td/$_n.sh" ] && case $_ts in *" $_n "*) ;; *) _int=1 ;; esac
    if [ -z "$_inr" ] && [ -z "$_int" ]; then echo "$_n runs in neither set"; continue; fi
    case " $CHECKS_ONCE " in *" $_n "*) _once=1 ;; *) _once="" ;; esac
    for _side in r t; do
      if [ "$_side" = r ]; then _d=$_rd _o=$_td _in=$_inr _ran=$_int _what="this repo's set"
      else _d=$_td _o=$_rd _in=$_int _ran=$_inr _what="the template's set"; fi
      [ -f "$_d/$_n.sh" ] && [ -z "$_in" ] || continue
      [ -n "$_once" ] || echo "$_n is left out of $_what, but it is not in CHECKS_ONCE: it reads that set's own state"
      [ -n "$_ran" ] && cmp -s "$_d/$_n.sh" "$_o/$_n.sh" || echo "$_n is left out of $_what, and the other set does not run a byte-identical copy"
    done
  done
  for _n in $CHECKS_ONCE; do
    [ -f "$_rd/$_n.sh" ] && [ -f "$_td/$_n.sh" ] || echo "CHECKS_ONCE names $_n, which is not a check in both sets"
  done
}

# ── 1. The gate's real steps ─────────────────────────────────────────────────────────────────
capture 0 > "$S/match"
capture 1 > "$S/differs"
out=$(judge "$S/match" "$REPO_DIR" "$TMPL_DIR")
[ -z "$out" ] && ok "while the framework matches the template: each set runs whole once, every check of both runs in at least one set, and only CHECKS_ONCE is left to the template's" \
  || fail "the gate's check sets: $(printf '%s' "$out" | tr '\n' ';')"
left=$(awk -F'\t' '$1 == ".claude/checks" { print $2 }' "$S/match")
[ "$(printf '%s\n' $left | sort | tr '\n' ' ')" = "$(printf '%s\n' $CHECKS_ONCE | sort | tr '\n' ' ')" ] \
  && ok "...and this repo's set leaves out exactly CHECKS_ONCE ($(printf '%s\n' $left | grep -c .) checks)" || fail "this repo's set leaves out '$left', not CHECKS_ONCE '$CHECKS_ONCE'"
out=$(judge "$S/differs" "$REPO_DIR" "$TMPL_DIR")
[ -z "$out" ] && [ -z "$(cut -f2 "$S/differs" | tr -d ' ')" ] \
  && ok "once the framework differs from the template, neither set leaves anything out" || fail "with the framework differing: skips '$(cut -f2 "$S/differs" | tr '\n' ' ')' $(printf '%s' "$out" | tr '\n' ';')"

# ── 2. The judgement fails each breach ───────────────────────────────────────────────────────
once1=$(printf '%s\n' $CHECKS_ONCE | head -n 1)
# A function, not inline in $(…): a case pattern's lone `)` there trips bash's parser.
first_project_check() {
  for _n in $(names "$TMPL_DIR"); do
    case " $CHECKS_ONCE " in *" $_n "*) ;; *) echo "$_n"; return 0 ;; esac
  done
}
proj1=$(first_project_check)
breach() {  # breach WHY CAPTURE [REPO_DIR]: judge must name the breach
  if [ -n "$(judge "$2" "${3:-$REPO_DIR}" "$TMPL_DIR")" ]; then ok "falsified: $1 fails the judgement"; else fail "falsified: $1 passes the judgement"; fi
}
awk -F'\t' -v n="$once1" 'BEGIN { OFS = "\t" } $1 ~ /^template/ { $2 = n } { print }' "$S/match" > "$S/both"
breach "a shared check ($once1) left out of both sets" "$S/both"
awk -F'\t' -v n="$proj1" 'BEGIN { OFS = "\t" } $1 == ".claude/checks" { $2 = $2 " " n } { print }' "$S/match" > "$S/proj"
breach "a project check ($proj1) left out of this repo's set" "$S/proj"
mkdir -p "$S/repo-checks"; cp "$REPO_DIR"/*.sh "$S/repo-checks/"; printf '\n# drift\n' >> "$S/repo-checks/$once1.sh"
breach "a check left out while this repo's copy ($once1) differs from the template's" "$S/match" "$S/repo-checks"
awk -F'\t' 'BEGIN { OFS = "\t" } $1 ~ /^template/ { $3 = " gate" } { print }' "$S/match" > "$S/named"
breach "the template's set run by name" "$S/named"
grep -v '^template' "$S/match" > "$S/gone"
breach "the template's set gone from the gate" "$S/gone"

# ── 3. run.sh's CHECKS_SKIP ──────────────────────────────────────────────────────────────────
F="$S/fixture"
mkdir -p "$F/.claude/checks" "$F/.claude/lib"
cp "$TMPL_DIR/run.sh" "$TMPL_DIR/lib.sh" "$F/.claude/checks/"
cp "$ROOT/template/.claude/lib/conf.sh" "$ROOT/template/.claude/lib/modules.sh" "$F/.claude/lib/"
for n in alpha beta gamma; do
  printf '. "$(dirname "$0")/lib.sh"\n[ -z "${CHECKS_SKIP+x}" ] && ok "%s does not inherit CHECKS_SKIP" || fail "%s inherited CHECKS_SKIP=$CHECKS_SKIP"\nfinish\n' "$n" "$n" > "$F/.claude/checks/$n.sh"
done
# A whole-set run of its own: none of the markers a run above this one exported (OPS-93).
o=$(env CHECKS_BARE= CHECKS_SET= CHECKS_DOUBLE_PASS= CHECKS_SKIP="beta" sh "$F/.claude/checks/run.sh" 2>&1); rc=$?
expect_rc 0 "$rc" "a whole-set run with CHECKS_SKIP passes"
case $o in *"PASS alpha"*"PASS gamma"*) ok "...runs every check it does not name, none of them inheriting it" ;; *) fail "CHECKS_SKIP run: $o" ;; esac
case $o in *"PASS beta"*) fail "CHECKS_SKIP=beta still ran beta: $o" ;; *"left to another run by CHECKS_SKIP: beta"*) ok "...leaves out the one it names, and says so" ;; *) fail "CHECKS_SKIP run does not name what it left out: $o" ;; esac
o=$(env CHECKS_SKIP="beta" sh "$F/.claude/checks/run.sh" beta 2>&1)
case $o in *"PASS beta"*) ok "a named run ignores CHECKS_SKIP" ;; *) fail "a named run of beta with CHECKS_SKIP=beta: $o" ;; esac
finish
