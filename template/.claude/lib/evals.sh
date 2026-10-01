# evals.sh: sourced by checks/model-roles.sh and by pr-merge-guard.sh (rule 13, through
# hooks/lib/change-guard.sh). What makes an eval receipt count as evidence for a role's model and
# effort (OPS-10), and which inputs make a role what it is (OPS-16). Receipts are written by
# .claude/evals/run.sh.
#
# THE HASHES ARE GIT BLOB IDS (`git hash-object`), so a hash computed from the working tree when
# the eval ran equals one read from a commit's objects when the guard runs, with no sha256 tool to
# differ between macOS and Linux. The working-tree functions below are duplicated, on purpose, in
# .claude/evals/run.sh: the evals directory is the project's (the plugin scaffolds it), so the
# runner cannot source this framework library. checks/evals.sh asserts the copies match.
#
# WHAT A RECEIPT IS HELD TO (OPS-16) is exactly what makes the role what it is, and nothing else:
#   - the role's own choice in model-roles.json: its model, effort and tier variants
#     (evals_role_hash). Not the whole file: the evidence is recorded in the same file
#     (roles.<r>.eval), and another role's choice does not change this one;
#   - its TRIGGERS, declared in model-roles.json .evals.triggers.<role> as a list of inputs:
#       path          a file (the agent: .claude/agents/reviewer.md);
#       path#MARKER   the lines of path between `// <MARKER>` and `// </MARKER>` (or `# <MARKER>`),
#                     each on a line of its own, once, in order: the review prompt and findings
#                     schema inside .claude/workflows/build-change.js, so other workflow edits do
#                     not need a new eval;
#       dir/          every file under dir.
#     A role with a suite that declares no triggers gets the broad default: its agent file and the
#     whole of .claude/workflows/ (the rule before OPS-16).
# A section whose markers are not there hashes as "markers-missing": the runner refuses to run on
# it, and the guard blocks a PR that changes such a file, since it cannot tell what changed.
#
# Needs: jq, git.

