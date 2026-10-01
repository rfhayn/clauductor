# change-guard.sh: sourced by pr-merge-guard.sh for the rules that read a change record at the PR's
# head (rules 9, 10 and 11), the squash body (rule 12) and the eval receipts (rule 13). Each function
# prints the reason to block, or nothing; none exits, so the guard alone decides (its block() is the
# only exit 2).
#
# Needs from the caller: ROOT_HOOK, CHANGES_DIR, SPECS_DIR, ROADMAP, BRANCH_CHANGE,
# .claude/lib/change.sh sourced (open_tasks, proposal_field), and for rule 13 .claude/lib/evals.sh.

# cg_show REV PATH: a file at a commit, into a temp file whose path is printed ("" if absent).
cg_show() {
  _f=$(mktemp "${TMPDIR:-/tmp}/cg.XXXXXX") || return 1
  if git show "$1:$2" > "$_f" 2>/dev/null; then printf '%s' "$_f"; else rm -f "$_f"; fi
}

# Rule 9: a BUILD PR finishes its change. A change branch whose diff touches files other than the
# change records (CHANGES_DIR, SPECS_DIR) and the two queues a proposal also edits (ROADMAP, for its
# "proposed (#N)" note, and OWNER_QUEUE) is a build, not a proposal: every task of each change it
# touches must be ticked (D7). A proposal PR passes with every task open. Any other doc counts: a
# build that only edits docs is still a build.
cg_build_tasks() {  # cg_build_tasks BASE HEAD "IDS (change dirs)"
  _other=$(git diff --name-only "$1" "$2" 2>/dev/null | grep -v "^$CHANGES_DIR/" | grep -v "^$SPECS_DIR/" | grep -v '^openspec/' \
    | grep -vxF -e "$ROADMAP" -e "$OWNER_QUEUE" | head -1)
  [ -n "$_other" ] || return 0
  for _d in $3; do
    _t=$(cg_show "$2" "$_d/tasks.md"); [ -n "$_t" ] || continue
    _n=$(open_tasks "$_t"); rm -f "$_t"
    [ "$_n" -eq 0 ] || printf '%s has %s open task(s), and this PR builds it (it changes %s and more). A build merges only finished: tick each task done, or name the change that owns it (AGENTS.md rule 1). Check with: sh "$CLAUDUCTOR_FW"/verify-change.sh %s\n' "$_d/tasks.md" "$_n" "$_other" "${_d##*/}"
    # The project's own required sections (CHANGE_RECORD_EXTRA), read at the head.
    if [ -n "${CHANGE_RECORD_EXTRA:-}" ]; then
      _x=$(mktemp -d "${TMPDIR:-/tmp}/cgx.XXXXXX") || continue
      git archive "$2" -- "$_d" 2>/dev/null | tar -x -C "$_x" 2>/dev/null
      change_extra_missing "$_x/$_d" | sed "s|^|$_d: |"
      rm -rf "$_x"
    fi
  done
}

# Rule 10: every enforced scenario is cited by a test at the head (D4). The trace script is the one
# the gate runs; here it reads the head commit's tree.
cg_trace() {  # cg_trace HEAD
  _tr="$CLAUDUCTOR_FW/scenario-trace.sh"
  [ -f "$_tr" ] || { echo "cannot find $_tr, so scenario traceability (rule 10) cannot be checked. Restore it."; return; }
  _out=$(sh "$_tr" --check --rev "$1" 2>&1) && return 0
  printf 'scenario traceability fails at %s:\n%s\nName each MISSING ID in a test that asserts its THEN, or add a (manual: <reason>) line naming it to tasks.md, then re-run the gate.\n' \
    "$(printf %.9s "$1")" "$(printf '%s\n' "$_out" | grep -E '^(MISSING|CANNOT)' | head -10)"
}

