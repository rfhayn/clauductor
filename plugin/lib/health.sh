# health.sh: sourced (never run) by the GitHub Actions health lines in .claude/health/. The logic
# lives here, in a framework file that `clauductor update` refreshes, and each health line is a
# thin caller: .claude/health/ is project-owned (created once, never overwritten), so logic kept
# there would never receive a fix. POSIX sh, git, jq and awk only.
#
#   health_days_since YYYY-MM-DD   whole days from that date to today (HEALTH_TODAY overrides
#                                  today, for the checks); empty when the date is not one
#   health_on_trigger FILE KEY     exit 0 when the workflow FILE's top-level `on:` carries KEY
#   health_crons FILE              the workflow's cron expressions, `; `-joined, as written
#   health_push_main               the push-main health line (one line per push workflow)
#   health_scheduled [--list]      the scheduled-workflows health line (one line per workflow)
#
# Both health lines report FACTS and name their SUBJECT (the run's commit, date and URL); neither
# draws a conclusion the facts do not carry, and neither ever degrades to silence: a lookup that
# fails says CANNOT CHECK, by name. Every line starts with its verdict (.claude/health/README.md).
# Upstreamed from a project that ran both for months (P2.2); its review history is why each branch
# below is shaped the way it is, and the comments keep the reasons.

# health_days_since DATE: days from DATE (YYYY-MM-DD) to today, by the civil calendar.
health_days_since() {
  _hd_today=${HEALTH_TODAY:-$(date +%Y-%m-%d)}
  printf '%s %s\n' "$1" "$_hd_today" | awk '
    function days(s,   y, m, d) {
      if (s !~ /^[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]$/) return -1
      y = substr(s, 1, 4) + 0; m = substr(s, 6, 2) + 0; d = substr(s, 9, 2) + 0
      if (m <= 2) { y--; m += 12 }
      return 365 * y + int(y / 4) - int(y / 100) + int(y / 400) + int((153 * (m - 3) + 2) / 5) + d
    }
    { a = days($1); b = days($2); if (a >= 0 && b >= 0) print b - a }'
}

