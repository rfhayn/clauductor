#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The ideas module's check of THIS project, run by checks/run.sh as ideas:queue only while the
# module is on:
#   - IDEAS_FILE is the renderer's output, unedited (render.sh --check): a hand edit reads exactly
#     like a rendered file, and the next close overwrites it without a word;
#   - the registry gives the Ideas page (IDEAS_PAGE) a url, so /ideas and the session steps can
#     reach the queue, and the page's source is in the repository;
#   - .claude/skills/ideas is the module's skill (enable.sh --check): a skill that is missing or
#     stale is a /ideas that is not there, or one that writes the wrong shape.
# The module's materials themselves are tested whether it is on or not, by .claude/checks/ideas.sh.
# shellcheck disable=SC1090
. "${CHECKS_LIB:?run this through checks/run.sh}"
need jq
r="$CLAUDUCTOR_FW/modules/ideas/bin/render.sh"
f=${IDEAS_FILE:-docs/ideas.md}
page=${IDEAS_PAGE:-docs/ideas.html}

if [ ! -f "$ROOT/$f" ]; then fail "$f does not exist (sh "$CLAUDUCTOR_FW"/modules/ideas/enable.sh renders an empty one)"
elif out=$(sh "$r" --check "$ROOT/$f" 2>&1); then ok "$f is the renderer's output, unedited"
else fail "$(printf '%s' "$out" | sed "s|^ideas-render: $ROOT/||") (render it at the close: the module's session-close step)"; fi

if u=$(sh "$r" --url 2>&1); then ok "the registry gives the Ideas page ($page) a url: $u"
else fail "$(printf '%s' "$u" | sed 's/^ideas-render: //')"; fi
[ -f "$ROOT/$page" ] && ok "the Ideas page's source $page is in the repository" \
  || fail "$page does not exist (sh "$CLAUDUCTOR_FW"/modules/ideas/enable.sh makes it from the module's page)"

if out=$(sh "$CLAUDUCTOR_FW/modules/ideas/enable.sh" --check 2>&1); then ok "$(printf '%s\n' "$out" | grep 'skills/ideas' | sed 's/^ok *//')"
else printf '%s\n' "$out" | grep '^FAIL' | while IFS= read -r l; do echo "$l"; done; _fails=$((_fails + 1)); fi
finish
