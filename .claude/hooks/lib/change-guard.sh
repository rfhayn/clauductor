# change-guard.sh: sourced by pr-merge-guard.sh for the rules that read a change record at the PR's
# head (rules 9, 10 and 11), the squash body (rule 12) and the eval receipts (rule 13). Each function
# prints the reason to block, or nothing; none exits, so the guard alone decides (its block() is the
# only exit 2).
#
# Needs from the caller: ROOT_HOOK, CHANGES_DIR, SPECS_DIR, ROADMAP, BRANCH_CHANGE,
# .claude/lib/change.sh sourced (open_tasks, proposal_field), and for rule 13 .claude/lib/evals.sh.

# cg_legacy ID: 0 when CHANGES_LEGACY grandfathers change ID (lib/records.sh change_is_legacy, which
# the guard sources when it exists). Without the library nothing is grandfathered: the stricter
# reading, never the silent one.
cg_legacy() {
  [ -n "${CHANGES_LEGACY:-}" ] && command -v change_is_legacy >/dev/null 2>&1 && change_is_legacy "$1"
}

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
    [ "$_n" -eq 0 ] || printf '%s has %s open task(s), and this PR builds it (it changes %s and more). A build merges only finished: tick each task done, or name the change that owns it (AGENTS.md rule 1). Check with: sh .claude/verify-change.sh %s\n' "$_d/tasks.md" "$_n" "$_other" "${_d##*/}"
    # The project's own required sections (CHANGE_RECORD_EXTRA), read at the head; not asked of a
    # grandfathered change (CHANGES_LEGACY), which keeps the format it was approved in.
    if [ -n "${CHANGE_RECORD_EXTRA:-}" ] && ! cg_legacy "${_d##*/}"; then
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
  _tr="$ROOT_HOOK/.claude/scenario-trace.sh"
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
    # A grandfathered change (CHANGES_LEGACY) is held to finishing, which is format-free, but not to
    # the records this format added since it was proposed: cost, delta or skip_specs, outcome row.
    if cg_legacy "$_id"; then rm -f "$_t"; continue; fi
    grep -Eiq '(^|[^a-z])actual cost:? (\$[0-9]|unknown \(.+\))' "$_t" \
      || echo "$_d/tasks.md does not record the change's actual cost: add under ## Progress '- <date> archived: actual cost \$X of budget \$N' (sh .claude/change-cost.sh $_id), or 'actual cost unknown (<why>)'"
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

