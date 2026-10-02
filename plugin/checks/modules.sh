#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The module interface and the local layer (lib/modules.sh, .claude/modules/README.md,
# .claude/local/README.md), in a throwaway project with a fixture module and a local layer:
#   - an ENABLED module's parts load at every point (health, context, skill fragments, conflict
#     rows, checks) and a DISABLED one's never do, while the local layer's always do;
#   - a module that requires one not on, or a name with no module, fails `extensions.sh list`;
#   - module.conf defaults apply only where project.conf sets nothing; the legacy switches
#     (PROPOSALS, REVIEW_PAGE) turn their modules on;
#   - a section that fails or prints nothing says CANNOT CHECK, never nothing;
#   - a failing module check fails checks/run.sh (so the run is not a silently shorter list).
# Then this project's own modules: each shipped or vendored module's module.conf names its
# directory, declares exactly the parts it has, and each script part parses under its #! line;
# every module MODULES enables loads. The guard.d point is held by checks/merge-guard.sh.
. "$(dirname "$0")/lib.sh"
need git

d=$(scratch)
R="$d/p"; new_repo "$R"
mkdir -p "$R/.claude/lib" "$R/.claude/checks" "$R/.claude/health"
cp "$CLAUDUCTOR_FW/lib/conf.sh" "$CLAUDUCTOR_FW/lib/modules.sh" "$R/.claude/lib/"
cp "$CLAUDUCTOR_FW/extensions.sh" "$R/.claude/"
cp "$CLAUDUCTOR_FW/checks/run.sh" "$CLAUDUCTOR_FW/checks/lib.sh" "$R/.claude/checks/"
cp -R "$CLAUDUCTOR_FW/modules" "$R/.claude/modules"
printf 'echo "OK core health"\n' > "$R/.claude/health/core.sh"

# part LAYER-DIR NAME: every point, each saying which layer it came from.
part() {
  b="$1"; n=$2
  mkdir -p "$b/guard.d" "$b/context.d/session-start" "$b/context.d/session-close" "$b/health" "$b/checks" "$b/skills/merge-pr"
  printf 'echo "allow from %s"\n' "$n" > "$b/guard.d/rule.sh"
  printf 'echo "start section from %s"\n' "$n" > "$b/context.d/session-start/sec.sh"
  printf 'echo "close section from %s"\n' "$n" > "$b/context.d/session-close/sec.sh"
  printf 'echo "OK health from %s"\n' "$n" > "$b/health/h.sh"
  printf '. "${CHECKS_LIB:?}"\n[ -f "$ROOT/FAIL_%s" ] && fail "check from %s" || ok "check from %s"\nfinish\n' "$n" "$n" "$n" > "$b/checks/c.sh"
  printf 'Fragment from %s.\n' "$n" > "$b/skills/merge-pr/f.md"
  printf 'docs/%s.json\tre-run the %s generator\n' "$n" "$n" > "$b/conflicts.tsv"
}
part "$R/.claude/modules/demo" demo
printf 'name="demo"\nrequires=""\nenables="guard.d context.d health checks skills conflicts.tsv"\nDEMO_KEY="from-module"\n' > "$R/.claude/modules/demo/module.conf"
part "$R/.claude/local" locallayer
mkdir -p "$R/.claude/modules/needy"
printf 'name="needy"\nrequires="demo"\nenables=""\n' > "$R/.claude/modules/needy/module.conf"

conf() { printf '%s\n' "$@" > "$R/.claude/project.conf"; }
ext() { (cd "$R" && sh .claude/extensions.sh "$@") 2>&1; }
# inr SCRIPT: sh code run in the project with its config read. CLAUDUCTOR_FW names its .claude/ too:
# the plugin build's copy of conf.sh finds its sibling libraries through it.
inr() { (cd "$R" && ROOT="$R" CLAUDUCTOR_FW="$R/.claude" sh -c ". .claude/lib/conf.sh; $1") 2>&1; }
has() {  # has WANT(yes|no) TEXT HAYSTACK LABEL
  case "$3" in *"$2"*) got=yes ;; *) got=no ;; esac
  [ "$got" = "$1" ] && ok "$4" || fail "$4 (looked for '$2'): $(printf '%s' "$3" | head -c 400)"
}

