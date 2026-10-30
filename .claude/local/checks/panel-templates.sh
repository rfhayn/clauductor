#!/bin/sh
# The shipped lane templates are all present, in the preset projects receive and in this repo's own
# panel config (OPS-126, #223 item 3). checks/panel-contract.sh holds each template that exists to
# its contract, so a deleted one would skip its assertions silently. A project's panel.json is its
# own, and may drop a template on purpose (docs/panel.md), so the shipped set is asserted here, in
# this repo's layer, not in the check every project runs.
. "${CHECKS_LIB:?}"
need jq
PRESET="$ROOT/template/.stridal/panel.json"
SET="build propose fix ops close"
missing() {  # missing FILE IDS: each of IDS that FILE's templates lack
  for w in $2; do jq -e --arg w "$w" '[.templates[]?.id] | index($w) != null' "$1" >/dev/null 2>&1 || printf '%s ' "$w"; done
}
# The preset against the shipped set; this repo's config against the preset itself, the authority,
# so a template the preset gains is required here too without editing SET.
[ -f "$PRESET" ] || { fail "template/.stridal/panel.json is missing"; finish; }
m=$(missing "$PRESET" "$SET")
[ -z "$m" ] && ok "template/.stridal/panel.json: the shipped templates ($SET) are all present" || fail "template/.stridal/panel.json: template(s) $m missing, so panel-contract skips their assertions"
ids=$(jq -r '.templates[]?.id' "$PRESET" 2>/dev/null | tr '\n' ' ')
[ -n "$ids" ] || fail "template/.stridal/panel.json lists no template ids"
if [ -f "$ROOT/.stridal/panel.json" ]; then
  m=$(missing "$ROOT/.stridal/panel.json" "$ids")
  [ -z "$m" ] && ok ".stridal/panel.json: every preset template (${ids% }) is present" || fail ".stridal/panel.json: template(s) $m of the preset missing, so panel-contract skips their assertions"
else
  fail ".stridal/panel.json is missing"
fi
# The negative cases: a preset with close deleted, and a config lacking one the preset added.
F=$(scratch)/panel.json
jq 'del(.templates[] | select(.id == "close"))' "$PRESET" > "$F" 2>/dev/null
[ "$(missing "$F" "$SET")" = "close " ] && ok "a preset with close deleted is caught" || fail "a preset with close deleted gave '$(missing "$F" "$SET")', not 'close'"
[ "$(missing "$ROOT/.stridal/panel.json" "$ids new-one")" = "new-one " ] && ok "a template the preset gains is required of this repo's config" || fail "a preset id this repo's config lacks was not caught"
finish
