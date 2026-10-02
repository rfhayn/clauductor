#!/bin/sh
# session-start's ideas section: the Ideas page's link (from the registry) and what the last close
# rendered into IDEAS_FILE (its counts and its data-through stamp). The LIVE count needs the page's
# database, which no shell reads: the module's session-start step reads it with ArtifactData and
# compares it with this file. Never silent: a missing url or file is CANNOT CHECK. No network.
ROOT=$(cd "$(dirname "$0")/../../../../.." 2>/dev/null && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
r="$ROOT/.claude/modules/ideas/bin/render.sh"
f=${IDEAS_FILE:-docs/ideas.md}
if u=$(sh "$r" --url 2>&1); then echo "Ideas page: $u (live count: the module's session-start step)"
else echo "CANNOT CHECK — the Ideas page: $(printf '%s' "$u" | sed 's/^ideas-render: //')"; fi
if [ ! -f "$ROOT/$f" ]; then
  echo "CANNOT CHECK — $f does not exist (sh .claude/modules/ideas/enable.sh makes it; every close renders it)"
  exit 0
fi
if why=$(sh "$r" --check "$ROOT/$f" 2>&1); then
  through=$(head -n 1 "$ROOT/$f" | sed -n 's/.* data-through: \([^ ]*\) · .*/\1/p')
  counts=$(sed -n 's/^## \(.*\) (\([0-9][0-9]*\))$/\2 \1/p' "$ROOT/$f" | awk '{ n = $1; $1 = ""; sub(/^ /, ""); printf "%s%s %s", (NR > 1 ? ", " : ""), n, tolower($0) }')
  echo "$f as the last close rendered it (data through ${through:-?}): ${counts:-no status sections}"
else
  echo "STALE — $f is not the renderer's output: $(printf '%s' "$why" | sed 's/^ideas-render: //')"
fi
exit 0