conf 'MODULES=""'
h=$(ext health); c=$(ext context session-start); cl=$(ext context session-close)
f=$(ext fragments merge-pr); t=$(ext conflicts); l=$(ext list)
has yes "OK core health" "$h" "health: .claude/health/ still runs"
has yes "OK health from locallayer" "$h" "health: the local layer's line runs"
has no "health from demo" "$h" "health: a DISABLED module's line does not run"
has yes "start section from locallayer" "$c" "context: the local layer's session-start section prints"
has no "from demo" "$c$cl" "context: a disabled module's sections do not print"
has yes "close section from locallayer" "$cl" "context: the local layer's session-close section prints"
has yes "Fragment from locallayer." "$f" "fragments: the local layer's merge-pr fragment is included"
has no "Fragment from demo." "$f" "fragments: a disabled module's fragment is not"
has yes "docs/locallayer.json" "$t" "conflicts: the local layer's row joins the table"
has no "docs/demo.json" "$t" "conflicts: a disabled module's row does not"
has no "demo" "$l" "list: a disabled module is not listed"
r=$(cd "$R" && sh .claude/checks/run.sh 2>&1)
has yes "PASS local:c" "$r" "checks/run.sh runs the local layer's check"
has no "demo:c" "$r" "checks/run.sh does not run a disabled module's check"

conf 'MODULES="demo"'
h=$(ext health); c=$(ext context session-start); f=$(ext fragments merge-pr); t=$(ext conflicts); l=$(ext list)
has yes "OK health from demo" "$h" "health: an ENABLED module's line runs"
has yes "Health: h (module demo)" "$h" "health: ...under a heading naming its module"
has yes "start section from demo" "$c" "context: an enabled module's section prints"
has yes "Fragment from demo." "$f" "fragments: an enabled module's fragment is included"
# A module's whole skill (skills/<s>/SKILL.md) is installed by its enable.sh, never appended as a
# fragment, not even to a host skill of the same name.
printf -- '---\nname: merge-pr\n---\nWhole skill from demo.\n' > "$R/.claude/modules/demo/skills/merge-pr/SKILL.md"
f=$(ext fragments merge-pr); rm -f "$R/.claude/modules/demo/skills/merge-pr/SKILL.md"
has no "Whole skill from demo." "$f" "fragments: a module's SKILL.md is not a fragment"
has yes "Fragment from demo." "$f" "...while its fragments beside it still are"
has yes "| docs/demo.json | re-run the demo generator (module demo) |" "$t" "conflicts: an enabled module's row joins the table"
has yes "module demo guard.d/rule.sh" "$l" "list: names the enabled module's guard rule"
# Module first, then local: the order the README promises.
first=$(printf '%s\n' "$h" | grep -n 'health from' | head -1)
case "$first" in *demo*) ok "a module's parts run before the local layer's" ;; *) fail "order: $first" ;; esac
r=$(cd "$R" && sh .claude/checks/run.sh 2>&1)
has yes "PASS demo:c" "$r" "checks/run.sh runs an enabled module's check"
touch "$R/FAIL_demo"
(cd "$R" && sh .claude/checks/run.sh >/dev/null 2>&1); expect_rc 1 $? "a failing module check fails checks/run.sh"
rm -f "$R/FAIL_demo"
v=$(inr 'echo "$DEMO_KEY"')
[ "$v" = from-module ] && ok "module.conf's default key applies when project.conf sets nothing" || fail "DEMO_KEY='$v'"
conf 'MODULES="demo"' 'DEMO_KEY="mine"'
v=$(inr 'echo "$DEMO_KEY"')
[ "$v" = mine ] && ok "project.conf's value wins over the module's default" || fail "DEMO_KEY='$v' (project.conf said mine)"