# ── Working tree (identical in .claude/evals/run.sh) ─────────────────────────────────────────
evals_roles_hash() {  # evals_roles_hash MODEL_ROLES_JSON_FILE: every role's choice (receipt context only)
  jq -cS '.roles | map_values({model: .model, effort: .effort, tiers: .tiers})' "$1" 2>/dev/null | git hash-object --stdin
}
evals_role_hash() {  # evals_role_hash ROLE MODEL_ROLES_JSON_FILE: one role's model, effort and tier variants
  jq -cS --arg r "$1" '(.roles[$r] // {}) | {model: .model, effort: .effort, tiers: .tiers}' "$2" 2>/dev/null | git hash-object --stdin
}
evals_triggers() {  # evals_triggers ROLE MODEL_ROLES_JSON_FILE: the role's trigger inputs, one per line, sorted
  jq -r --arg r "$1" '
    ([(.agents // {}) | to_entries[] | select(.value == $r) | .key] | first // $r) as $a
    | ((.evals.triggers // {})[$r] // [".claude/agents/\($a).md", ".claude/workflows/"]) | .[]' "$2" 2>/dev/null | LC_ALL=C sort -u
}
evals_section() {  # evals_section MARKER < FILE: the lines between the MARKER lines; fails unless each is there once, in order
  awk -v o="<$1>" -v c="</$1>" '
    { t = $0; m = (t ~ /^[ \t]*(\/\/|#)/); sub(/^[ \t]*(\/\/|#)[ \t]*/, "", t); sub(/[ \t]+$/, "", t) }
    m && t == o { no++; if (inside || no > 1) bad = 1; inside = 1; next }
    m && t == c { nc++; if (!inside || nc > 1) bad = 1; inside = 0; next }
    inside { print }
    END { if (bad || no != 1 || nc != 1 || inside) exit 1 }'
}
evals_blob() {  # evals_blob FILE: its blob id, or "none"
  if [ -f "$1" ]; then git hash-object "$1"; else echo none; fi
}
evals_tree_hash() {  # evals_tree_hash DIR REL: one id for every file under DIR, named by REL/<path>
  if [ -d "$1" ]; then
    (cd "$1" && find . -type f ! -name .DS_Store | sed 's|^\./||' | LC_ALL=C sort | while IFS= read -r _f; do
      printf '%s %s/%s\n' "$(git hash-object "$_f")" "$2" "$_f"
    done)
  fi | LC_ALL=C sort -k2 | git hash-object --stdin
}
evals_input_hash() {  # evals_input_hash ROOT INPUT: one trigger input's id; "none" if absent, "markers-missing" for a section that is not there
  case "$2" in
    *'#'*)
      if [ ! -f "$1/${2%%#*}" ]; then echo none
      elif _es=$(evals_section "${2#*#}" < "$1/${2%%#*}"); then printf '%s\n' "$_es" | git hash-object --stdin
      else echo markers-missing; fi ;;
    */) evals_tree_hash "$1/${2%/}" "${2%/}" ;;
    *) evals_blob "$1/$2" ;;
  esac
}
# ── end of the duplicated functions ───────────────────────────────────────────────────────────

# The same, read from a commit instead of the working tree.
evals_roles_hash_at() {  # evals_roles_hash_at REV PATH
  git show "$1:$2" 2>/dev/null | jq -cS '.roles | map_values({model: .model, effort: .effort, tiers: .tiers})' 2>/dev/null | git hash-object --stdin
}
evals_role_hash_at() {  # evals_role_hash_at REV ROLE: evals_role_hash of .claude/model-roles.json at REV
  git show "$1:.claude/model-roles.json" 2>/dev/null | evals_role_hash "$2" /dev/stdin
}
evals_blob_at() {  # evals_blob_at REV PATH
  git rev-parse -q --verify "$1:$2" 2>/dev/null || echo none
}
evals_tree_hash_at() {  # evals_tree_hash_at REV DIR (repo-relative, no trailing slash)
  git ls-tree -r "$1" -- "$2/" 2>/dev/null | awk -F'\t' '{ split($1, m, " "); print m[3] " " $2 }' | grep -v '/\.DS_Store$' \
    | LC_ALL=C sort -k2 | git hash-object --stdin
}
evals_triggers_at() {  # evals_triggers_at REV ROLE: the trigger inputs .claude/model-roles.json declares at REV
  git show "$1:.claude/model-roles.json" 2>/dev/null | evals_triggers "$2" /dev/stdin
}
evals_input_hash_at() {  # evals_input_hash_at REV INPUT: evals_input_hash, read from a commit
  case "$2" in
    *'#'*)
      if ! git cat-file -e "$1:${2%%#*}" 2>/dev/null; then echo none
      elif _es=$(git show "$1:${2%%#*}" | evals_section "${2#*#}"); then printf '%s\n' "$_es" | git hash-object --stdin
      else echo markers-missing; fi ;;
    */) evals_tree_hash_at "$1" "${2%/}" ;;
    *) evals_blob_at "$1" "$2" ;;
  esac
}

# evals_verdict RECEIPT ROLE MODEL EFFORT MODEL_ROLES_JSON: prints why RECEIPT is not passing
# evidence for ROLE on MODEL/EFFORT, one reason per line, or nothing when it is. The scores are
# judged against the thresholds in MODEL_ROLES_JSON (.evals.thresholds) NOW, not against the
# `pass` the runner wrote: a threshold raised since the run applies to it.
evals_verdict() {
  if ! jq -e . "$1" >/dev/null 2>&1; then echo "$1 is missing or not valid JSON"; return; fi
  jq -r --arg r "$2" --arg m "$3" --arg e "$4" --slurpfile mr "$5" '
    ($mr[0].evals.thresholds // {}) as $t
    | (.scores // {}) as $s
    | [ (if .schema != 1 then "schema is \(.schema // "absent"), not 1" else empty end),
        (if .role != $r then "it evaluates role \(.role // "?"), not \($r)" else empty end),
        (if .model != $m or .effort != $e then "it evaluates \(.model // "?")/\(.effort // "?"), not \($m)/\($e)" else empty end),
        (if .complete != true then "it ran part of the suite (--cases), not all of it" else empty end),
        (if (.errors // 1) != 0 then "\(.errors // "?") case(s) failed to run" else empty end),
        (if ($s.recall | type) != "number" or $s.recall < ($t.recall // 0.8) then "recall \($s.recall) is below \($t.recall // 0.8)" else empty end),
        (if ($s.fp_rate | type) != "number" or $s.fp_rate > ($t.fp_rate // 0.2) then "false-positive rate \($s.fp_rate) is above \($t.fp_rate // 0.2)" else empty end),
        (if ($s.severity_accuracy | type) != "number" or $s.severity_accuracy < ($t.severity_accuracy // 0) then "severity accuracy \($s.severity_accuracy) is below \($t.severity_accuracy // 0)" else empty end)
      ] | .[]' "$1" 2>/dev/null || echo "$1 could not be read"
}

# evals_suite_roles ROOT: the roles that have a suite (.claude/evals/<role>/cases/<id>/case.json).
evals_suite_roles() {
  for _d in "$1"/.claude/evals/*/cases; do
    [ -d "$_d" ] || continue
    ls "$_d"/*/case.json >/dev/null 2>&1 && basename "$(dirname "$_d")"
  done
}
evals_suite_roles_at() {  # evals_suite_roles_at REV
  git ls-tree -r --name-only "$1" -- .claude/evals/ 2>/dev/null | sed -n 's|^\.claude/evals/\([^/]*\)/cases/[^/]*/case\.json$|\1|p' | sort -u
}