# health_on_trigger FILE KEY: is KEY a trigger under the workflow's top-level `on:`?
#
# THREE SPELLINGS of `on:`: YAML 1.1 reads a bare `on` as the boolean true, so linters push authors
# to `"on":` or `'on':`, and a workflow spelled that way is ordinary. And the inline flow form,
# `on: {schedule: [...]}` or `on: [push, pull_request]`, which never produces an indented key line.
# Anchored so the key in a comment, a job name, a step or an `if:` cannot match: the block runs to
# the next column-0 key, and the key must be indented and start its line. A trailing comment on
# the `on:` line is not a trigger (`on: workflow_dispatch  # no schedule here` once matched).
health_on_trigger() {
  awk -v key="$2" '
    /^["'"'"']?on["'"'"']?[[:space:]]*:/ {
      inon = 1; ind = -1; rest = $0
      sub(/^[^:]*:/, "", rest); sub(/#.*$/, "", rest)
      if (rest ~ "[{[].*" key) found = 1
      # `on: push`, one event as a scalar (a form the upstream version did not read).
      if (rest ~ "^[[:space:]]*" key "[[:space:]]*$") found = 1
      next
    }
    /^[^[:space:]#]/ { inon = 0 }
    # Only the block'"'"'s FIRST level is an event: an input named `push` under workflow_call is not.
    # The first indented line that is not a comment sets that level.
    inon && /^[[:space:]]+[^[:space:]#]/ {
      match($0, /^[[:space:]]+/); if (ind < 0) ind = RLENGTH
      if (RLENGTH != ind) next
      if ($0 ~ "^[[:space:]]+" key "[[:space:]]*:") found = 1
      # The block-list form: `on:` then `  - push`.
      if ($0 ~ "^[[:space:]]+-[[:space:]]*" key "[[:space:]]*(#.*)?$") found = 1
    }
    END { exit !found }
  ' "$1"
}

# health_crons FILE: the cron expressions, shown rather than CLASSIFIED. A staleness verdict derived
# from the cron was wrong twice in review (the inline form has no `cron:` line to anchor on; a
# monthly cadence makes a 61-day window GitHub never fires, as it disables a schedule at 60). The
# cadence next to the age gives the reader everything that verdict was trying to compute.
health_crons() {
  awk '
    { line = $0; sub(/#.*$/, "", line) }
    match(line, /cron[[:space:]]*:[[:space:]]*["'"'"']?[^"'"'"',}\]]+/) {
      v = substr(line, RSTART, RLENGTH)
      sub(/^cron[[:space:]]*:[[:space:]]*["'"'"']?/, "", v)
      gsub(/[[:space:]]+$/, "", v)
      out = out (out ? "; " : "") v
    }
    END { print out }
  ' "$1"
}

# _health_workflows KEY: the basenames of the workflows whose `on:` carries KEY, one per line. The
# set comes from the workflows' OWN keys (the authority), never from a list here: a list inherits
# the omission these lines exist to detect the moment a second such workflow lands.
_health_workflows() {
  for _hw_f in "$ROOT"/.github/workflows/*.yml "$ROOT"/.github/workflows/*.yaml; do
    [ -f "$_hw_f" ] || continue
    health_on_trigger "$_hw_f" "$1" && basename "$_hw_f"
  done
}

# _health_gh OUTVAR-FILE ERR-FILE ARGS...: run gh, its JSON to one file and its stderr to another.
# stderr is kept SEPARATE: gh prints notices on perfectly successful calls, and folding them into
# the JSON makes the parse fail, turning a healthy check into a permanent CANNOT CHECK.
_health_gh() {
  _hg_out=$1 _hg_err=$2; shift 2
  (cd "$ROOT" && gh "$@") >"$_hg_out" 2>"$_hg_err"
}

# _health_tools: gh and jq, or the CANNOT CHECK line that names which is missing. jq is required
# (the template's one JSON reader); its absence is a MACHINE problem, said as such, never a
# "could not parse gh output" that sends the reader to GitHub.
_health_tools() {
  if [ "${CONTEXT_OFFLINE:-}" = 1 ]; then echo "CANNOT CHECK — offline"; return 1; fi
  command -v gh >/dev/null 2>&1 || { echo "CANNOT CHECK — gh is not installed, so $1 is UNKNOWN, not healthy"; return 1; }
  command -v jq >/dev/null 2>&1 || { echo "CANNOT CHECK — jq is not installed, so nothing can read gh's JSON here. This is a MACHINE problem, not a CI problem; $1 is UNKNOWN, not healthy"; return 1; }
}

# health_push_main: the latest `push` run on MAIN_BRANCH of every workflow whose `on:` has `push`.
#
# A post-merge run catches what no PR can (two PRs merged in sequence without a rebase; a docs commit
# that breaks a doc-asserting test), and its failure reaches no PR either: this line is its reader.
# THE SUBJECT, not just the verdict: a green run three merges back says nothing about what is on the
# main branch now, so a run that is not the current origin/<main> reads STALE, with the count of
# commits it does not cover. Facts here, judgement in the reader.
health_push_main() {
  [ -d "$ROOT/.github/workflows" ] || { echo "OK no .github/workflows (no post-merge safety net to report on)"; return 0; }
  _pm_wfs=$(_health_workflows push)
  [ -n "$_pm_wfs" ] || { echo "OK no workflow in .github/workflows/ has a push: trigger — there is no post-merge safety net to report on"; return 0; }
  _health_tools "push:$MAIN_BRANCH CI" || return 0
  _pm_tmp=$(mktemp -d "${TMPDIR:-/tmp}/health.XXXXXX" 2>/dev/null) || { echo "CANNOT CHECK — mktemp failed; push:$MAIN_BRANCH CI is UNKNOWN, not healthy"; return 0; }
  # The current tip, from git. Absent (no remote ref, git missing): say so, never claim agreement.
  _pm_head=$(git -C "$ROOT" rev-parse "origin/$MAIN_BRANCH" 2>/dev/null) || _pm_head=""
  for _pm_n in $_pm_wfs; do
    _pm_l="push:$MAIN_BRANCH $_pm_n"
    if ! _health_gh "$_pm_tmp/out" "$_pm_tmp/err" run list --workflow "$_pm_n" --event push --branch "$MAIN_BRANCH" --limit 1 \
         --json conclusion,status,createdAt,url,headSha; then
      echo "CANNOT CHECK — $_pm_l: gh failed ($(head -1 "$_pm_tmp/err")). UNKNOWN, not healthy."
      continue
    fi
    # One jq call, fixed field order, one field per line; a non-array is a parse failure.
    if ! _pm_p=$(jq -r 'if type != "array" then error("not an array") else
          (length | tostring), (.[0] // {} | .conclusion // "", .status // "", ((.createdAt // "")[0:10]), .url // "", .headSha // "") end' \
          "$_pm_tmp/out" 2>/dev/null); then
      echo "CANNOT CHECK — $_pm_l: could not parse gh output. UNKNOWN, not healthy."
      continue
    fi
    _pm_count=$(printf '%s\n' "$_pm_p" | sed -n 1p)
    if [ "$_pm_count" = 0 ]; then echo "NEVER RAN $_pm_l: no run on record yet"; continue; fi
    _pm_concl=$(printf '%s\n' "$_pm_p" | sed -n 2p); _pm_status=$(printf '%s\n' "$_pm_p" | sed -n 3p)
    _pm_when=$(printf '%s\n' "$_pm_p" | sed -n 4p); _pm_url=$(printf '%s\n' "$_pm_p" | sed -n 5p)
    _pm_sha=$(printf '%s\n' "$_pm_p" | sed -n 6p)
    _pm_age=$(health_days_since "$_pm_when")
    _pm_aged=$([ -n "$_pm_age" ] && echo "${_pm_age}d ago" || echo "$_pm_when")
    _pm_cur=""
    if [ -z "$_pm_head" ]; then
      _pm_subject="(cannot compare with origin/$MAIN_BRANCH here)"
    elif [ -z "$_pm_sha" ]; then
      _pm_subject="(the run names no commit)"
    elif [ "$_pm_head" = "$_pm_sha" ]; then
      _pm_subject="on the current origin/$MAIN_BRANCH ($(printf %.9s "$_pm_sha"))"; _pm_cur=1
    else
      _pm_behind=$(git -C "$ROOT" rev-list --count "$_pm_sha..origin/$MAIN_BRANCH" 2>/dev/null) || _pm_behind=""
      _pm_subject="on $(printf %.9s "$_pm_sha"), NOT current origin/$MAIN_BRANCH ($(printf %.9s "$_pm_head"))${_pm_behind:+ — $_pm_behind commit(s) since are uncovered}"
    fi
    case "$_pm_status" in
      completed)
        case "$_pm_concl" in
          success) echo "$([ -n "$_pm_cur" ] && echo OK || echo STALE) $_pm_l: last run PASSED $_pm_aged, $_pm_subject" ;;
          "") echo "CANNOT CHECK — $_pm_l: last run completed with NO conclusion $_pm_aged, $_pm_subject — UNKNOWN. $_pm_url" ;;
          *) echo "FAILED $_pm_l: last run $(printf '%s' "$_pm_concl" | tr '[:lower:]' '[:upper:]') $_pm_aged, $_pm_subject — nobody was told. $_pm_url" ;;
        esac ;;
      "") echo "CANNOT CHECK — $_pm_l: last run has no status $_pm_aged, $_pm_subject — UNKNOWN, not healthy. $_pm_url" ;;
      # `queued` forever is what an Actions billing block looks like: reported with its age, and
      # STUCK once it is more than a day old, never as a benign "no verdict yet".
      *) echo "$([ -n "$_pm_age" ] && [ "$_pm_age" -gt 1 ] && echo STUCK || echo RUNNING) $_pm_l: last run is $_pm_status $_pm_aged, $_pm_subject — no verdict. $_pm_url" ;;
    esac
  done
  rm -rf "$_pm_tmp"
}

# health_scheduled [--list]: every workflow with a `schedule:` trigger, and its last scheduled run.
#
# A scheduled run's failure reaches no PR and no person; this line is its reader. FOUR PROPERTIES,
# each earned by an incident (a weekly audit that never started under an Actions billing block,
# reported failure in three seconds, and was unnoticed for ten days):
#   1. It enumerates the AUTHORITY: the workflows' own `schedule:` keys, never a list.
#   2. It never degrades to silence: a failed lookup prints CANNOT CHECK, not nothing.
#   3. It reports the healthy case too, so absence is never ambiguous.
#   4. It reports AGE, not just the conclusion: GitHub disables a schedule after 60 days of repo
#      inactivity, and a disabled schedule leaves its last SUCCESS standing forever.
# --list prints only the workflow basenames it would check, one per line, with no network call, so
# a check can compare its enumeration with an independent reading of the same directory.
health_scheduled() {
  if [ ! -d "$ROOT/.github/workflows" ]; then
    [ "${1:-}" = --list ] || echo "OK no .github/workflows (nothing scheduled)"
    return 0
  fi
  _hs_wfs=$(_health_workflows schedule)
  if [ "${1:-}" = --list ]; then [ -z "$_hs_wfs" ] || printf '%s\n' "$_hs_wfs"; return 0; fi
  [ -n "$_hs_wfs" ] || { echo "OK no workflow in .github/workflows/ carries a schedule: trigger — nothing to watch"; return 0; }
  _health_tools "every scheduled workflow" || return 0
  _hs_tmp=$(mktemp -d "${TMPDIR:-/tmp}/health.XXXXXX" 2>/dev/null) || { echo "CANNOT CHECK — mktemp failed; scheduled results are UNKNOWN, not healthy"; return 0; }
  for _hs_n in $_hs_wfs; do
    _hs_f="$ROOT/.github/workflows/$_hs_n"
    if ! _health_gh "$_hs_tmp/out" "$_hs_tmp/err" run list --workflow "$_hs_n" --event schedule --limit 1 \
         --json conclusion,status,createdAt,url,databaseId; then
      echo "CANNOT CHECK — $_hs_n: gh failed ($(head -1 "$_hs_tmp/err")). UNKNOWN, not healthy."
      continue
    fi
    if ! _hs_p=$(jq -r 'if type != "array" then error("not an array") else
          (length | tostring), (.[0] // {} | .conclusion // "", .status // "", ((.createdAt // "")[0:10]), .url // "", (.databaseId // "" | tostring)) end' \
          "$_hs_tmp/out" 2>/dev/null); then
      echo "CANNOT CHECK — $_hs_n: could not parse gh output. UNKNOWN, not healthy."
      continue
    fi
    if [ "$(printf '%s\n' "$_hs_p" | sed -n 1p)" = 0 ]; then
      echo "NEVER RAN $_hs_n: no scheduled run on record yet (the schedule may never have fired)"
      continue
    fi
    _hs_concl=$(printf '%s\n' "$_hs_p" | sed -n 2p); _hs_status=$(printf '%s\n' "$_hs_p" | sed -n 3p)
    _hs_when=$(printf '%s\n' "$_hs_p" | sed -n 4p); _hs_url=$(printf '%s\n' "$_hs_p" | sed -n 5p)
    _hs_run=$(printf '%s\n' "$_hs_p" | sed -n 6p)
    # AGE BEFORE the status branch: a run stuck in `queued` (a billing block) once reported "no
    # verdict yet" with no age and no alarm, forever.
    _hs_age=$(health_days_since "$_hs_when")
    _hs_crons=$(health_crons "$_hs_f")

    # THE SECOND FACT, on any non-success: when this workflow last completed CLEAN, on the default
    # branch, by which trigger, at which commit. REPORTED, NOT JUDGED: a first version drew a
    # verdict from it ("dependencies are current") and review found six false claims in it. The
    # reader compares two dates. Positive evidence only: a failed lookup leaves it empty. Resolved
    # lazily, here, so the healthy path makes exactly one gh call.
    _hs_clean=""
    if [ "$_hs_concl" != success ]; then
      _hs_db=""
      _health_gh "$_hs_tmp/db" "$_hs_tmp/err" repo view --json defaultBranchRef --jq '.defaultBranchRef.name' && _hs_db=$(head -1 "$_hs_tmp/db")
      # --branch: GitHub runs schedules only on the default branch, but a dispatch runs anywhere
      # (a Dependabot branch, whose lockfile provably differs).
      if [ -n "$_hs_db" ] && _health_gh "$_hs_tmp/ok" "$_hs_tmp/err" run list --workflow "$_hs_n" --status success --branch "$_hs_db" --limit 1 \
           --json createdAt,event,url,headSha &&
         _hs_s=$(jq -r 'if type != "array" then error("not an array") else
              (length | tostring), (.[0] // {} | ((.createdAt // "")[0:10]), .event // "", .url // "", ((.headSha // "")[0:7])) end' \
              "$_hs_tmp/ok" 2>/dev/null); then
        _hs_swhen=$(printf '%s\n' "$_hs_s" | sed -n 2p); _hs_sevent=$(printf '%s\n' "$_hs_s" | sed -n 3p)
        _hs_surl=$(printf '%s\n' "$_hs_s" | sed -n 4p); _hs_ssha=$(printf '%s\n' "$_hs_s" | sed -n 5p)
        if [ "$(printf '%s\n' "$_hs_s" | sed -n 1p)" = 0 ]; then
          _hs_clean=" LAST CLEAN RUN on $_hs_db: none on record."
        elif [ -n "$_hs_swhen" ]; then
          _hs_sage=$(health_days_since "$_hs_swhen")
          # The commit is part of the fact: a clean audit names the lockfile it audited.
          _hs_clean=" LAST CLEAN RUN on $_hs_db: $_hs_swhen${_hs_sage:+ (${_hs_sage}d ago)}${_hs_sevent:+, $_hs_sevent}${_hs_ssha:+, at $_hs_ssha}."
          _hs_clean="$_hs_clean Compare the two dates yourself; this line draws no conclusion. $_hs_surl"
        fi
      fi
    fi

    _hs_stale=""
    [ -n "$_hs_age" ] && [ "$_hs_age" -gt 45 ] && _hs_stale=" — GitHub disables a schedule after 60d of repo inactivity and this is ${_hs_age}d, so check it still fires"

    case "$_hs_status" in
      queued | in_progress | waiting | requested | pending)
        if [ -n "$_hs_age" ] && [ "$_hs_age" -gt 1 ]; then
          echo "STUCK $_hs_n: scheduled run STUCK in $_hs_status since $_hs_when (${_hs_age}d) — a run that never starts is what a billing block looks like. $_hs_url$_hs_clean"
        else
          echo "RUNNING $_hs_n: scheduled run $_hs_status right now ($_hs_when) — no verdict yet. $_hs_url$_hs_clean"
        fi
        continue ;;
    esac

    if [ "$_hs_concl" = success ]; then
      echo "$([ -n "$_hs_stale" ] && echo STALE || echo OK) $_hs_n: last scheduled run OK ($_hs_when${_hs_age:+, ${_hs_age}d ago}${_hs_crons:+, cron $_hs_crons})$_hs_stale"
      continue
    fi

    # WHICH KIND OF FAILURE. A run that never started (a billing block) and a run that found a real
    # problem print the same conclusion and demand opposite responses. The signature is exact: jobs
    # that executed ZERO STEPS. Positive evidence only: the sentence needs jobs to exist and every
    # one to have zero steps; anything else, the extra call failing included, adds nothing. And zero
    # steps says it did not run, not WHY: only `failure` names the billing block; a `cancelled` run
    # (its own concurrency rule, or a spending limit cancelling queued runs) names both causes; any
    # other conclusion names none. One more call, on the failure path only.
    _hs_why=""
    if [ -n "$_hs_run" ] && _health_gh "$_hs_tmp/steps" "$_hs_tmp/err" run view "$_hs_run" --json jobs --jq '[.jobs[].steps|length]' &&
       _hs_c=$(jq -r 'if type != "array" then error("not an array") else (length | tostring), ([.[] | numbers] | add // 0 | tostring) end' "$_hs_tmp/steps" 2>/dev/null); then
      _hs_jobs=$(printf '%s\n' "$_hs_c" | sed -n 1p); _hs_total=$(printf '%s\n' "$_hs_c" | sed -n 2p)
      if [ -n "$_hs_jobs" ] && [ "$_hs_jobs" -gt 0 ] && [ "$_hs_total" = 0 ]; then
        case "$_hs_concl" in
          cancelled)
            _hs_why=" It was CANCELLED before any step ran (${_hs_jobs} job(s), 0 steps) — either the workflow's own concurrency rule dropping a queued run when a newer one started, or a quota/spending-limit cancellation. Nothing ran either way, so this run is not evidence about its subject. Re-run with:  gh workflow run $_hs_n" ;;
          failure)
            _hs_why=" It NEVER STARTED — ${_hs_jobs} job(s), 0 steps executed, conclusion failure. That is a billing/quota block or a disabled schedule, NOT a finding of the workflow: THIS RUN examined nothing, so it is not evidence either way about its subject. Check the Actions minutes allowance first, then re-run with:  gh workflow run $_hs_n" ;;
          *)
            _hs_why=" It ran no steps at all (${_hs_jobs} job(s), conclusion ${_hs_concl}), so nothing ran and this run is not evidence about its subject. Re-run with:  gh workflow run $_hs_n" ;;
        esac
      fi
    fi
    # ORDER: url, then the clean-run fact, then the diagnosis LAST, because it can end in a command
    # a reader copies, and a URL glued after it made the command uncopyable.
    echo "FAILED $_hs_n: TRIGGER — last scheduled run ${_hs_concl:-unknown} ($_hs_when${_hs_age:+, ${_hs_age}d ago}${_hs_crons:+, cron $_hs_crons}) — NOBODY IS NOTIFIED OF THIS. $_hs_url$_hs_clean$_hs_why"
  done
  rm -rf "$_hs_tmp"
}