# cg_field COMMAND CWD body|subject: what a merge command sets for the squash commit's body or
# subject, read the way gh reads its flags. Prints it; returns 1 when the command sets none, 2 when
# it cannot be read (a substitution in the text, a file that cannot be read or is stdin, an API
# merge whose fields come from --input). Forms read:
#   gh pr merge   body:    --body T, --body=T, --body-file F, --body-file=F, and the short flags
#                          -b/-F with the value next, glued (-bT, -FF) or after = (-b=T), also
#                          inside a cluster of boolean shorthands (-sdbT)
#                 subject: --subject T, --subject=T, -t T, -tT, -t=T, -sdtT
#   gh api …/merge  body: -f|-F|--field|--raw-field commit_message=T (and glued -fcommit_message=T,
#                         --field=commit_message=T); -F commit_message=@F reads F
#                 subject: the same with commit_title
# The last occurrence wins, as it does for gh.
cg_body() { cg_field "$1" "$2" body; }        # cg_body COMMAND CWD (rule 12; premise-check)
cg_subject() { cg_field "$1" "$2" subject; }  # cg_subject COMMAND CWD (premise-check)
cg_field() {
  _b=$(printf '%s' "$1" | awk -v want="$3" '
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
        # Unquoted, a backslash makes the next character literal (Fixes\ \#239 is one word), and a
        # backslash before a newline joins the lines.
        if (c == "\\" && j < n) { j++; if (substr(s, j, 1) != "\n") { tok = tok substr(s, j, 1); have = 1 }; continue }
        if (c == " " || c == "\t" || c == "\n") { if (have) { t[++nt] = tok; oddt[nt] = odd; tok = ""; have = 0; odd = 0 }; continue }
        if (c == "$" || c == "`") odd = 1
        tok = tok c; have = 1
      }
      if (have) { t[++nt] = tok; oddt[nt] = odd }
      mode = "pr"
      for (k = 1; k < nt; k++) if (t[k] == "gh") { if (t[k + 1] == "api") mode = "api"; break }
      long = (want == "body") ? "--body" : "--subject"
      key = (want == "body") ? "commit_message" : "commit_title"
      for (k = 1; k <= nt; k++) {
        x = t[k]
        if (mode == "api") {
          kv = ""; ko = 0; isF = 0; got = 0
          if ((x == "-f" || x == "-F" || x == "--field" || x == "--raw-field") && k < nt) { kv = t[k + 1]; ko = oddt[k + 1]; isF = (x == "-F" || x == "--field"); got = 1; k++ }
          else if (x ~ /^--(raw-)?field=/) { kv = substr(x, index(x, "=") + 1); ko = oddt[k]; isF = (x ~ /^--field=/); got = 1 }
          else if (x ~ /^-[fF]./) { kv = substr(x, 3); sub(/^=/, "", kv); ko = oddt[k]; isF = (substr(x, 2, 1) == "F"); got = 1 }
          else if (x == "--input" || x ~ /^--input=/) { bad = 1 }
          if (got && index(kv, key "=") == 1) {
            v = substr(kv, length(key) + 2); vo = ko; found = 1; kind = "text"
            if (isF && substr(v, 1, 1) == "@") { kind = "file"; v = substr(v, 2) }
          }
          continue
        }
        if (x == "--") break   # the end of the flags: what follows is an argument, never a flag
        if (x ~ /^--/) {
          # Every value-taking long flag of gh pr merge consumes its value, whichever field is
          # wanted: a value that starts with - is text, never another flag.
          name = x; hasv = 0
          if (index(x, "=") > 0) { name = substr(x, 1, index(x, "=") - 1); val = substr(x, index(x, "=") + 1); vov = oddt[k]; hasv = 1 }
          if (name !~ /^--(author-email|body|body-file|match-head-commit|repo|subject)$/) continue
          if (!hasv) { if (k < nt) { val = t[k + 1]; vov = oddt[k + 1]; k++ } else continue }
          if (want == "body" && name == "--body") { kind = "text"; v = val; vo = vov; found = 1 }
          if (want == "body" && name == "--body-file") { kind = "file"; v = val; vo = vov; found = 1 }
          if (want == "subject" && name == "--subject") { kind = "text"; v = val; vo = vov; found = 1 }
          continue
        }
        if (x ~ /^-[^-]/) {
          # A cluster of shorthands, as pflag reads it: boolean ones (d s m r) go by; the first that
          # takes a value (b F t A R) takes the rest of the token, or the next token.
          for (c = 2; c <= length(x); c++) {
            ch = substr(x, c, 1)
            if (ch ~ /[dsmr]/) continue
            if (ch ~ /[bFtAR]/) {
              rest = substr(x, c + 1); sub(/^=/, "", rest)
              if (rest != "") { val = rest; vov = oddt[k] } else if (k < nt) { val = t[k + 1]; vov = oddt[k + 1]; k++ } else break
              if (want == "body" && ch == "b") { kind = "text"; v = val; vo = vov; found = 1 }
              if (want == "body" && ch == "F") { kind = "file"; v = val; vo = vov; found = 1 }
              if (want == "subject" && ch == "t") { kind = "text"; v = val; vo = vov; found = 1 }
            }
            break
          }
        }
      }
      if (bad) exit 2
      if (!found) exit 1
      if (vo) exit 2
      printf "%s\n%s", kind, v
    }')
  _rc=$?
  [ "$_rc" -eq 0 ] || return "$_rc"
  _kind=$(printf '%s\n' "$_b" | head -1); _val=$(printf '%s\n' "$_b" | sed '1d')
  case "$_kind" in
    file)
      [ "$_val" != - ] || return 2   # stdin: not readable from here
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

