#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The premise-check module's own check, run by checks/run.sh only while the module is on: on THIS
# project, the script produces a report that ends in a receipt (from an issue fixture, so no
# network), the guard rule and its library parse, PREMISE_REQUIRED_ON names a branch prefix, and
# every panel fix lane starts from the check (enable.sh --check). The module's materials are tested
# whether it is on or not, by .claude/checks/premise-check.sh.
# shellcheck disable=SC1090
. "${CHECKS_LIB:?run this through checks/run.sh}"
need git jq

M=$(cd "$(dirname "$0")/.." && pwd)
d=$(scratch)

[ -n "${PREMISE_REQUIRED_ON:-}" ] && ok "PREMISE_REQUIRED_ON names the branches that need a receipt: $PREMISE_REQUIRED_ON" \
  || fail "PREMISE_REQUIRED_ON is empty, so no PR needs a premise-check receipt"
for f in "$M/guard.d/premise.sh" "$M/lib/receipt.sh" "$M/premise-check.sh" "$M/enable.sh"; do
  sh -n "$f" 2>/dev/null && ok "${f#"$M"/} parses" || fail "${f#"$M"/} does not parse: sh -n $f"
done

if git -C "$ROOT" rev-parse -q --verify 'HEAD^{commit}' >/dev/null 2>&1; then
  jq -n '{number: 1, title: "check", createdAt: "2020-01-01T00:00:00Z", body: "Does `README.md` still say \"premise-check self test\"?"}' > "$d/issue.json"
  out=$(cd "$ROOT" && sh "$M/premise-check.sh" --issue-json "$d/issue.json" --at HEAD 2>&1); rc=$?
  last=$(printf '%s\n' "$out" | tail -n 1)
  case $rc:$last in
    "0:premise-check: #1 @ "*) ok "premise-check.sh reports on this project and ends with its receipt ($last)" ;;
    *) fail "premise-check.sh did not report on this project (exit $rc): $(printf '%s' "$out" | tail -n 3)" ;;
  esac
else
  fail "this project has no commit, so premise-check.sh has nothing to check against"
fi

out=$(cd "$ROOT" && sh "$M/enable.sh" --check 2>&1)
printf '%s\n' "$out" | grep -E '^(ok|FAIL)' > "$d/panel"
cat "$d/panel"; _fails=$((_fails + $(grep -c '^FAIL' "$d/panel")))
finish
