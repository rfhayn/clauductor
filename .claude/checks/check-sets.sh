#!/bin/sh
# The gate runs each shared check once, and never loses one (OPS-89 (c)). This repo's gate runs two
# check sets over one model: its own (.claude/checks, ROOT = this repo) and the template's
# (template/.claude/checks, ROOT = template/). scripts/ci/steps.sh leaves CHECKS_ONCE out of this
# repo's set, by CHECKS_SKIP, while the framework and the project.conf values those checks can read
# match the template's. This repo's own check: the template ships one check set and no CHECKS_ONCE.
#
# Five parts:
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
#     are compared but for CONF_NOT_READ. This repo's own project.conf is the template's but for
#     CONF_NOT_READ and CONF_DOUBLE_RUN_OK.
#  4. run.sh's CHECKS_SKIP on a fixture: it leaves out what it names and says so, a named run
#     ignores it, and no check inherits it.
#  5. That no CHECKS_ONCE check reads a CONF_NOT_READ key is CI's to find, not a reading of check
#     source (OPS-171): test.yml's process-checks step is the command that runs this repo's whole
#     set over its own project.conf. The step is held to that form, each refused form shown
#     failing; a fixture shows such a read passing the local gate's form and failing the step's
#     command. Whether a merge waits for it is read from project.conf: while GATE_PR_CHECKS names
#     the step's job, the job must pass, so the step must have run and passed; while it is empty
#     (before OPS-98), nothing enforces it, a SKIPPED line under the PASS; naming other checks but
#     not that job fails.
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

# A compared key that differs turns run-once off on every gate, and the gate's line says so where
# nobody reads it (OPS-170), so it fails here unless CONF_DOUBLE_RUN_OK accepts the double run.
# double_run DIR TMPL: `ok KEY` for each differing key CONF_DOUBLE_RUN_OK names, `fail KEY` for the
# rest, or one `unreadable <why>` line when either project.conf cannot be read.
double_run() {
  _dr=$(conf_differs "$1" "$2")
  case $_dr in *unreadable*) echo "unreadable $_dr"; return ;; esac
  for _k in $_dr; do
    case " $CONF_DOUBLE_RUN_OK " in *" $_k "*) echo "ok $_k" ;; *) echo "fail $_k" ;; esac
  done
}
# double_run_verdict DIR TMPL: double_run's lines as this check's ok and fail.
double_run_verdict() {
  _dv=$(double_run "$1" "$2")
  case $_dv in unreadable*) fail "cannot compare $1's project.conf with $2's: ${_dv#unreadable }"; return ;; esac
  [ -n "$_dv" ] || ok "$1's project.conf is $2's but for CONF_NOT_READ: the gate runs CHECKS_ONCE once"
  for _k in $(printf '%s\n' "$_dv" | awk '$1 == "ok" { print $2 }'); do ok "both sets run every check: $_k (CONF_DOUBLE_RUN_OK)"; done
  _dvf=$(printf '%s\n' "$_dv" | sed -n 's/^fail //p' | tr '\n' ' ' | sed 's/ $//')
  [ -z "$_dvf" ] || fail "$1's project.conf differs from $2's in $_dvf, so every gate runs CHECKS_ONCE twice. In scripts/ci/steps.sh, add it to CONF_NOT_READ if no CHECKS_ONCE check reads it, or to CONF_DOUBLE_RUN_OK to accept the double run; or make it the template's"
}
dr=$(CONF_DOUBLE_RUN_OK="" double_run "$C/a" "$C/b")
[ "$dr" = "fail GATE_RUN" ] && ok "falsified: a compared key that differs (GATE_RUN) fails the check" || fail "a differing GATE_RUN read as '$dr', not a failure"
dr=$(CONF_DOUBLE_RUN_OK="GATE_RUN" double_run "$C/a" "$C/b")
[ "$dr" = "ok GATE_RUN" ] && ok "...and passes once CONF_DOUBLE_RUN_OK accepts the double run" || fail "GATE_RUN in CONF_DOUBLE_RUN_OK read as '$dr'"
dr=$(double_run "$C/a" "$C/none")
[ "$dr" = "unreadable (unreadable in $C/none)" ] && ok "...and a project.conf that cannot be read is its own verdict" || fail "an unreadable project.conf read as '$dr'"
# The verdict reaches this check's fail count: each case in a subshell with its own count.
verdict_fails() { (CONF_DOUBLE_RUN_OK=$1; _fails=0; double_run_verdict "$2" "$3" >"$S/verdict"; echo "$_fails"); }
[ "$(verdict_fails "" "$C/a" "$C/b")" = 1 ] && grep -q "^FAIL .*in GATE_RUN, so every gate" "$S/verdict" \
  && ok "falsified: the verdict counts a differing key as a failure, naming it and the remedies" || fail "the verdict on a differing GATE_RUN: $(cat "$S/verdict")"