# Rule 11: an ARCHIVED change was finished. For each directory this PR adds under
# CHANGES_DIR/archive/: no open task, a spec delta or `skip_specs: true`, the actual cost recorded in
# tasks.md, and, when the proposal says how we'll know, the outcome check queued in the roadmap.
# `openspec archive -y` archives with no delta and with open tasks, printing only a warning.
cg_archives() {  # cg_archives BASE HEAD
  for _d in $(git diff --name-only --diff-filter=A "$1" "$2" 2>/dev/null | sed -n "s|^\($CHANGES_DIR/archive/[^/]*\)/.*|\1|p" | sort -u); do
    _id=$(basename "$_d" | sed 's/^[0-9]\{4\}-[0-9]\{2\}-[0-9]\{2\}-//')
    _t=$(cg_show "$2" "$_d/tasks.md")
    if [ -z "$_t" ]; then echo "$_d has no tasks.md: an archived change keeps its record"; continue; fi
    _n=$(open_tasks "$_t")
    [ "$_n" -eq 0 ] || echo "$_d is archived with $_n open task(s): a change is archived only when finished (tick each, or name the change that owns it)"
    grep -Eiq '(^|[^a-z])actual cost:? (\$[0-9]|unknown \(.+\))' "$_t" \
      || echo "$_d/tasks.md does not record the change's actual cost: add under ## Progress '- <date> archived: actual cost \$X of budget \$N' (sh "$CLAUDUCTOR_FW"/change-cost.sh $_id), or 'actual cost unknown (<why>)'"
    rm -f "$_t"
    if ! git ls-tree -r --name-only "$2" -- "$_d/specs/" 2>/dev/null | grep -q '/spec\.md$'; then
      _y=$(cg_show "$2" "$_d/.openspec.yaml")
      if [ -z "$_y" ] || ! grep -Eq '^skip_specs:[[:space:]]*true[[:space:]]*$' "$_y"; then
        echo "$_d is archived with no spec delta and no 'skip_specs: true' in its .openspec.yaml: a change that changes no behaviour says so"
      fi
      rm -f "$_y"
    fi
    _p=$(cg_show "$2" "$_d/proposal.md")
    if [ -n "$_p" ] && grep -Eq "^## How we.ll know" "$_p"; then
      _r=$(cg_show "$2" "$ROADMAP")
      if [ -z "$_r" ] || ! grep -q "check-outcome-$_id" "$_r"; then
        echo "$_d says how we'll know, but $ROADMAP has no 'ops/check-outcome-$_id' row: queue it, dated to when the proposal says to look (archive-change step 4)"
      fi
      rm -f "$_r"
    fi
    rm -f "$_p"
  done
}

