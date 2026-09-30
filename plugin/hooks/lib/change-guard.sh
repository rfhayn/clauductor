# change-guard.sh: sourced by pr-merge-guard.sh for the rules that read a change record at the PR's
# head (rules 9, 10 and 11) and the squash body (rule 12). Each function prints the reason to block,
# or nothing; none exits, so the guard alone decides (its block() is the only exit 2).
#
# Needs from the caller: ROOT_HOOK, CHANGES_DIR, SPECS_DIR, ROADMAP, BRANCH_CHANGE, and
# .claude/lib/change.sh sourced (open_tasks, proposal_field).

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