conf 'MODULES="needy"'
l=$(ext list); (cd "$R" && sh .claude/extensions.sh list >/dev/null 2>&1); expect_rc 1 $? "list fails when a module requires one that is not on"
has yes "requires module demo" "$l" "...and says which"
conf 'MODULES="needy demo"'
(cd "$R" && sh .claude/extensions.sh list >/dev/null 2>&1); expect_rc 0 $? "list passes once the required module is on too"
conf 'MODULES="nosuch"'
l=$(ext list); has yes "nosuch/module.conf" "$l" "an unknown module in MODULES is a failure, not ignored"

conf 'PROPOSALS="openspec"'
v=$(inr '. ./.claude/lib/modules.sh; modules_enabled | tr "\n" " "')
has yes "openspec" "$v" "the legacy PROPOSALS=openspec turns the openspec module on"
conf 'MODULES="openspec review-page"'
v=$(inr 'echo "$PROPOSALS $REVIEW_PAGE"')
[ "$v" = "openspec artifact" ] && ok "turning the modules on sets PROPOSALS and REVIEW_PAGE" || fail "PROPOSALS REVIEW_PAGE = '$v'"

# roadmap.d: a rule over the parsed queue holds EVERY read (any mode, the front door too) while its
# layer is on, and never while its module is off. The rule refuses any row owned by "Mallory".
cp "$CLAUDUCTOR_FW/roadmap-queue.sh" "$R/.claude/"
mkdir -p "$R/docs" "$R/.claude/modules/demo/roadmap.d"
printf '## Phase 1 — one\n**Owner:** Mallory\n\n| # | Change | Scope | Deps | Status |\n|---|---|---|---|---|\n| 1.1 | `add-x` — x | x | — | ⬜ queued |\n' > "$R/docs/roadmap.md"
printf '#!/bin/sh\nawk -F"\\t" '"'"'$9 == "Mallory" { print "row " $4 ": Mallory may own nothing"; bad = 1 } END { exit bad }'"'"'\n' > "$R/.claude/modules/demo/roadmap.d/no-mallory.sh"
sed -i.bak 's/conflicts.tsv"/conflicts.tsv roadmap.d"/' "$R/.claude/modules/demo/module.conf" && rm -f "$R/.claude/modules/demo/module.conf.bak"
conf 'MODULES=""'
v=$(inr 'roadmap_queue --check'); has yes "roadmap ok" "$v" "roadmap.d: a disabled module's rule does not run"
conf 'MODULES="demo"'
for m in --check --text --tsv --queued; do
  v=$(inr "roadmap_queue $m; echo rc=\$?")
  has yes "rc=1" "$v" "roadmap.d: an enabled module's rule refuses the queue on roadmap_queue $m"
