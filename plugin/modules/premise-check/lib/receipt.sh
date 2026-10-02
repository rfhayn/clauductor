# receipt.sh: sourced by the premise-check module's guard rule (guard.d/premise.sh) and by the
# checks. POSIX sh; needs only grep and awk. Ported unchanged from Standing Tee's
# .claude/hooks/lib/premise-receipt.sh, where it decided merge-guard rule 6.
#
# WHAT IT DECIDES. Whether a PR's body carries a premise-check receipt (the last line
# premise-check.sh prints, `premise-check: #N @ <sha>`) for EVERY issue the PR closes. The issue
# list is GitHub's own `closingIssuesReferences` (the authority for what the merge will close),
# UNIONED with a parse of the body by GitHub's own keyword rules (closing_refs_in_body), because
# that field is computed asynchronously and can read empty at merge time.
#
# WHAT IT CANNOT DECIDE. That the check ran BEFORE the fix started, or that anyone read it. A
# receipt pasted at merge time passes. What it does guarantee is that no fix reaches main without
# the drift report having been produced and put where the reviewer reads: presence, not correctness.

# closing_refs_in_body — $1 = PR body, $2 = this repo as lowercased owner/name ("" if unknown).
# Prints, space-separated and sorted, every issue the body closes by GitHub's keywords:
# close/closes/closed/fix/fixes/fixed/resolve/resolves/resolved, any case, an optional colon, then
# #N, owner/name#N or https://github.com/owner/name/issues/N. A reference to another repository is
# skipped; with $2 empty none can be told apart, so all are kept (more receipts, never fewer).
closing_refs_in_body() {
  # Code is not a reference: GitHub links neither `inline code` nor a ``` / ~~~ fenced block.
  printf '%s\n' "$1" \
    | awk '/^[ \t]*(```|~~~)/ { fence = !fence; next } !fence { gsub(/`[^`]*`/, ""); print }' \
    | grep -oiE '(^|[^A-Za-z0-9_])(close[sd]?|fix(e[sd])?|resolve[sd]?)[[:space:]]*:?[[:space:]]*((https?://(www\.)?github\.com/)?[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+(#|/issues/)|#)[0-9]+' \
    | awk -v here="$2" '{
        s = tolower($0); n = s; sub(/.*[^0-9]/, "", n)
        if (match(s, /[a-z0-9_.-]+\/[a-z0-9_.-]+(#|\/issues\/)[0-9]+$/)) {
          r = substr(s, RSTART, RLENGTH); sub(/(#|\/issues\/)[0-9]+$/, "", r)
          if (here != "" && r != here) next
        }
        print n + 0
      }' \
    | sort -un | tr '\n' ' ' | sed 's/ $//'
}

# $1 = PR body, $2 = space-separated issue numbers the PR closes.
# Prints the issue numbers that have NO receipt, space-separated; returns 1 if any are missing.
premise_receipt_missing() {
  _body=$1
  _missing=""
  for _n in $2; do
    if ! printf '%s\n' "$_body" | grep -qE "premise-check: #$_n @ [0-9a-f]{7,40}"; then
      _missing="$_missing #$_n"
    fi
  done
  _missing=${_missing# }
  [ -z "$_missing" ] && return 0
  printf '%s\n' "$_missing"
  return 1
}

# premise_command ROOT MODULE_DIR: the command a reader runs to produce a receipt, as they would
# type it at ROOT. The module's own copy when it sits in the repository; the plugin's resolver when
# the module comes from the clauductor plugin; else the script's full path.
premise_command() {
  case $2 in
    "$1"/*) printf 'sh %s/premise-check.sh\n' "${2#"$1"/}" ;;
    *) if [ -f "$1/scripts/ci/clauductor-model.sh" ]; then
         printf 'sh scripts/ci/clauductor-model.sh modules/premise-check/premise-check.sh\n'
       else printf 'sh %s/premise-check.sh\n' "$2"; fi ;;
  esac
}