# cg_body: the squash body a merge command sets: its --body/-b text, or the contents of its
# --body-file/-F file (relative to CWD). Prints it; returns 1 when there is none, 2 when it cannot be
# read (a substitution in the text, or an unreadable file).
cg_body() {  # cg_body COMMAND CWD
  _b=$(printf '%s' "$1" | awk '
    { buf = buf $0 "\n" }
    END {
      s = buf; n = length(s); q = ""; tok = ""; have = 0; nt = 0; odd = 0
      for (j = 1; j <= n; j++) {
        c = substr(s, j, 1)
        if (q == "\047") { if (c == "\047") q = ""; else tok = tok c; continue }
        if (q == "\"") {
          if (c == "\\" && j < n) { j++; tok = tok substr(s, j, 1); continue }
          if (c == "$" || c == "`") odd = 1
          if (c == "\"") q = ""; else tok = tok c; continue
        }
        if (c == "\047" || c == "\"") { q = c; have = 1; continue }
        if (c == " " || c == "\t" || c == "\n") { if (have) { t[++nt] = tok; oddt[nt] = odd; tok = ""; have = 0; odd = 0 }; continue }
        if (c == "$" || c == "`") odd = 1
        tok = tok c; have = 1
      }
      if (have) { t[++nt] = tok; oddt[nt] = odd }
      for (k = 1; k <= nt; k++) {
        x = t[k]
        if ((x == "--body" || x == "-b" || x == "--body-file" || x == "-F") && k < nt) { kind = x; v = t[k + 1]; vo = oddt[k + 1]; found = 1 }
        else if (x ~ /^--body=/) { kind = "--body"; v = substr(x, 8); vo = oddt[k]; found = 1 }
        else if (x ~ /^--body-file=/) { kind = "--body-file"; v = substr(x, 13); vo = oddt[k]; found = 1 }
      }
      if (!found) exit 1
      if (vo) exit 2
      printf "%s\n%s", kind, v
    }')
  _rc=$?
  [ "$_rc" -eq 0 ] || return "$_rc"
  _kind=$(printf '%s\n' "$_b" | head -1); _val=$(printf '%s\n' "$_b" | sed '1d')
  case "$_kind" in
    --body-file|-F)
      case "$_val" in /*) _f=$_val ;; *) _f="${2:-.}/$_val" ;; esac
      [ -r "$_f" ] || return 2
      cat "$_f" ;;
    *) printf '%s\n' "$_val" ;;
  esac
}

# Rule 12: the squash commit carries the provenance trailers (model-roles.json provenance).
cg_trailers() {  # cg_trailers COMMAND CWD SESSION_ID ROLES_JSON
  _body=$(cg_body "$1" "$2"); _rc=$?
  _want="Change: <change id, or none>
Agent-Role: <the role that wrote it: $(jq -r '[.roles | keys[]] | join(", ")' "$4" 2>/dev/null)>
Model: <the model that wrote it>
Session: ${3:-<the id of this session>}"
  case "$_rc" in
    1) printf 'this squash merge sets no --body, so its commit would carry no provenance trailers. Write the body to a file ending in:\n%s\nthen merge with: gh pr merge <n> --squash --delete-branch --body-file <file>\n' "$_want"; return ;;
    2) printf 'cannot read this merge'"'"'s body (a substitution in --body, or an unreadable --body-file), so its provenance trailers cannot be checked. Write the body to a file and pass --body-file <file>. It must end in:\n%s\n' "$_want"; return ;;
  esac
  _miss=""
  for _k in Change Agent-Role Model Session; do
    printf '%s\n' "$_body" | grep -Eq "^$_k: [^[:space:]]" || _miss="$_miss $_k:"
  done
  if [ -n "$_miss" ]; then printf 'the squash body lacks the provenance trailer(s)%s. End it with:\n%s\n' "$_miss" "$_want"; return; fi
  _role=$(printf '%s\n' "$_body" | sed -n 's/^Agent-Role: *//p' | tail -1 | tr -d '[:space:]')
  jq -e --arg r "$_role" '.roles | has($r)' "$4" >/dev/null 2>&1 || { echo "Agent-Role: '$_role' is not a role in .claude/model-roles.json ($(jq -r '[.roles | keys[]] | join(", ")' "$4" 2>/dev/null))"; return; }
  if [ -n "$3" ]; then
    _s=$(printf '%s\n' "$_body" | sed -n 's/^Session: *//p' | tail -1 | tr -d '[:space:]')
    [ "$_s" = "$3" ] || echo "Session: '$_s' is not this session ($3). The trailer says which session merged; write 'Session: $3'"
  fi
}