done
has yes "module demo roadmap.d/no-mallory.sh: row 1.1: Mallory may own nothing" "$v" "roadmap.d: the refusal names its rule and the row"
# The parser's front door, called by path ON PURPOSE (checks/roadmap.sh's bypass scan matches the
# literal path, which a reader must not use; this case proves the door holds a direct caller too).
door=.claude/roadmap-queue
v=$(cd "$R" && sh "$door.sh" --check 2>&1; echo "rc=$?")
has yes "rc=1" "$v" "roadmap.d: a direct call of the parser's front door is held to the rule too"
sed -i.bak 's/Mallory/Ana/' "$R/docs/roadmap.md" && rm -f "$R/docs/roadmap.md.bak"
v=$(inr 'roadmap_queue --check; echo rc=$?'); has yes "rc=0" "$v" "roadmap.d: a queue the rule accepts reads as before"
sed -i.bak 's/Ana/Mallory/' "$R/docs/roadmap.md" && rm -f "$R/docs/roadmap.md.bak"
conf 'MODULES=""'
mkdir -p "$R/.claude/local/roadmap.d"; cp "$R/.claude/modules/demo/roadmap.d/no-mallory.sh" "$R/.claude/local/roadmap.d/"
v=$(inr 'roadmap_queue --check; echo rc=$?'); has yes "local roadmap.d/no-mallory.sh" "$v" "roadmap.d: the local layer's rule runs with no module on"
# A rule's listing that FAILS is not "no rules": with the loader gone and a local rule present,
# the queue is UNKNOWN rather than read unchecked.
mv "$R/.claude/lib/modules.sh" "$R/.claude/lib/modules.sh.off"
v=$(inr 'roadmap_queue --check; echo rc=$?')
has yes "the roadmap rules (roadmap.d) could not be listed" "$v" "roadmap.d: a rule listing that fails makes the queue UNKNOWN, never ruleless"
mv "$R/.claude/lib/modules.sh.off" "$R/.claude/lib/modules.sh"
rm -rf "$R/.claude/local/roadmap.d"
# ...but a roadmap.d/ holding no rule (empty, or only a non-.sh file) is an empty list: the queue
# reads as before, with no module on and with one whose own rule runs (people, Mallory a person).
printf '{"people":[{"name":"Mallory","github":"mallory-gh","role":"owner"}]}\n' > "$R/docs/people.json"
for mods in "" people; do
  conf "MODULES=\"$mods\""
  mkdir -p "$R/.claude/local/roadmap.d"
  v=$(inr 'roadmap_queue --check; echo rc=$?')
  has yes "rc=0" "$v" "roadmap.d: an EMPTY local roadmap.d/ reads as no rules (MODULES=\"$mods\")"
  : > "$R/.claude/local/roadmap.d/.gitkeep"; : > "$R/.claude/local/roadmap.d/x.sh.off"
  v=$(inr 'roadmap_queue --check; echo rc=$?')
  has yes "rc=0" "$v" "roadmap.d: a local roadmap.d/ holding only non-.sh files reads as no rules (MODULES=\"$mods\")"
  rm -rf "$R/.claude/local/roadmap.d"
done
rm -f "$R/docs/people.json"; conf 'MODULES=""'
# A rule that reads the queue itself is refused, not recursed into (each read would run the rule
# again: hundreds of levels, then a fork failure that could read as "no rules" and ACCEPT). The
# rule ignores its inner read's failure, so the outer read completes; it must do so promptly, and
# the inner read must have been refused by name.
mkdir -p "$R/.claude/local/roadmap.d"
door=.claude/roadmap-queue
printf '#!/bin/sh\ncat >/dev/null\nsh %s.sh --tsv > "$ROOT/inner.out" 2>&1\nexit 0\n' "$door" > "$R/.claude/local/roadmap.d/reads-queue.sh"
sed -i.bak 's/Mallory/Ana/' "$R/docs/roadmap.md" && rm -f "$R/docs/roadmap.md.bak"
. "$CLAUDUCTOR_FW/lib/modules.sh"
v=$(cd "$R" && ROOT="$R" CLAUDUCTOR_FW="$R/.claude" with_timeout 60 sh -c '. .claude/lib/conf.sh; roadmap_queue --check' 2>&1; echo "rc=$?")
has yes "rc=0" "$v" "roadmap.d: a rule that reads the queue does not recurse (the outer read finishes)"
has yes "a rule reads its rows on stdin, never the queue" "$(cat "$R/inner.out" 2>/dev/null)" "roadmap.d: ...its own read of the queue is refused by name"
rm -rf "$R/.claude/local/roadmap.d" "$R/inner.out"
sed -i.bak 's/Ana/Mallory/' "$R/docs/roadmap.md" && rm -f "$R/docs/roadmap.md.bak"

# CANNOT CHECK discipline: a section that fails, or says nothing, is never shown as nothing.
conf 'MODULES=""'
printf 'exit 3\n' > "$R/.claude/local/context.d/session-start/broken.sh"
: > "$R/.claude/local/context.d/session-start/silent.sh"
c=$(ext context session-start)
has yes "CANNOT CHECK — .claude/local/context.d/session-start/broken.sh exited 3" "$c" "a context section that fails says CANNOT CHECK"
has yes "CANNOT CHECK — .claude/local/context.d/session-start/silent.sh printed nothing" "$c" "a context section that prints nothing says CANNOT CHECK"
printf 'bad\n' >> "$R/.claude/local/conflicts.tsv"
t=$(ext conflicts); has yes "CANNOT CHECK — this row of .claude/local/conflicts.tsv has no TAB" "$t" "a malformed conflicts.tsv row is named, not dropped"