# Rule 13: a model choice rests on evidence (OPS-10). A PR that CHANGES what a role IS carries a
# passing eval receipt for that role, run at the head's hashes. What counts as a change (OPS-16),
# compared between base and head by content hash, never by "the file was touched":
#   - the role's model, effort or tier variants in model-roles.json (its recorded evidence, `eval`,
#     is not a choice, so recording a receipt is not a change);
#   - any of its trigger inputs (model-roles.json .evals.triggers.<role>; lib/evals.sh): for the
#     reviewer, its agent file and the marked review-prompt section of build-change.js, so a
#     workflow edit outside the markers changes nothing the reviewer is;
#   - its declared trigger list itself, so a PR cannot drop an input and a later one edit it free;
#   - a file holding a declared section whose markers are gone at the head and whose bytes differ:
#     what changed inside cannot be told, so it counts (and cg_eval_receipt then blocks: fail closed).
# No model-roles.json at the base means the PR adopts the model rather than changing it. The guard
# requires a receipt only for a role that has a suite (.claude/evals/<role>/cases/); for any other
# it says so.
cg_eval_roles() {  # cg_eval_roles BASE HEAD: the roles this PR changes, one per line
  _mr=.claude/model-roles.json
  git cat-file -e "$1:$_mr" 2>/dev/null || return 0
  {
    _b=$(git show "$1:$_mr" 2>/dev/null | jq -c '.roles | map_values({model, effort, tiers})' 2>/dev/null)
    [ -n "$_b" ] || _b='{}'
    git show "$2:$_mr" 2>/dev/null | jq -r --argjson b "$_b" \
      '.roles | to_entries[] | select(($b[.key] // null) != {model: .value.model, effort: .value.effort, tiers: .value.tiers}) | .key' 2>/dev/null
    for _r in $( { evals_suite_roles_at "$2"
                   git show "$1:$_mr" 2>/dev/null | jq -r '(.evals.triggers // {}) | keys[] | select(startswith("_") | not)' 2>/dev/null
                   git show "$2:$_mr" 2>/dev/null | jq -r '(.evals.triggers // {}) | keys[] | select(startswith("_") | not)' 2>/dev/null
                 } | sort -u); do
      _tb=$(evals_triggers_at "$1" "$_r"); _th=$(evals_triggers_at "$2" "$_r")
      if [ "$_tb" != "$_th" ]; then echo "$_r"; continue; fi
      for _i in $_th; do
        _hh=$(evals_input_hash_at "$2" "$_i")
        if [ "$(evals_input_hash_at "$1" "$_i")" != "$_hh" ]; then echo "$_r"; break; fi
        case "$_i" in *'#'*)
          if [ "$_hh" = markers-missing ] && [ "$(evals_blob_at "$1" "${_i%%#*}")" != "$(evals_blob_at "$2" "${_i%%#*}")" ]; then echo "$_r"; break; fi ;;
        esac
      done
    done
  } | grep . | sort -u
}

cg_eval_receipt() {  # cg_eval_receipt HEAD ROLE: why the head holds no passing receipt for ROLE, or nothing
  _mrf=$(cg_show "$1" .claude/model-roles.json)
  [ -n "$_mrf" ] || { echo "cannot read .claude/model-roles.json at the head, so role $2's eval receipt cannot be checked"; return; }
  _m=$(jq -r --arg r "$2" '.roles[$r].model // empty' "$_mrf"); _e=$(jq -r --arg r "$2" '.roles[$r].effort // empty' "$_mrf")
  # The head's own trigger inputs and their hashes, sorted by input: what a receipt must name.
  _want=$(for _i in $(evals_triggers "$2" "$_mrf"); do printf '%s %s\n' "$_i" "$(evals_input_hash_at "$1" "$_i")"; done)
  _mm=$(printf '%s\n' "$_want" | sed -n 's/ markers-missing$//p' | tr '\n' ' ')
  if [ -n "$_mm" ]; then
    rm -f "$_mrf"
    printf 'role %s is changed by this PR, and its trigger input(s) %scannot be read at the head: the file has no `// <MARKER>` ... `// </MARKER>` section (each marker once, in order), so what changed inside it cannot be told. Restore the markers around the section, then run the eval.\n' "$2" "$_mm"
    return
  fi
  _wrole=$(evals_role_hash "$2" "$_mrf")
  _seen=""
  for _p in $(git ls-tree --name-only "$1" -- .claude/evals/receipts/ 2>/dev/null | grep '\.json$'); do
    _f=$(cg_show "$1" "$_p"); [ -n "$_f" ] || continue
    if [ "$(jq -r '.role // empty' "$_f" 2>/dev/null)" != "$2" ]; then rm -f "$_f"; continue; fi
    _why=$(evals_verdict "$_f" "$2" "$_m" "$_e" "$_mrf" | tr '\n' ';' | sed 's/;$//; s/;/; /g')
    _got=$(jq -r '(.hashes.triggers // {}) | to_entries | sort_by(.key)[] | "\(.key) \(.value)"' "$_f" 2>/dev/null)
    _grole=$(jq -r '.hashes.role // "-"' "$_f" 2>/dev/null)
    # The agent it evaluated must be one of the head's trigger inputs, at the head's content: a
    # receipt naming reviewer.md's hash proves nothing if another agent file was the one run.
    _gaf=$(jq -r '.hashes.agent_file // "-"' "$_f" 2>/dev/null); _gab=$(jq -r '.hashes.agent // "-"' "$_f" 2>/dev/null)
    if ! printf '%s\n' "$_want" | cut -d' ' -f1 | grep -qxF "$_gaf"; then
      _why="${_why}${_why:+; }the agent it evaluated ($_gaf) is not one of role $2's trigger inputs at the head"
    elif [ "$_gab" != "$(evals_input_hash_at "$1" "$_gaf")" ]; then
      _why="${_why}${_why:+; }the agent file it evaluated ($_gaf, blob $_gab) is not the head's"
    fi
    # The suite it was scored on must be the head's: an easier suite scores higher.
    _gsh=$(jq -r '.suite.hash // "-"' "$_f" 2>/dev/null)
    [ "$_gsh" = "$(evals_tree_hash_at "$1" ".claude/evals/$2/cases")" ] \
      || _why="${_why}${_why:+; }it was scored on another suite than the head's .claude/evals/$2/cases (suite hash $_gsh)"
    # ...and under the head's suite AGENTS.md. A receipt from before OPS-16 round 3 does not name it
    # (suite.agents_md absent) and is accepted on its cases hash alone: cg_eval_policy blocks every
    # PR that changes that AGENTS.md, so it cannot have moved under such a receipt without the owner.
    _gam=$(jq -r '.suite.agents_md // empty' "$_f" 2>/dev/null)
    [ -z "$_gam" ] || [ "$_gam" = "$(evals_blob_at "$1" ".claude/evals/$2/AGENTS.md")" ] \
      || _why="${_why}${_why:+; }it ran under another .claude/evals/$2/AGENTS.md than the head's"
    [ "$_grole" = "$_wrole" ] || _why="${_why}${_why:+; }it ran on another model, effort or tier variant of role $2 than the head's (role hash $_grole, head $_wrole)"
    [ "$_got" = "$_want" ] || _why="${_why}${_why:+; }it ran at other trigger inputs than the head's (has [$(printf '%s' "$_got" | tr '\n' ',')], head [$(printf '%s' "$_want" | tr '\n' ',')]), so something it measured changed after it ran"
    rm -f "$_f"
    if [ -z "$_why" ]; then rm -f "$_mrf"; return 0; fi
    _seen="$_seen
  - $_p: $_why"
  done
  rm -f "$_mrf"
  printf 'role %s (%s/%s) is changed by this PR, and the head holds no passing eval receipt for it.%s\nRun: sh .claude/evals/run.sh --role %s --model %s --effort %s on the final head (the receipt names its hashes), commit the receipt, record it as model-roles.json .roles.%s.eval, then re-run the gate.\n' \
    "$2" "$_m" "$_e" "${_seen:- No receipt for role $2 under .claude/evals/receipts/.}" "$2" "$_m" "$_e" "$2"
}

# SCOPE. Rule 13 guards against ACCIDENTAL drift: a PR that changes what the reviewer is without
# anyone re-measuring it. A receipt is self-reported (run.sh writes it on the author's machine and
# nothing re-runs it), so a deliberate forger who edits a receipt, or hides a change from these
# string and hash checks, is out of scope. Deliberate bypasses are what review and the owner are for.
#
# Rule 13's own controls (OPS-16). Each would let a later PR weaken the evidence without a receipt
# ever failing, so a receipt cannot excuse it: it is the owner's decision. No owner-approval marker
# exists for an ops PR, so these BLOCK OUTRIGHT here; the owner merges such a PR themselves.
#   - a role's trigger inputs narrowed: any input the base declares (or defaults to) that the head
#     does not, including a declaration emptied or one replacing the broad default;
#   - a suite at the base that the head lacks (deleting it would turn rule 13 into an advisory), a
#     base case removed or changed in any file (case.json, before/, after/), or the suite's
#     AGENTS.md changed: a suite can be weakened one case, or one hint, at a time;
#   - .evals.thresholds weakened: recall or severity_accuracy lowered, fp_rate raised, or removed;
#   - build-change.js's pick() changed. It chooses the reviewer's model and effort at run time,
#     outside every hashed section, and no receipt names it, so a receipt cannot cover a change
#     to it. checks/build-change.sh executes it and holds pick('reviewer') to model-roles.json at
#     every Risk tier; this makes ANY edit to it the owner's call, so a change the contract's
#     inputs miss (one that keys on the change id, the date, an environment variable) cannot land
#     through Claude either.
# An input is covered (not narrowed) when the head declares it, or declares a dir/ holding it: a
# role whose declaration is deleted falls back to the broader default, which is not a narrowing.
cg_eval_policy() {  # cg_eval_policy BASE HEAD: why this PR weakens rule 13 itself, one per line, or nothing
  _mr=.claude/model-roles.json
  git cat-file -e "$1:$_mr" 2>/dev/null || return 0
  _hs=$(evals_suite_roles_at "$2")
  for _r in $(evals_suite_roles_at "$1"); do
    if ! printf '%s\n' "$_hs" | grep -qxF "$_r"; then
      echo "it deletes role $_r's eval suite (.claude/evals/$_r/cases/), which turns rule 13 for $_r into an advisory"; continue
    fi
    # Any change to a base case's directory: its expectations, but equally its brief, title, tasks,
    # deletions or before/after trees (a hint in a brief makes a case easier). New cases are fine.
    for _c in $(git ls-tree --name-only "$1" -- ".claude/evals/$_r/cases/" 2>/dev/null); do
      git cat-file -e "$1:$_c/case.json" 2>/dev/null || continue
      if ! git cat-file -e "$2:$_c/case.json" 2>/dev/null; then echo "it removes eval case ${_c##*/} from role $_r's suite"; continue; fi
      [ "$(evals_tree_hash_at "$1" "$_c")" = "$(evals_tree_hash_at "$2" "$_c")" ] \
        || echo "it changes eval case ${_c##*/} of role $_r (its case.json, before/ or after/)"
    done
    # The suite's AGENTS.md is every case's project rules, so it is part of what the cases ask.
    _ab=$(evals_blob_at "$1" ".claude/evals/$_r/AGENTS.md")
    [ "$_ab" = none ] || [ "$_ab" = "$(evals_blob_at "$2" ".claude/evals/$_r/AGENTS.md")" ] \
      || echo "it changes or removes .claude/evals/$_r/AGENTS.md, which every case of role $_r runs under"
  done
  for _r in $( { evals_suite_roles_at "$1"
                 git show "$1:$_mr" 2>/dev/null | jq -r '(.evals.triggers // {}) | keys[] | select(startswith("_") | not)' 2>/dev/null
               } | sort -u); do
    _th=$(evals_triggers_at "$2" "$_r")
    _gone=$(evals_triggers_at "$1" "$_r" | while IFS= read -r _i; do
      printf '%s\n' "$_th" | grep -qxF "$_i" && continue
      _cov=""
      for _d in $(printf '%s\n' "$_th" | grep '/$'); do case "${_i%%#*}" in ("$_d"*) _cov=1 ;; esac; done
      [ -n "$_cov" ] || printf '%s ' "$_i"
    done)
    [ -z "$_gone" ] || echo "it narrows role $_r's eval triggers: ${_gone}would no longer need a receipt to change"
  done
  _pk() { git show "$1:.claude/workflows/build-change.js" 2>/dev/null | awk '/^const pick = \(role\) => \{$/ { p = 1 } p { print } p && /^}$/ { exit }'; }
  _pb=$(_pk "$1")
  if [ -n "$_pb" ] && [ "$_pb" != "$(_pk "$2")" ]; then
    echo "it changes (or removes) build-change.js's pick(), which chooses the reviewer's model at run time outside the hashed sections"
  fi
  _tb=$(git show "$1:$_mr" 2>/dev/null | jq -c '.evals.thresholds // {}' 2>/dev/null); [ -n "$_tb" ] || _tb='{}'
  git show "$2:$_mr" 2>/dev/null | jq -r --argjson b "$_tb" '(.evals.thresholds // {}) as $h
    | [ ("recall", "severity_accuracy") as $k | select($b[$k] != null and (($h[$k] | type) != "number" or $h[$k] < $b[$k])) | "\($k) \($b[$k]) -> \($h[$k])" ]
    + [ select($b.fp_rate != null and (($h.fp_rate | type) != "number" or $h.fp_rate > $b.fp_rate)) | "fp_rate \($b.fp_rate) -> \($h.fp_rate)" ]
    | .[] | "it weakens .evals.thresholds: \(.)"' 2>/dev/null
}
