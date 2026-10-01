#!/bin/sh
# The artifacts module's own check of THIS project's registry (ARTIFACT_REGISTRY), run by
# checks/run.sh as artifacts:registry only while the module is on:
#   - every artifact declares authorities that match something, and a well-formed review stamp
#     (currency.sh --lint: files on disk, no history, so it runs in a container with no .git);
#   - every url has the one form https://claude.ai/artifact/<id>, and no two entries share one
#     (two spellings of one artifact would walk past the duplicate test);
#   - with ARTIFACT_PUBLISH=claude.ai, the registry's "pages" are exactly the docs/*.html files, each
#     with a 40-hex `published` hash: a page nobody registered has no shared copy and no staleness
#     check, and would be absent from every list without an error.
# The module's materials themselves are tested whether it is on or not, by .claude/checks/artifacts.sh.
# shellcheck disable=SC1090
. "${CHECKS_LIB:?run this through checks/run.sh}"
need jq
reg=${ARTIFACT_REGISTRY:-docs/artifacts.json}
cur="$ROOT/.claude/modules/artifacts/bin/currency.sh"
[ -f "$ROOT/$reg" ] || { fail "the artifacts module is on, but $reg does not exist (start it: .claude/modules/artifacts/README.md)"; finish; }

out=$(sh "$cur" --root "$ROOT" --lint 2>&1); rc=$?
if [ "$rc" -eq 0 ]; then ok "$(printf '%s' "$out" | tail -n 1)"
else fail "currency.sh --lint ($reg):"; printf '%s\n' "$out" | sed 's/^/     /'; fi

bad=$(jq -r '
  [to_entries[] | select(.key | startswith("$") | not) | .key as $s
   | (.value | if type == "object" then to_entries[] else empty end) | select(.key | startswith("$") | not)
   | {name: "\($s)/\(.key)", url: (.value.url? // null)}] as $all
  | ($all[] | select((.url | type) != "string" or (.url | test("\\Ahttps://claude\\.ai/artifact/[A-Za-z0-9]{10,40}\\z") | not))
     | "\(.name) has a malformed url: \(.url)"),
    ($all | group_by(.url) | map(select(length > 1 and (.[0].url | type) == "string"))[]
     | "\(map(.name) | join(" and ")) share the url \(.[0].url)")' "$ROOT/$reg" 2>&1)
if [ -z "$bad" ]; then ok "every artifact in $reg has a well-formed url of its own"
else printf '%s\n' "$bad" | while IFS= read -r l; do echo "FAIL $l"; done; _fails=$((_fails + 1)); fi

if [ "${ARTIFACT_PUBLISH:-off}" = claude.ai ]; then
  d=$(scratch)
  (cd "$ROOT" && ls docs/*.html 2>/dev/null) | LC_ALL=C sort > "$d/disk"
  jq -r '.pages // {} | keys[]' "$ROOT/$reg" | LC_ALL=C sort > "$d/reg"
  miss=$(comm -23 "$d/disk" "$d/reg"); gone=$(comm -13 "$d/disk" "$d/reg")
  [ -z "$miss" ] || fail "docs/*.html pages with no shared copy in $reg: $(echo $miss) (publish one from the owner's session with no url, add its URL, then publish.sh --record <page>)"
  [ -z "$gone" ] || fail "$reg registers pages that do not exist: $(echo $gone) (drop their entries)"
  nohash=$(jq -r '.pages // {} | to_entries[] | select((.value.published | type) != "string" or (.value.published | test("\\A[0-9a-f]{40}\\z") | not)) | .key' "$ROOT/$reg")
  [ -z "$nohash" ] || fail "pages with no 40-hex published hash: $(echo $nohash)"
  [ -z "$miss$gone$nohash" ] && ok "ARTIFACT_PUBLISH=claude.ai: every docs/*.html page is registered, with a published hash"
fi
finish
