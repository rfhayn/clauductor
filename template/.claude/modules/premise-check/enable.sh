#!/bin/sh
# enable.sh: put the premise check into the panel's fix lane, so a fix starts from it.
#
#   sh .claude/modules/premise-check/enable.sh           add panel/fix-prompt.txt's sentence to the
#                                                        first_prompt of every lane_type "fix"
#                                                        template in .clauductor/panel.json
#   sh .claude/modules/premise-check/enable.sh --check   ok / FAIL lines: is it there (checks/project.sh)
#
# The sentence names the command as a reader types it here (premise_command, lib/receipt.sh). A
# template already naming premise-check.sh is left alone. jq rewrites the file, so its layout
# becomes jq's; the content is unchanged but for the sentence. No panel config: nothing to do (the
# panel is optional, D10), said as such. Then set MODULES="premise-check" in .claude/project.conf.
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || ROOT=$(pwd)
# shellcheck disable=SC1091
. "$HERE/lib/receipt.sh"
# The panel's config, named once: messages print $REL (checks/no-clauductor.sh reads every echo
# line that names the product for one that asks the user to install it).
REL=.clauductor/panel.json
PANEL="$ROOT/$REL"
cmd=$(premise_command "$ROOT" "$HERE")
line=$(sed "s|{cmd}|$cmd|" "$HERE/panel/fix-prompt.txt")

if [ ! -f "$PANEL" ]; then
  echo "ok   no $REL, so there is no panel fix lane to tell (the panel is optional)"
  exit 0
fi
command -v jq >/dev/null 2>&1 || { echo "FAIL jq is not installed, so $REL cannot be read"; exit 1; }
fixes=$(jq -r '[.templates[]? | select(.lane_type == "fix")] | length' "$PANEL" 2>/dev/null) \
  || { echo "FAIL $REL is not valid JSON"; exit 1; }
missing=$(jq -r '.templates[]? | select(.lane_type == "fix") | select((.first_prompt // "") | contains("premise-check.sh") | not) | .id' "$PANEL")

if [ "${1:-}" = --check ]; then
  if [ "$fixes" -eq 0 ]; then echo "ok   $REL has no fix lane template, so no lane starts a fix without the check"
  elif [ -z "$missing" ]; then echo "ok   every fix lane in $REL starts from the premise check"
  else echo "FAIL the fix lane template(s) $(echo $missing) in $REL do not run the premise check first: run sh ${HERE#"$ROOT"/}/enable.sh"; exit 1; fi
  exit 0
fi
[ -n "$missing" ] || { echo "ok   every fix lane in $REL already starts from the premise check"; exit 0; }
tmp="$PANEL.premise.$$"
jq --arg s "$line" '.templates |= map(if .lane_type == "fix" and ((.first_prompt // "") | contains("premise-check.sh") | not)
  then .first_prompt = ((.first_prompt // "") | if . == "" then $s else . + " " + $s end) else . end)' "$PANEL" > "$tmp" \
  && mv "$tmp" "$PANEL" || { rm -f "$tmp"; echo "FAIL could not rewrite $REL"; exit 1; }
echo "ok   added the premise check to the fix lane template(s): $(echo $missing)"