[ "$(verdict_fails GATE_RUN "$C/a" "$C/b")" = 0 ] && grep -q "^ok   both sets run every check: GATE_RUN" "$S/verdict" \
  && ok "...passes one CONF_DOUBLE_RUN_OK names, saying both sets run every check" || fail "the verdict with GATE_RUN accepted: $(cat "$S/verdict")"
[ "$(verdict_fails "" "$C/a" "$C/none")" = 1 ] && grep -q "^FAIL cannot compare" "$S/verdict" && ! grep -q "CONF_DOUBLE_RUN_OK" "$S/verdict" \
  && ok "...and fails an unreadable project.conf with its own message, not the remedies" || fail "the verdict on an unreadable project.conf: $(cat "$S/verdict")"
double_run_verdict "$ROOT" "$ROOT/template"

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

# ── 5. What runs CHECKS_ONCE over this repo's project.conf: CI, once a merge waits for it ──────
# A check left to the template's set must not read a CONF_NOT_READ key. Reading a check's source
# cannot tell (a key read by a file it runs, in any of a shell's forms, OPS-171), and comparing
# runs on every gate costs the run the local gate leaves out. CI's process-checks step (test.yml)
# is the command that runs this repo's whole set, CHECKS_ONCE included, over this repo's
# project.conf; it counts only where the merge guard waits for its job (GATE_PR_CHECKS names it),
# since the job then passes only once the step has run and passed. With GATE_PR_CHECKS empty (as
# before OPS-98, #350, while go test failed intermittently, #343), it is Nothing. You.
CI="$ROOT/.github/workflows/test.yml"
# ci_read FILE: one line per way FILE's process-checks step stops being this repo's whole set with
# its failure counted, then `@job <check name>`, the job that holds it. Forms are refused, not
# parsed: one step's run: is exactly `sh .claude/checks/run.sh`, and it holds only id, name,
# timeout-minutes, working-directory: ., run and env (not if, continue-on-error or shell: a key
# not listed is refused until someone reads what it does and lists it); CHECKS_SKIP appears
# neither outside the jobs, nor in that job outside its steps, nor in the step; and the job has no
# continue-on-error; no `shell:` appears in the workflow or the job outside its steps (where
# defaults.run.shell, block or flow, would reach the step; a matrix key named shell is refused
# too); no earlier step of the job names GITHUB_ENV or GITHUB_PATH (or github.env, github.path).
# Other jobs and the job's later steps are read only for a second run of the set. Nor is what an
# earlier step EXECUTES read: a
# script, `go test` or a `uses:` action (setup-go sets GOROOT) can write either file without naming
# it, which no reading of test.yml can see; its actions are pinned to a commit.
ci_read() {
  [ -f "$1" ] || { echo "is missing"; return; }
  awk '
    function ind(s) { match(s, /^[[:space:]]*/); return RLENGTH }
    # key(s): the mapping key a line starts, bare or quoted; "" if none (as double-pass.sh reads).
    function key(s,   c, i, rest) {
      sub(/^[[:space:]]*(-[[:space:]]+)?/, "", s); c = substr(s, 1, 1)
      if (c == "\"" || c == Q) {
        s = substr(s, 2); i = index(s, c); if (!i) return ""
        rest = substr(s, i + 1); return rest ~ /^[[:space:]]*:/ ? substr(s, 1, i - 1) : ""
      }
      if (!match(s, /^[A-Za-z0-9_-]+[[:space:]]*:/)) return ""
      s = substr(s, 1, RLENGTH); sub(/[[:space:]]*:$/, "", s); return s
    }
    # val(s): the value of a key, without one pair of surrounding quotes.
    # A trailing comment goes with it.
    function val(s,   c, i) {
      sub(/^[^:]*:[[:space:]]*/, "", s); c = substr(s, 1, 1)
      if (c == "\"" || c == Q) { s = substr(s, 2); i = index(s, c); return i ? substr(s, 1, i - 1) : c s }
      sub(/[[:space:]]+#.*$/, "", s); sub(/[[:space:]]+$/, "", s); return s
    }
    BEGIN { Q = sprintf("%c", 39); RUN = "^[[:space:]]*(-[[:space:]]+)?run:[[:space:]]*sh [.]claude/checks/run[.]sh([[:space:]]+#.*)?[[:space:]]*$"
      SH = "(^|[^A-Za-z0-9_-])shell[\"" Q "]?[[:space:]]*:" }
    /^[[:space:]]*#/ || /^[[:space:]]*$/ { next }
    { n++; L[n] = $0; N[n] = NR; I[n] = ind($0); k = key($0); J[n] = ""; ST[n] = 0 }
    I[n] == 0 { injobs = (k == "jobs"); jind = -1; job = ""; next }
    injobs && jind < 0 { jind = I[n] }
    injobs && I[n] == jind { job = k; kind = -1; insteps = 0; J[n] = job; next }
    injobs && kind < 0 { kind = I[n] }
    # The steps of a job: under its steps: key, items at the indent of that key included.
    injobs && insteps && (I[n] > kind || $0 ~ /^[[:space:]]*-/) { ST[n] = 1 }
    injobs && !ST[n] { insteps = (I[n] == kind && k == "steps") }
    injobs && !ST[n] && I[n] == kind && k == "name" { jname[job] = val($0) }
    injobs && !ST[n] && I[n] == kind && k == "continue-on-error" { jcoe[job] = N[n] }
    injobs { J[n] = job }
    END {
      for (t = 1; t <= n && L[t] !~ RUN; t++) ;
      if (t > n) { print "has no step whose run: is exactly sh .claude/checks/run.sh (this repo'"'"'s whole set)"; exit }
      for (i = t + 1; i <= n; i++) if (L[i] ~ RUN) print "runs the set again (line " N[i] "), so which run counts is not read"
      tj = J[t]
      if (tj == "") { print "runs the set outside any job (line " N[t] ")"; exit }
      for (s = t; s > 1 && L[s] !~ /^[[:space:]]*-[[:space:]]/; s--) ;
      si = I[s]; match(L[s], /^[[:space:]]*-[[:space:]]+/); sk = RLENGTH; wd = 0
      for (i = s; i <= n && (i == s || I[i] > si); i++) {
        IN[i] = 1
        if (i != s && I[i] != sk) continue
        k = key(L[i])
        if (k == "working-directory") { if (val(L[i]) == ".") wd = 1; else print "runs the step in working-directory " val(L[i]) ", not this repo'"'"'s root (line " N[i] ")" }
        else if (k != "id" && k != "name" && k != "timeout-minutes" && k != "run" && k != "env") print "carries " k ": on the step (line " N[i] "), which can skip it, swallow its failure or change what it runs"
      }
      if (!wd) print "has no working-directory: . on the step, so a default decides which set it runs"
      for (i = 1; i <= n; i++) if ((J[i] == "" || (J[i] == tj && !ST[i]) || (i in IN)) && L[i] ~ /CHECKS_SKIP/) print "names CHECKS_SKIP where the step inherits it, in the workflow, job " tj " or the step (line " N[i] "): CI could leave checks out of this repo'"'"'s set too"
      # An earlier step of the job reaches this one through the files GitHub reads between steps.
      for (i = 1; i < t; i++) if (J[i] == tj && L[i] ~ /GITHUB_ENV|GITHUB_PATH|github[.](env|path)/) print "writes GITHUB_ENV or GITHUB_PATH in job " tj " before the step (line " N[i] "), which can set its CHECKS_SKIP or the sh it runs"
      # defaults.run.shell, of the workflow or the job, is the shell of every step without its own.
      for (i = 1; i <= n; i++) if ((J[i] == "" || (J[i] == tj && !ST[i])) && L[i] ~ SH) print "names a shell: key in the workflow or job " tj " outside its steps (line " N[i] "), refused wherever defaults.run.shell could reach the step and decide what it runs and what its status is"
      if (tj in jcoe) print "tolerates a failure: continue-on-error on job " tj " (line " jcoe[tj] ")"
      print "@job " ((tj in jname) ? jname[tj] : tj)
    }
  ' "$1"
}
ci_problems() { ci_read "$1" | grep -v '^@job '; }
ci_job() { ci_read "$1" | sed -n 's/^@job //p'; }
job=$(ci_job "$CI") out=$(ci_problems "$CI")
[ -z "$out" ] && ok "test.yml's job $job holds the step that runs this repo's whole set: one step, run: sh .claude/checks/run.sh from working-directory: ., of listed keys only; no CHECKS_SKIP in the workflow, the job's own keys or the step; no continue-on-error on the job; no shell: in the workflow or the job outside its steps; no GITHUB_ENV or GITHUB_PATH named before it" \
  || fail "test.yml's process-checks step is no longer this repo's whole set with its failure counted, so nothing runs CHECKS_ONCE over this repo's project.conf: $(printf '%s' "$out" | tr '\n' ';')"
# variant MODE ARG [TEXT]: a copy of test.yml in $S/ci.yml, changed one way (sed EXPR; append TEXT;
# after LINE TEXT, inserted after the first line that is exactly LINE); fails if nothing changed.
variant() {
  case $1 in
    sed) sed "$2" "$CI" > "$S/ci.yml" ;;
    append) { cat "$CI"; printf '%s\n' "$2"; } > "$S/ci.yml" ;;
    after) awk -v l="$2" -v t="$3" '{ print } !d && $0 == l { print t; d = 1 }' "$CI" > "$S/ci.yml" ;;
  esac
  ! cmp -s "$CI" "$S/ci.yml"
}
# doctor WHY MODE ARG [TEXT]: ci_problems must name the variant. passes WHY ...: it must not.
doctor() {
  _w=$1; shift
  if ! variant "$@"; then fail "falsified: $_w: the doctored copy is test.yml unchanged"
  elif [ -n "$(ci_problems "$S/ci.yml")" ]; then ok "falsified: $_w fails"
  else fail "falsified: $_w passes"; fi
}
passes() {
  _w=$1; shift
  if ! variant "$@"; then fail "$_w: the changed copy is test.yml unchanged"
  elif [ -z "$(ci_problems "$S/ci.yml")" ]; then ok "...and $_w passes"
  else fail "$_w fails: $(ci_problems "$S/ci.yml" | tr '\n' ';')"; fi
}
RUNL='        run: sh .claude/checks/run.sh' JOBL='  test:'
doctor "CHECKS_SKIP on the step's command" sed 's|run: sh \.claude/checks/run\.sh$|run: CHECKS_SKIP="gate" sh .claude/checks/run.sh|'
doctor "CHECKS_SKIP in the step's env" append '          CHECKS_SKIP: "gate"'
doctor "CHECKS_SKIP in the workflow's env" append "$(printf 'env:\n  CHECKS_SKIP: gate')"
doctor "a set run by name" sed 's|run: sh \.claude/checks/run\.sh$|run: sh .claude/checks/run.sh hooks|'
doctor "a set whose failure is swallowed" sed 's|run: sh \.claude/checks/run\.sh$|run: sh .claude/checks/run.sh \|\| true|'
doctor "the step gone" sed '/run: sh \.claude\/checks\/run\.sh$/d'
doctor "continue-on-error on the step" after "$RUNL" '        continue-on-error: true'
doctor "an if: on the step (PRs run Ubuntu only)" after "$RUNL" "        if: runner.os == 'macOS'"
doctor "the step run from template/" sed 's|^        working-directory: \.$|        working-directory: template|'
doctor "the step with no working-directory of its own" sed '/^        working-directory: \.$/d'
doctor "continue-on-error on the job" after "$JOBL" '    continue-on-error: true'
doctor "an earlier step writing CHECKS_SKIP to GITHUB_ENV" after '      - run: go vet ./...' '      - run: echo "CHECKS_SKIP=gate" >> "$GITHUB_ENV"'
doctor "...or to github.env" after '      - run: go vet ./...' '      - run: echo "CHECKS_SKIP=gate" >> "${{ github.env }}"'
doctor "the job's default shell" after '        working-directory: framework' '        shell: "true {0}"'
doctor "the workflow's default shell" append "$(printf 'defaults:\n  run:\n    shell: "true {0}"')"
passes "a comment naming CHECKS_SKIP" append '        # CHECKS_SKIP: a comment'
passes "a job-level if: (OPS-98's draft filter)" after "$JOBL" "    if: github.event_name != 'pull_request' || github.event.pull_request.draft == false"
passes "continue-on-error and CHECKS_SKIP in another job" append "$(printf '  other:\n    runs-on: ubuntu-latest\n    continue-on-error: true\n    env:\n      CHECKS_SKIP: gate\n    steps:\n      - run: echo other')"
passes "another step's own CHECKS_SKIP" after '    steps:' '      - run: CHECKS_SKIP=x sh template/.claude/checks/run.sh'
passes "a timeout on the step" after "$RUNL" '        timeout-minutes: 30'
passes "an id on the step" after "$RUNL" '        id: checks'
passes "a trailing comment on the step's run:" sed 's|^\(        run: sh \.claude/checks/run\.sh\)$|\1  # whole set|'
passes "a trailing comment on the step's working-directory:" sed 's|^\(        working-directory: \.\)$|\1  # the root|'
passes "GITHUB_ENV written by a later step" append '      - run: echo "X=1" >> "$GITHUB_ENV"'
passes "GITHUB_ENV written by a job before test" after 'jobs:' "$(printf '  pre:\\n    runs-on: ubuntu-latest\\n    steps:\\n      - run: echo "X=1" >> "$GITHUB_ENV"')"
doctor "the job's default shell as a flow mapping" sed 's|^    defaults:$|    defaults: { run: { shell: "true {0}", working-directory: framework } }|'
passes "a default shell and GITHUB_ENV in another job" append "$(printf '  other:\n    runs-on: ubuntu-latest\n    defaults:\n      run:\n        shell: bash\n    steps:\n      - run: echo "X=1" >> "$GITHUB_ENV"')"
variant after "$JOBL" '    name: test # the suite' && [ "$(ci_job "$S/ci.yml")" = test ] \
  && ok "...and a trailing comment on the job's name: is no part of it" || fail "name: test # the suite read as '$(ci_job "$S/ci.yml")'"
