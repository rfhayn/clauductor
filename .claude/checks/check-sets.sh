#!/bin/sh
# The gate runs each shared check once, and never loses one (OPS-89 (c)). This repo's gate runs two
# check sets over one model: its own (.claude/checks, ROOT = this repo) and the template's
# (template/.claude/checks, ROOT = template/). scripts/ci/steps.sh leaves CHECKS_ONCE out of this
# repo's set, by CHECKS_SKIP, while the framework and the project.conf values those checks can read
# match the template's. This repo's own check: the template ships one check set and no CHECKS_ONCE.
#
# Four parts:
#  1. The gate's real steps, full and quick, captured by a stand-in `step`: exactly two whole-set
#     runs, one per set, each passing its own CHECKS_SKIP (never the caller's); against the
#     AUTHORITY, the two directories, every check runs in at least one set; a check left out of one
#     set runs in the other from a byte-identical copy; only CHECKS_ONCE is ever left out; and
#     nothing is left out once the framework differs from the template.
#  2. That judgement on doctored steps, each shown failing: a shared check left out of both sets, a
#     project check left out, a copy that differs, a set run by name, a set gone, a set that takes
#     the caller's CHECKS_SKIP.
#  3. What decides "matches", on fixtures: the diff report's verdict fails closed (no report, no
#     file list, a file that differs, an extra file outside .claude/checks/), project.conf values
#     are compared but for CONF_NOT_READ, and no CHECKS_ONCE check, nor a file it sources or runs
#     from ROOT in place, names a CONF_NOT_READ key or copies ROOT's project.conf.
#  4. run.sh's CHECKS_SKIP on a fixture: it leaves out what it names and says so, a named run
#     ignores it, and no check inherits it.
. "$(dirname "$0")/lib.sh"
need git jq

STEPS="$ROOT/scripts/ci/steps.sh"
REPO_DIR="$ROOT/.claude/checks" TMPL_DIR="$ROOT/template/.claude/checks"
[ -f "$STEPS" ] && [ -d "$TMPL_DIR" ] || { fail "no $STEPS or $TMPL_DIR: nothing to check the two sets against"; finish; }
. "$STEPS"
for fn in framework_matches_template framework_verdict conf_differs check_sets; do
  command -v "$fn" >/dev/null 2>&1 || { fail "steps.sh defines no $fn: the two sets are not deduplicated as this check expects"; finish; }
done
[ -n "${CHECKS_ONCE:-}" ] || { fail "steps.sh sets no CHECKS_ONCE"; finish; }

