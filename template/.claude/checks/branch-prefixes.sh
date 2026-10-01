#!/bin/sh
# Branch names are the project's: BRANCH_CHANGE, BRANCH_FIX and BRANCH_OPS in .claude/project.conf
# (defaults in .claude/lib/conf.sh). The OPS-8 rehearsal found them configurable but written as
# literals in the skills, the build-change workflow and the panel preset, so a project that set
# BRANCH_CHANGE="feature/" got a model contradicting itself. This check holds both ends:
#
#   1. The model's own files name a branch by its key, never by a literal prefix. It enumerates
#      every file in the framework directories (skills, hooks, lib, workflows, modules), every
#      script directly in .claude/, the agents, and the gate's scripts, and fails on each literal
#      `change/` (and, in skills and workflows, `fix/` and `ops/`). Not counted: the line that
#      sets a key's default (`BRANCH_CHANGE="change/"`), the roadmap's `ops/check-outcome-<id>`
#      row ids (a record grammar, read by roadmap-queue.sh), and a line a project marks
#      `branch-literal-ok`. The checks and examples are test fixtures and are not scanned.
#   2. .clauductor/panel.json, when there is one, agrees with the keys: a lane for each prefix, no
#      lane on a default the project does not use, and every template's branch_pattern starting
#      with a configured prefix (`clauductor install` writes it so; this catches drift after).
. "$(dirname "$0")/lib.sh"

# scan DIR: prints "path:line: text" for every literal prefix in DIR's framework files.
# (DIR is a project root: the scan reads the project's own copy of the model, whichever install
# path put it there; under the plugin that copy is only the project's own skills and scripts.)
scan() {
  ( cd "$1" || exit 0
    { for sub in skills hooks lib workflows modules agents; do find "./.claude/$sub" -type f 2>/dev/null; done
      ls ./.claude/*.sh ./scripts/ci/run-local.sh ./scripts/ci/gate.sh ./scripts/ci/lease.sh 2>/dev/null; } | sed 's|^\./||' | LC_ALL=C sort -u |
    while IFS= read -r f; do
      case $f in */.DS_Store) continue ;; esac
      re='(^|[^A-Za-z0-9_.-])change/'
      case ${f#.claude/} in skills/* | workflows/*) re='(^|[^A-Za-z0-9_.-])(change|fix|ops)/' ;; esac
      grep -nE "$re" "$f" 2>/dev/null | grep -vE 'BRANCH_(CHANGE|FIX|OPS)=|ops/check-outcome|branch-literal-ok' | sed "s|^|$f:|"
    done )
}

# Self-test: a literal in a skill and in a hook fails; a default and a roadmap id do not.
d=$(scratch)
mkdir -p "$d/t/.claude/skills/x" "$d/t/.claude/hooks" "$d/t/.claude/lib"
printf 'Work on `change/<id>`.\nLand it on an `ops/` branch.\n' > "$d/t/.claude/skills/x/SKILL.md"
printf 'case $b in fix/*) ;; esac\nBRANCH_CHANGE="change/"\n' > "$d/t/.claude/hooks/h.sh"
printf 'BRANCH_CHANGE="change/"\nqueue `ops/check-outcome-<id>`\nfor changes/ and exchange/ only\n' > "$d/t/.claude/lib/conf.sh"
got=$(scan "$d/t")
[ "$(printf '%s\n' "$got" | grep -c .)" = 2 ] && printf '%s\n' "$got" | grep -q 'skills/x/SKILL.md:1:' && printf '%s\n' "$got" | grep -q 'skills/x/SKILL.md:2:' \
  && ok "self-test: literal change/ and ops/ in a skill are found; a default, a roadmap id, changes/ and a hook's fix/ are not" \
  || fail "self-test: the scan found [$got]"

found=$(scan "$ROOT")
if [ -n "$found" ]; then
  fail "the model's files name a branch by a literal prefix; use BRANCH_CHANGE/BRANCH_FIX/BRANCH_OPS (.claude/project-config.sh prints them):"
  printf '%s\n' "$found" | sed 's/^/       /'
else
  ok "no framework file names a branch by a literal prefix (BRANCH_CHANGE=$BRANCH_CHANGE BRANCH_FIX=$BRANCH_FIX BRANCH_OPS=$BRANCH_OPS)"
fi

# The panel preset against the keys.
panel_problems() { # PANEL CHANGE FIX OPS
  jq -r --arg c "$2" --arg f "$3" --arg o "$4" '
    (.lanes // {}) as $l
    | ([$c, $f, $o] | map(select(. as $p | $l | has($p) | not) | "lanes has no \"\(.)\" for its branch prefix")[]),
      ((["change/", "fix/", "ops/"] - [$c, $f, $o]) | map(select(. as $p | $l | has($p)) | "lanes has \"\(.)\", a default prefix this project does not use")[]),
      ((.templates // [])[] | select((.branch_pattern // "") | contains("/"))
        | select(.branch_pattern as $b | [$c, $f, $o] | map(select(. as $p | $p != "" and ($b | startswith($p)))) | length == 0)
        | "template \"\(.id)\" has branch_pattern \"\(.branch_pattern)\", which starts with no configured prefix")' "$1"
}
if command -v jq >/dev/null 2>&1; then
  printf '{"lanes": {"change/": "build", "fix/": "fix", "ops/": "ops"}, "templates": [{"id": "b", "branch_pattern": "change/{name}"}]}\n' > "$d/p.json"
  n=$(panel_problems "$d/p.json" feature/ fix/ ops/ | grep -c .)
  [ "$n" = 3 ] && ok "self-test: a preset on change/ disagrees with BRANCH_CHANGE=feature/ three ways" || fail "self-test: the panel comparison found $n problem(s), want 3"
  [ -z "$(panel_problems "$d/p.json" change/ fix/ ops/)" ] && ok "self-test: the preset agrees with the default keys" || fail "self-test: the preset disagrees with the defaults"
  p="$ROOT/.clauductor/panel.json"
  if [ -f "$p" ]; then
    probs=$(panel_problems "$p" "$BRANCH_CHANGE" "$BRANCH_FIX" "$BRANCH_OPS" 2>&1)
    if [ -n "$probs" ]; then
      fail ".clauductor/panel.json disagrees with the branch keys in .claude/project.conf:"
      printf '%s\n' "$probs" | sed 's/^/       /'
    else
      ok ".clauductor/panel.json's lanes and branch patterns follow BRANCH_CHANGE, BRANCH_FIX and BRANCH_OPS"
    fi
  else
    ok "no .clauductor/panel.json in this project"
  fi
else
  fail "cannot run: jq is not installed (the panel comparison needs it)"
fi
finish
