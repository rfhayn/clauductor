# evals.sh: sourced by checks/model-roles.sh and by pr-merge-guard.sh (rule 13, through
# hooks/lib/change-guard.sh). What makes an eval receipt count as evidence for a role's model and
# effort (OPS-10). Receipts are written by .claude/evals/run.sh.
#
# THE HASHES ARE GIT BLOB IDS (`git hash-object`), so a hash computed from the working tree when
# the eval ran equals one read from a commit's objects when the guard runs, with no sha256 tool to
# differ between macOS and Linux. The three working-tree functions below are duplicated, on
# purpose, in .claude/evals/run.sh: the evals directory is the project's (the plugin scaffolds it),
# so the runner cannot source this framework library. checks/evals.sh asserts the copies match.
#
# The model-roles hash covers what the file CHOOSES (each role's model, effort and tier variants),
# not the whole file: the evidence is recorded in the same file (roles.<r>.eval), and a hash of
# the whole file would be invalidated by writing down the result it certifies.
#
# Needs: jq, git.

# ── Working tree (identical in .claude/evals/run.sh) ─────────────────────────────────────────
evals_roles_hash() {  # evals_roles_hash MODEL_ROLES_JSON_FILE
  jq -cS '.roles | map_values({model: .model, effort: .effort, tiers: .tiers})' "$1" 2>/dev/null | git hash-object --stdin
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
# ── end of the duplicated functions ───────────────────────────────────────────────────────────

# The same three, read from a commit instead of the working tree.
evals_roles_hash_at() {  # evals_roles_hash_at REV PATH
  git show "$1:$2" 2>/dev/null | jq -cS '.roles | map_values({model: .model, effort: .effort, tiers: .tiers})' 2>/dev/null | git hash-object --stdin
}
evals_blob_at() {  # evals_blob_at REV PATH
  git rev-parse -q --verify "$1:$2" 2>/dev/null || echo none
}
evals_tree_hash_at() {  # evals_tree_hash_at REV DIR (repo-relative, no trailing slash)
  git ls-tree -r "$1" -- "$2/" 2>/dev/null | awk -F'\t' '{ split($1, m, " "); print m[3] " " $2 }' | grep -v '/\.DS_Store$' \
    | LC_ALL=C sort -k2 | git hash-object --stdin
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
