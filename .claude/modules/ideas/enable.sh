#!/bin/sh
# enable.sh: switch the ideas module on. Makes what the module needs in the project, never
# overwriting the project's own:
#   - .claude/skills/ideas/SKILL.md, the /ideas skill, from this module's skill/SKILL.md (a copy, not
#     a link: a link into a plugin's cache breaks when the plugin moves). Its `model:` and `effort:`
#     lines are the project's (checks/model-roles.sh holds them to the role it maps), so a refresh
#     keeps them and the comparison ignores them;
#   - IDEAS_PAGE (default docs/ideas.html), the Ideas page's source, from page/ideas.html with the
#     project's name and owner filled in, when it does not exist;
#   - IDEAS_FILE (default docs/ideas.md), rendered from an empty queue, when it does not exist.
#
#   sh .claude/modules/ideas/enable.sh          make them
#   sh .claude/modules/ideas/enable.sh --check  only report, each one; exit 1 if any is missing
#
# Then publish the page once (its README), register its url, and add ideas to MODULES.
ROOT=$(cd "$(dirname "$0")/../../.." 2>/dev/null && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
CHECK=""; [ "${1:-}" = --check ] && CHECK=1
rc=0
page=${IDEAS_PAGE:-docs/ideas.html}
f=${IDEAS_FILE:-docs/ideas.md}
mod="$ROOT/.claude/modules/ideas"

# How a project runs the module's tools: the template's path, or, from the plugin, its command.
case ${CLAUDUCTOR_FW:-} in '' | */.claude) plugin="" ;; *) plugin=1 ;; esac
# The two spellings of the tool. Held in variables on purpose: the plugin build rewrites a literal
# `sh .claude/...` in a script to the plugin's copy, which would rewrite these patterns too.
tpl=".claude/modules/ideas/bin/render.sh"
plg="clauductor-model modules/ideas/bin/render.sh"
# skill_body FILE: FILE without its frontmatter's model: and effort: lines, the tool spelt the
# template's way (so a plugin project's copy compares equal to the module's).
skill_body() {
  awk 'NR==1 && $0=="---" { fm=1; print; next } fm && $0=="---" { fm=0 } fm && /^(model|effort):/ { next } { print }' "$1" \
    | sed "s|$plg|sh $tpl|g"
}
src="$mod/skill/SKILL.md"
# Spelt with the quote before /skills on purpose: the destination is the PROJECT's .claude/, which
# the plugin build must not rewrite to the plugin's own copy.
dst="$ROOT/.claude"/skills/ideas/SKILL.md
mark="installed by the ideas module"
if [ -f "$dst" ] && [ "$(skill_body "$src")" = "$(skill_body "$dst")" ]; then
  echo "ok   .claude/skills/ideas is the module's /ideas skill"
elif [ -f "$dst" ] && ! grep -qF "$mark" "$dst"; then
  # A project's own /ideas skill (no module marker) is never overwritten: it is theirs.
  echo "FAIL .claude/skills/ideas/SKILL.md exists and is not the ideas module's (it lacks the \"$mark\" line): rename or remove the project's own skill, then run this again"; rc=1
elif [ -n "$CHECK" ]; then
  echo "FAIL .claude/skills/ideas $( [ -f "$dst" ] && echo 'differs from' || echo 'is not installed from') .claude/modules/ideas/skill (run sh .claude/modules/ideas/enable.sh)"; rc=1
else
  keep=""; [ -f "$dst" ] && keep=$(awk 'NR>1 && $0=="---" { exit } NR>1 && /^(model|effort):/' "$dst")
  mkdir -p "$(dirname "$dst")" &&
    KEEP="$keep" awk 'BEGIN { n = split(ENVIRON["KEEP"], k, "\n"); for (i = 1; i <= n; i++) { split(k[i], kv, ":"); mine[kv[1]] = k[i] } }
      NR==1 && $0=="---" { fm=1; print; next } fm && $0=="---" { fm=0 }
      fm && /^(model|effort):/ { split($0, kv, ":"); if (kv[1] in mine) { print mine[kv[1]]; next } }
      { print }' "$src" \
    | if [ -n "$plugin" ]; then sed "s|sh $tpl|$plg|g"; else cat; fi > "$dst" &&
    echo "made .claude/skills/ideas (map it in .claude/model-roles.json: \"ideas\": \"orient\", and add its row to the playbook's skills table)"
fi

if [ -f "$ROOT/$page" ]; then echo "ok   $page exists (the Ideas page's source)"
elif [ -n "$CHECK" ]; then echo "FAIL $page does not exist (run this without --check to make it from the module's page)"; rc=1
else
  # The page's text names the project and who scopes the queue; both are plain text in the page.
  owner=${OWNER_NAME:-the ${OWNER_ROLE:-owner}}
  esc() { printf '%s' "$1" | sed 's/&/\&amp;/g; s/</\&lt;/g; s/>/\&gt;/g; s/[|\\]/\\&/g'; }
  mkdir -p "$(dirname "$ROOT/$page")" &&
    sed "s|__PROJECT__|$(esc "${PROJECT_NAME:-This project}")|g; s|__OWNER__|$(esc "$owner")|g" "$mod/page/ideas.html" > "$ROOT/$page" &&
    echo "made $page (publish it once with capabilities {\"db\": {}, \"user\": {\"scopes\": [\"profile\"]}}, then register its url: .claude/modules/ideas/README.md)"
fi

if [ -f "$ROOT/$f" ]; then echo "ok   $f exists"
elif [ -n "$CHECK" ]; then echo "FAIL $f does not exist (run this without --check to render an empty queue)"; rc=1
else
  e=$(mktemp -d "${TMPDIR:-/tmp}/ideas-enable.XXXXXX") && mkdir -p "$e/ideas" "$(dirname "$ROOT/$f")" &&
    sh "$mod/bin/render.sh" "$e" --out "$ROOT/$f" && echo "made $f (an empty queue; every close renders it from the page)"
  rm -rf "$e"
fi
[ -n "$CHECK" ] || echo "next: add ideas to MODULES in .claude/project.conf (it requires the artifacts module)"
exit "$rc"