# The hosts call the loader: the context scripts, every template skill's include line, and the
# session-close table.
grep -q 'extensions.sh health' "$CLAUDUCTOR_FW/skills/session-start/context.sh" && ok "session-start runs the health lines through extensions.sh" || fail "session-start/context.sh does not run extensions.sh health"
# (Each path spelt out, not built from a loop variable: the plugin build rewrites literal paths.)
grep -q "extensions.sh context session-start" "$CLAUDUCTOR_FW/skills/session-start/context.sh" && ok "session-start/context.sh prints the modules' and local layer's sections" || fail "session-start/context.sh does not run extensions.sh context session-start"
grep -q "extensions.sh context session-close" "$CLAUDUCTOR_FW/skills/session-close/context.sh" && ok "session-close/context.sh prints the modules' and local layer's sections" || fail "session-close/context.sh does not run extensions.sh context session-close"
# (The include lines are matched whatever path names extensions.sh: the plugin build rewrites it.)
grep -qE '^!`sh [^ `]*extensions\.sh conflicts`$' "$CLAUDUCTOR_FW/skills/session-close/SKILL.md" && ok "session-close's conflict table includes the modules' and local layer's rows" || fail "session-close/SKILL.md has no conflicts include line"
for sk in "$CLAUDUCTOR_FW"/skills/*/SKILL.md; do
  s=$(basename "$(dirname "$sk")")
  # The project's own skills, and the plugin's scaffolding skill (init), extend nothing; nor does a
  # whole skill a module ships (modules/<m>/skills/<s>/SKILL.md, installed by its enable.sh).
  case $s in architecture-audit | release-prep | init) continue ;; esac
  ls "$CLAUDUCTOR_FW"/modules/*/skills/"$s"/SKILL.md >/dev/null 2>&1 && continue
  grep -qE "^!\`sh [^ \`]*extensions\\.sh fragments $s\`\$" "$sk" || fail "skill $s has no '!\`sh .claude/extensions.sh fragments $s\`' include line under ## Project steps"
done
[ "$_fails" -eq 0 ] && ok "every template skill includes its fragments"

# This project's own modules.
. "$CLAUDUCTOR_FW/lib/modules.sh"
for md in "$CLAUDUCTOR_FW"/modules/*/; do
  [ -d "$md" ] || continue
  m=$(basename "$md")
  [ -f "$md/module.conf" ] || { fail "module $m has no module.conf"; continue; }
  n=$(module_meta "$md" name); [ "$n" = "$m" ] && ok "module $m: module.conf names its directory" || fail "module $m: module.conf says name=\"$n\""
  en=$(module_meta "$md" enables)
  for p in $MODULE_POINTS; do
    declared=no; for e in $en; do [ "$e" = "$p" ] && declared=yes; done
    there=no; [ -e "$md/$p" ] && there=yes
    [ "$declared" = "$there" ] || fail "module $m: enables says $p is $( [ "$declared" = yes ] && echo declared || echo 'not declared'), but it is $( [ "$there" = yes ] && echo present || echo missing)"
  done
  for e in $en; do case " $MODULE_POINTS " in *" $e "*) ;; *) fail "module $m: enables names '$e', which is not a point ($MODULE_POINTS)" ;; esac; done
  for f in $(find "$md" -path "*/guard.d/*.sh" -o -path "*/context.d/*/*.sh" -o -path "*/health/*.sh" -o -path "*/checks/*.sh" -o -path "*/roadmap.d/*.sh" 2>/dev/null); do
    shebang_parse "$f"; rc=$?
    [ "$rc" -eq 0 ] || fail "module $m: ${f#"$md"} does not parse under $(shebang_interp "$f") (rc $rc)"
  done
done
[ "$_fails" -eq 0 ] && ok "every module here declares exactly its parts, and every script part parses"
probs=$(modules_problems)
[ -z "$probs" ] && ok "every module MODULES enables loads (${MODULES:-none})" || fail "MODULES: $probs"
finish