# The check the merge guard would wait for is the job's name, else its id: moved or renamed, the
# step's job is read as such, so GATE_PR_CHECKS naming `test` no longer counts it (merge_half).
variant after "$JOBL" "    name: 'suite'" && cp "$S/ci.yml" "$S/ci-renamed.yml" && [ "$(ci_job "$S/ci.yml")" = suite ] \
  && ok "falsified: a job renamed (name: 'suite') is read as suite" || fail "a job renamed suite read as '$(ci_job "$S/ci.yml")'"
{ sed "\\|^$RUNL\$|d" "$CI"; printf '  process-checks:\n    runs-on: ubuntu-latest\n    steps:\n      - name: process checks\n        working-directory: .\n%s\n' "$RUNL"; } > "$S/ci-moved.yml" \
  && ! cmp -s "$CI" "$S/ci-moved.yml" && [ "$(ci_job "$S/ci-moved.yml")" = process-checks ] && [ -z "$(ci_problems "$S/ci-moved.yml")" ] \
  && ok "falsified: the step moved to a job of its own is read as that job" || fail "the step moved to job process-checks read as '$(ci_job "$S/ci-moved.yml")'"
variant after 'jobs:' "$(printf '  early:\\n    runs-on: ubuntu-latest\\n    steps:\\n      - working-directory: .\\n%s' "$RUNL")" && [ -n "$(ci_problems "$S/ci.yml")" ] \
  && ok "falsified: the set also run by a job before test fails" || fail "a job before test running the set passes"