# Rule 13: a model choice rests on evidence (OPS-10). A PR that CHANGES what a role runs on carries
# a passing eval receipt for that role, run at the head's hashes. What counts as a change, read from
# the PR's own diff:
#   - model-roles.json: a role whose model, effort or tier variants differ from the base's (its
#     recorded evidence, `eval`, is not a choice, so recording a receipt is not a change);
#   - .claude/agents/<a>.md modified (present at base and head): the role .agents maps <a> to;
#   - anything under .claude/workflows/ that existed at base: every role with a suite, since the
#     workflows frame each agent's prompt and restate its model.
# No model-roles.json at the base means the PR adopts the model rather than changing it, and an
# added or deleted agent has nothing to compare: neither is policed. The guard requires a receipt
# only for a role that has a suite (.claude/evals/<role>/cases/); for any other it says so.
cg_eval_roles() {  # cg_eval_roles BASE HEAD: the roles this PR changes, one per line
  _mr=.claude/model-roles.json
  git cat-file -e "$1:$_mr" 2>/dev/null || return 0
  _files=$(git diff --name-only "$1" "$2" 2>/dev/null)
  {
    if printf '%s\n' "$_files" | grep -qxF "$_mr"; then
      _b=$(git show "$1:$_mr" 2>/dev/null | jq -c '.roles | map_values({model, effort, tiers})' 2>/dev/null)
      [ -n "$_b" ] || _b='{}'
      git show "$2:$_mr" 2>/dev/null | jq -r --argjson b "$_b" \
        '.roles | to_entries[] | select(($b[.key] // null) != {model: .value.model, effort: .value.effort, tiers: .value.tiers}) | .key' 2>/dev/null
    fi
    for _a in $(printf '%s\n' "$_files" | sed -n 's|^\.claude/agents/\([^/]*\)\.md$|\1|p'); do
      git cat-file -e "$1:.claude/agents/$_a.md" 2>/dev/null && git cat-file -e "$2:.claude/agents/$_a.md" 2>/dev/null || continue
      git show "$2:$_mr" 2>/dev/null | jq -r --arg a "$_a" '.agents[$a] // empty' 2>/dev/null
    done
    if printf '%s\n' "$_files" | grep -q '^\.claude/workflows/' && [ -n "$(git ls-tree -r --name-only "$1" -- ".claude/workflows/" 2>/dev/null)" ]; then
      evals_suite_roles_at "$2"
    fi
  } | grep . | sort -u
}

cg_eval_receipt() {  # cg_eval_receipt HEAD ROLE: why the head holds no passing receipt for ROLE, or nothing
  _mrf=$(cg_show "$1" .claude/model-roles.json)
  [ -n "$_mrf" ] || { echo "cannot read .claude/model-roles.json at the head, so role $2's eval receipt cannot be checked"; return; }
  _m=$(jq -r --arg r "$2" '.roles[$r].model // empty' "$_mrf"); _e=$(jq -r --arg r "$2" '.roles[$r].effort // empty' "$_mrf")
  _ag=$(jq -r --arg r "$2" '(.agents // {}) | to_entries[] | select(.value == $r) | .key' "$_mrf" | head -1)
  [ -n "$_ag" ] || _ag=$2
  _want="$(evals_roles_hash_at "$1" .claude/model-roles.json) $(evals_blob_at "$1" ".claude/agents/$_ag.md") $(evals_tree_hash_at "$1" ".claude/workflows")"
  _seen=""
  for _p in $(git ls-tree --name-only "$1" -- .claude/evals/receipts/ 2>/dev/null | grep '\.json$'); do
    _f=$(cg_show "$1" "$_p"); [ -n "$_f" ] || continue
    if [ "$(jq -r '.role // empty' "$_f" 2>/dev/null)" != "$2" ]; then rm -f "$_f"; continue; fi
    _why=$(evals_verdict "$_f" "$2" "$_m" "$_e" "$_mrf" | tr '\n' ';' | sed 's/;$//; s/;/; /g')
    _got=$(jq -r '"\(.hashes.model_roles // "-") \(.hashes.agent // "-") \(.hashes.workflows // "-")"' "$_f" 2>/dev/null)
    [ "$_got" = "$_want" ] || _why="${_why}${_why:+; }it ran at other hashes than the head's (model-roles, agent, workflows: has [$_got], head [$_want]), so something it measured changed after it ran"
    rm -f "$_f"
    if [ -z "$_why" ]; then rm -f "$_mrf"; return 0; fi
    _seen="$_seen
  - $_p: $_why"
  done
  rm -f "$_mrf"
  printf 'role %s (%s/%s) is changed by this PR, and the head holds no passing eval receipt for it.%s\nRun: sh .claude/evals/run.sh --role %s --model %s --effort %s on the final head (the receipt names its hashes), commit the receipt, record it as model-roles.json .roles.%s.eval, then re-run the gate.\n' \
    "$2" "$_m" "$_e" "${_seen:- No receipt for role $2 under .claude/evals/receipts/.}" "$2" "$_m" "$_e" "$2"
}