S=$(scratch)
# names DIR: the checks run.sh finds in DIR (its own rule: every *.sh but run and lib).
names() { (cd "$1" && ls ./*.sh 2>/dev/null) | sed 's|^\./||; s|\.sh$||' | grep -vx 'run\|lib'; }

# capture MODE MATCH: run gate_steps MODE with a `step` that records, and runs nothing but
# check_sets. One line per step that runs a check set: `<dir>\t<CHECKS_SKIP>\t<arguments after
# run.sh>`, the skip `<inherited>` when the step passes none of its own. framework_matches_template
# answers MATCH (0 or 1), so the capture needs no Go toolchain.
capture() {
  _mode=$1 _match=$2
  (
    framework_matches_template() { return "$_match"; }
    step() {
      shift; _skip="<inherited>" _set="" _args=""
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
    cd "$ROOT" && gate_steps "$_mode" 3>&1 >/dev/null 2>&1
  )
}

# judge CAPTURE REPO_DIR TMPL_DIR: every rule part 1 states; prints one line per breach.
judge() {
  _cap=$1 _rd=$2 _td=$3
  _rl=$(awk -F'\t' '$1 == ".claude/checks"' "$_cap") _tl=$(awk -F'\t' '$1 == "template/.claude/checks"' "$_cap")
  [ "$(printf '%s\n' "$_rl" | grep -c .)" = 1 ] || echo "this repo's set (.claude/checks/run.sh) is run by $(printf '%s\n' "$_rl" | grep -c .) gate steps, not one"
  [ "$(printf '%s\n' "$_tl" | grep -c .)" = 1 ] || echo "the template's set (template/.claude/checks/run.sh) is run by $(printf '%s\n' "$_tl" | grep -c .) gate steps, not one"
  for _l in "$_rl" "$_tl"; do
    [ -n "$_l" ] || continue
    [ -z "$(printf '%s' "$_l" | cut -f3 | tr -d ' ')" ] || echo "$(printf '%s' "$_l" | cut -f1)/run.sh runs by name ($(printf '%s' "$_l" | cut -f3)): every check it does not name goes unrun"
    [ "$(printf '%s' "$_l" | cut -f2)" != "<inherited>" ] || echo "$(printf '%s' "$_l" | cut -f1)/run.sh passes no CHECKS_SKIP of its own: a caller's would leave checks out of it"
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
for mode in full quick; do
  capture "$mode" 0 > "$S/match.$mode"
  capture "$mode" 1 > "$S/differs.$mode"
  out=$(judge "$S/match.$mode" "$REPO_DIR" "$TMPL_DIR")
  [ -z "$out" ] && ok "$mode gate, the framework matching: each set runs whole once with its own CHECKS_SKIP, every check of both runs in at least one set, only CHECKS_ONCE is left to the template's" \
    || fail "the $mode gate's check sets: $(printf '%s' "$out" | tr '\n' ';')"
  left=$(awk -F'\t' '$1 == ".claude/checks" { print $2 }' "$S/match.$mode")
  [ "$(printf '%s\n' $left | sort | tr '\n' ' ')" = "$(printf '%s\n' $CHECKS_ONCE | sort | tr '\n' ' ')" ] \
    && ok "...and this repo's set leaves out exactly CHECKS_ONCE ($(printf '%s\n' $left | grep -c .) checks)" || fail "$mode: this repo's set leaves out '$left', not CHECKS_ONCE '$CHECKS_ONCE'"
  out=$(judge "$S/differs.$mode" "$REPO_DIR" "$TMPL_DIR")
  [ -z "$out" ] && [ -z "$(cut -f2 "$S/differs.$mode" | tr -d ' ')" ] \
    && ok "...and once the framework differs from the template, neither set leaves anything out" || fail "$mode, the framework differing: skips '$(cut -f2 "$S/differs.$mode" | tr '\n' ' ')' $(printf '%s' "$out" | tr '\n' ';')"
done

# The decision is a `==> ` line, so gate.sh's filtered output names what the gate left out.
for m in 0 1; do
  o=$(framework_matches_template() { return "$m"; }; check_sets)
  case $o in "==> check sets: "*) ok "the check-sets decision is a step marker line (framework matching: $([ "$m" = 0 ] && echo yes || echo no))" ;;
    *) fail "the check-sets decision is not a '==> check sets: ' line: $o" ;; esac
done

# ── 2. The judgement fails each breach ───────────────────────────────────────────────────────
M="$S/match.full"
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
awk -F'\t' -v n="$once1" 'BEGIN { OFS = "\t" } $1 ~ /^template/ { $2 = n } { print }' "$M" > "$S/both"
breach "a shared check ($once1) left out of both sets" "$S/both"
awk -F'\t' -v n="$proj1" 'BEGIN { OFS = "\t" } $1 == ".claude/checks" { $2 = $2 " " n } { print }' "$M" > "$S/proj"
breach "a project check ($proj1) left out of this repo's set" "$S/proj"
mkdir -p "$S/repo-checks"; cp "$REPO_DIR"/*.sh "$S/repo-checks/"; printf '\n# drift\n' >> "$S/repo-checks/$once1.sh"
breach "a check left out while this repo's copy ($once1) differs from the template's" "$M" "$S/repo-checks"
awk -F'\t' 'BEGIN { OFS = "\t" } $1 ~ /^template/ { $3 = " gate" } { print }' "$M" > "$S/named"
breach "the template's set run by name" "$S/named"
grep -v '^template' "$M" > "$S/gone"
breach "the template's set gone from the gate" "$S/gone"
awk -F'\t' 'BEGIN { OFS = "\t" } $1 ~ /^template/ { $2 = "<inherited>" } { print }' "$M" > "$S/inherits"
breach "the template's set taking the caller's CHECKS_SKIP" "$S/inherits"

# ── 3. What decides "matches" ────────────────────────────────────────────────────────────────
verdict() {  # verdict WANT WHY JSON: framework_verdict on JSON passes (0) or fails closed (1)
  printf '%s' "$3" | framework_verdict >/dev/null 2>&1; _rc=$?
  [ "$_rc" -ne 0 ] && _rc=1
  [ "$_rc" -eq "$1" ] && ok "diff verdict: $2" || fail "diff verdict: $2 (got $_rc, want $1)"
}
id='{"path":".claude/lib/conf.sh","tier":"framework","status":"identical"}'
verdict 0 "every framework file identical matches" "{\"files\":[$id,{\"path\":\"docs/x.md\",\"tier\":\"doc\",\"status\":\"present\"}]}"
verdict 0 "an extra check of this repo's own (.claude/checks/) matches" "{\"files\":[$id,{\"path\":\"\",\"dest\":\".claude/checks/mine.sh\",\"tier\":\"framework\",\"status\":\"extra\"}]}"
verdict 1 "an extra skill or module of this repo's own does not" "{\"files\":[$id,{\"path\":\"\",\"dest\":\".claude/skills/mine/SKILL.md\",\"tier\":\"framework\",\"status\":\"extra\"}]}"
verdict 1 "a framework file that differs does not" "{\"files\":[$id,{\"path\":\".claude/hooks/x.sh\",\"tier\":\"framework\",\"status\":\"differs\"}]}"
verdict 1 "a framework file missing does not" "{\"files\":[{\"path\":\".claude/hooks/x.sh\",\"tier\":\"framework\",\"status\":\"missing\"}]}"
verdict 1 "no report at all (the diff failed) does not" ""
verdict 1 "a report without its file list does not" '{"summary":{}}'

C="$S/conf"
mkdir -p "$C/a/.claude/lib" "$C/b/.claude/lib"
cp "$ROOT/template/.claude/lib/conf.sh" "$C/a/.claude/lib/"; cp "$ROOT/template/.claude/lib/conf.sh" "$C/b/.claude/lib/"
printf 'PROJECT_NAME="A"\nOWNER_NAME="a"\n' > "$C/a/.claude/project.conf"
printf 'PROJECT_NAME="B"\nOWNER_NAME="b"\n' > "$C/b/.claude/project.conf"
[ -z "$(conf_differs "$C/a" "$C/b")" ] && ok "project.conf: values in CONF_NOT_READ may differ" || fail "project.conf: CONF_NOT_READ keys counted as a difference: $(conf_differs "$C/a" "$C/b")"
# This repo's own keys (OPS-170): while they counted, no gate ran a check once.
printf 'APPROVAL_DELEGATED="1"\nGATE_DOCS="1"\n' >> "$C/a/.claude/project.conf"
[ -z "$(conf_differs "$C/a" "$C/b")" ] && ok "...and so may this repo's own APPROVAL_DELEGATED and GATE_DOCS" || fail "project.conf: this repo's own keys counted as a difference: $(conf_differs "$C/a" "$C/b")"
printf 'GATE_RUN="infra/gate.sh"\n' >> "$C/b/.claude/project.conf"
[ "$(conf_differs "$C/a" "$C/b")" = GATE_RUN ] && ok "...and any other key that differs is named (GATE_RUN)" || fail "project.conf: a GATE_RUN difference read as '$(conf_differs "$C/a" "$C/b")'"
case $(conf_differs "$C/a" "$C/none") in *unreadable*) ok "...and a project.conf that cannot be read is a difference" ;; *) fail "an unreadable project.conf compared as equal" ;; esac
# No CHECKS_ONCE check reads a key the comparison ignores. A key reaches a check from ROOT's
# project.conf only through the files that run with it (OPS-170): the check, lib.sh and conf.sh
# (every check sources them), each file it sources or runs from $ROOT in place (review-lane runs
# review-lane.sh there), and ROOT's project.conf itself, named. A model file a check copies into a
# fixture reads the fixture's project.conf over conf.sh's defaults, never ROOT's.
in_place() {  # in_place CHECK [CHECKS_DIR]: the model files CHECK runs with ROOT's project.conf
  _ipd=${2:-$TMPL_DIR} _ipf="${2:-$TMPL_DIR}/$1.sh"
  _ipr=$(cd "$_ipd/../.." && pwd)
  echo "$_ipd/lib.sh"; echo "$_ipr/.claude/lib/conf.sh"
  # Sourced or run by path: `. "$ROOT/x"`, `sh "$ROOT/x"`.
  grep -oE '(^|[^A-Za-z0-9_./-])(\.|sh|bash)[[:space:]]+"?\$\{?ROOT\}?/[^"[:space:])]+' "$_ipf" 2>/dev/null \
    | sed -E 's|.*\$\{?ROOT\}?/|'"$_ipr"'/|'
  # Through a variable that holds a $ROOT path: `L="$ROOT/x"` then `sh "$L"`, or `. "$M/lib/y.sh"`.
  sed -nE 's/^[[:space:]]*([A-Za-z_][A-Za-z0-9_]*)="\$\{?ROOT\}?\/([^"$]+)".*/\1 \2/p' "$_ipf" 2>/dev/null \
    | while read -r _ipv _ipp; do
        grep -oE "(^|[^A-Za-z0-9_./-])(\\.|sh|bash)[[:space:]]+\"?\\\$\\{?$_ipv\\}?([^A-Za-z0-9_\"[:space:])][^\"[:space:])]*)?" "$_ipf" \
          | sed -E "s|.*\\\$\\{?$_ipv\\}?|$_ipr/$_ipp|"
      done
}
key_reads() {  # key_reads CHECKS KEYS [CHECKS_DIR]: each `check:key` where a file in_place names $KEY or ${KEY
  for _n in $1; do
    grep -qE '\$\{?ROOT\}?/\.claude/project\.conf' "${3:-$TMPL_DIR}/$_n.sh" 2>/dev/null && printf ' %s:project.conf' "$_n"
    _files=$(in_place "$_n" "${3:-}" | sort -u)
    for _k in $2; do
      for _f in "${3:-$TMPL_DIR}/$_n.sh" $_files; do
        grep -qE "\\\$\\{?$_k([^A-Za-z0-9_]|\$)" "$_f" 2>/dev/null && { printf ' %s:%s' "$_n" "$_k"; break; }
      done
    done
  done
}
[ "$(key_reads modules MODULES)" = " modules:MODULES" ] && ok "falsified: the key-read scan finds a key a check reads (modules:MODULES)" \
  || fail "the key-read scan misses modules reading MODULES: '$(key_reads modules MODULES)'"
case " $(in_place review-lane | tr '\n' ' ') " in *"/.claude/skills/merge-pr/review-lane.sh "*) ok "...and reaches the script a check runs in place (review-lane.sh)" ;;
  *) fail "the key-read scan does not reach review-lane.sh, which review-lane runs with ROOT's project.conf: $(in_place review-lane | tr '\n' ' ')" ;; esac