# Steps written at their key's own indent (`steps:` then `- name:` under it): the job is still test.
awk 'd && /^      / { sub(/^  /, "") } { print } $0 == "    steps:" { d = 1 }' "$CI" > "$S/ci.yml" \
  && ! cmp -s "$CI" "$S/ci.yml" && [ "$(ci_job "$S/ci.yml")" = test ] && [ -z "$(ci_problems "$S/ci.yml")" ] \
  && ok "...and steps at their key's own indent read as job test" || fail "indentless steps read as job '$(ci_job "$S/ci.yml")': $(ci_problems "$S/ci.yml" | tr '\n' ';')"

# A check that reads PROJECT_NAME through a file it runs (`cd "$ROOT" && sh <relative path>`)
# passes the local gate's form (part 1: CHECKS_SKIP naming it) and fails test.yml's run: command.
cmd=$(awk '/^[[:space:]]*#/ { next } /^[[:space:]]*(-[[:space:]]+)?run:[[:space:]]*sh [.]claude\/checks\/run[.]sh([[:space:]]+#.*)?[[:space:]]*$/ { sub(/^[^:]*:[[:space:]]*/, ""); sub(/[[:space:]]+#.*$/, ""); sub(/[[:space:]]+$/, ""); print; exit }' "$CI")
G="$S/ci"
mkdir -p "$G/.claude/checks" "$G/.claude/lib"
cp "$TMPL_DIR/run.sh" "$TMPL_DIR/lib.sh" "$G/.claude/checks/"
cp "$ROOT/template/.claude/lib/conf.sh" "$ROOT/template/.claude/lib/modules.sh" "$G/.claude/lib/"
printf 'PROJECT_NAME="Mine"\n' > "$G/.claude/project.conf"
printf '. ./.claude/lib/conf.sh\n[ "$PROJECT_NAME" = Template ]\n' > "$G/.claude/brand.sh"
printf '. "$(dirname "$0")/lib.sh"\ncd "$ROOT" && sh .claude/brand.sh && ok "the template'"'"'s name" || fail "not the template'"'"'s name"\nfinish\n' > "$G/.claude/checks/once.sh"
o=$(cd "$G" && env CHECKS_BARE= CHECKS_SET= CHECKS_DOUBLE_PASS= CHECKS_SKIP="once" sh .claude/checks/run.sh 2>&1); rc=$?
o2=$(cd "$G" && env CHECKS_BARE= CHECKS_SET= CHECKS_DOUBLE_PASS= CHECKS_SKIP= sh -c "${cmd:-false}" 2>&1); rc2=$?
[ "$rc" = 0 ] && [ "$rc2" != 0 ] && case $o2 in *"FAIL once"*) true ;; *) false ;; esac \
  && ok "a check reading a key through a file it runs passes the local gate's form and fails test.yml's command ($cmd)" \
  || fail "the run-form fixture: the local form exited $rc ($o), test.yml's command '$cmd' $rc2 ($o2)"

