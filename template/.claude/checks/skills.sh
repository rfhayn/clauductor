#!/bin/sh
# Every skill is wired to things that exist (AGENTS.md rule 4: a delegation target is a named
# thing, and a missing instruction gets interpreted rather than failing): its frontmatter name is
# its directory's, it has a description, every `!`sh …`` context line names a file that exists,
# and every context script runs to completion in this checkout. Also every script path a skill or
# agent names under .claude/ exists.
. "$(dirname "$0")/lib.sh"

for f in "$ROOT"/.claude/skills/*/SKILL.md; do
  [ -f "$f" ] || continue
  s=$(basename "$(dirname "$f")")
  n=$(awk 'NR>1 && /^---$/{exit} /^name:/{sub(/^name:[ \t]*/, ""); print; exit}' "$f")
  [ "$n" = "$s" ] && ok "skill $s: name matches its directory" || fail "skill $s: frontmatter name is '$n'"
  awk 'NR>1 && /^---$/{exit} /^description:/{found=1} END{exit !found}' "$f" || fail "skill $s has no description"
  grep -oE '!`(sh|bash) [^`]+`' "$f" | sed -E 's/^!`(sh|bash) //; s/`$//' | while read -r script _; do
    if [ -f "$ROOT/$script" ]; then
      out=$(cd "$ROOT" && sh "$script" 2>&1 >/dev/null </dev/null); rc=$?
      [ "$rc" -eq 0 ] && echo "ok   skill $s: $script runs" || echo "FAIL skill $s: $script exits $rc: $(printf '%s' "$out" | tail -1)"
    else
      echo "FAIL skill $s: its context line names $script, which does not exist"
    fi
  done > "$(scratch)/ctx"
  cat "$(scratch)/ctx"; _fails=$((_fails + $(grep -c '^FAIL' "$(scratch)/ctx")))
done

# Paths to scripts under .claude/ named in skills, agents, AGENTS.md and settings.json.
for p in $(cat "$ROOT"/.claude/skills/*/SKILL.md "$ROOT"/.claude/agents/*.md "$ROOT/AGENTS.md" "$ROOT/.claude/settings.json" 2>/dev/null \
  | grep -oE '\.claude/[A-Za-z0-9_./-]+\.(sh|js|awk|json)' | sort -u); do
  [ -e "$ROOT/$p" ] || fail "named but missing: $p"
done
[ "$_fails" -eq 0 ] && ok "every .claude/ script path named in skills, agents, AGENTS.md and settings exists"
finish