K="$S/keys"; mkdir -p "$K/.claude/checks"
mkdir -p "$K/.claude/mod/lib"
printf '. "$ROOT/.claude/sourced.sh"\nL="$ROOT/.claude/run.sh"\nprintf x | sh "$L"\nM="$ROOT/.claude/mod"\n. "$M/lib/y.sh"\ncp "$ROOT/.claude/copied.sh" "$F/"\n' > "$K/.claude/checks/x.sh"
printf 'echo "$KEY_SOURCED"\n' > "$K/.claude/sourced.sh"; printf 'echo "${KEY_RUN}"\n' > "$K/.claude/run.sh"
printf 'echo "$KEY_DIR"\n' > "$K/.claude/mod/lib/y.sh"; printf 'echo "$KEY_COPIED"\n' > "$K/.claude/copied.sh"
printf 'cp "$ROOT/.claude/project.conf" "$F/.claude/"\n' > "$K/.claude/checks/y.sh"
o=$(key_reads "x y" "KEY_SOURCED KEY_RUN KEY_DIR KEY_COPIED" "$K/.claude/checks")
[ "$o" = " x:KEY_SOURCED x:KEY_RUN x:KEY_DIR y:project.conf" ] && ok "falsified: the scan finds a key read by a file a check sources or runs in place, or ROOT's project.conf copied, and not one a fixture's copy reads" \
  || fail "the key-read scan on a fixture: '$o', want ' x:KEY_SOURCED x:KEY_RUN x:KEY_DIR y:project.conf'"
reads=$(key_reads "$CHECKS_ONCE" "$CONF_NOT_READ")
[ -z "$reads" ] && ok "no CHECKS_ONCE check reads a CONF_NOT_READ key ($CONF_NOT_READ)" \
  || fail "CHECKS_ONCE checks read keys the comparison ignores:$reads"

# ── 4. run.sh's CHECKS_SKIP ──────────────────────────────────────────────────────────────────
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