# merge_half CHECKS JOB: whether a merge waits for JOB (pr-merge-guard.sh (a2) waits for each check
# GATE_PR_CHECKS names): `waits`; `off` while CHECKS is empty (no CI check is merge evidence; OPS-98
# fills it); `fail` when CHECKS names others but not JOB, so the run that counts is not one a merge
# waits for.
merge_half() {
  [ -n "$1" ] || { echo off; return; }
  for _w in $1; do [ "$_w" != "$2" ] || { echo waits; return; }; done
  echo fail
}
case $(merge_half "${GATE_PR_CHECKS:-}" "$job") in
  waits) ok "...and the merge guard waits for it: GATE_PR_CHECKS ($GATE_PR_CHECKS) names job $job, which passes only once the step has run CHECKS_ONCE over this repo's project.conf and passed" ;;
  off) ok "SKIPPED the merge guard's half: GATE_PR_CHECKS is empty, so no merge waits for CI's run of CHECKS_ONCE over this repo's project.conf (and that run needs its job's earlier steps green): Nothing. You. until GATE_PR_CHECKS names job $job (OPS-98)" ;;
  *) fail "GATE_PR_CHECKS ($GATE_PR_CHECKS) does not name test.yml's job that runs this repo's whole set ($job): no merge waits for the run of CHECKS_ONCE over this repo's project.conf" ;;
esac
# Driven on the copies above: OPS-98's setting with the real file waits; with the step moved to a
# job of its own, or the job renamed, it fails.
[ "$(merge_half test "$(ci_job "$CI")")" = waits ] && ok "falsified: GATE_PR_CHECKS=test waits for the real test.yml's run" || fail "GATE_PR_CHECKS=test with the real test.yml: $(merge_half test "$(ci_job "$CI")")"
[ "$(merge_half test "$(ci_job "$S/ci-moved.yml")")" = fail ] && [ "$(merge_half test "$(ci_job "$S/ci-renamed.yml")")" = fail ] \
  && ok "...and fails it once the step moves to a job of its own or the job is renamed" || fail "GATE_PR_CHECKS=test with the step moved or the job renamed does not fail"
finish
