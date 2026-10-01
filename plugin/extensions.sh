#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# extensions.sh: run or print what the enabled modules and the local layer add to the operating
# model (lib/modules.sh names the points). The template's hosts call it; nothing else needs to.
#
#   sh .claude/extensions.sh health              every health line: .claude/health/, then each
#                                                enabled module's health/, then .claude/local/health/
#   sh .claude/extensions.sh context <skill>     every context.d/<skill>/*.sh, each under a heading
#   sh .claude/extensions.sh fragments <skill>   every skills/<skill>/*.md fragment, for the skill's
#                                                "Project steps" include line
#   sh .claude/extensions.sh conflicts           conflicts.tsv rows as session-close table rows
#   sh .claude/extensions.sh list                the enabled modules and every part they and the
#                                                local layer contribute; exit 1 if a module cannot load
#
# Every script runs per its own #! line (a bash health line is never forced through sh), and every
# section that cannot run says CANNOT CHECK instead of printing nothing: an absent answer must not
# read as a healthy one. Always exit 0 except `list`, so one broken part never hides the rest.
ROOT=$(case $CLAUDUCTOR_FW in (*/.claude) dirname "$CLAUDUCTOR_FW" ;; (*) [ -n "${ROOT:-}" ] && echo "$ROOT" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/conf.sh"
# shellcheck disable=SC1091
. "$CLAUDUCTOR_FW/lib/modules.sh" || { echo "CANNOT CHECK — .claude/lib/modules.sh is missing"; exit 0; }
cd "$ROOT" || exit 0
TAB=$(printf '\t')
ind() { sed 's/^/    /'; }

# section HEADING FILE: run FILE per its #! line under HEADING, indented, with CANNOT CHECK when
# it fails or says nothing.
section() {
  echo "$1"
  _out=$(run_shebang "$2" 2>&1 </dev/null); _rc=$?
  [ -n "$_out" ] && printf '%s\n' "$_out" | ind
  if [ "$_rc" -eq 127 ]; then :   # run_shebang said CANNOT CHECK itself
  elif [ "$_rc" -ne 0 ]; then echo "    CANNOT CHECK — ${2#"$ROOT"/} exited $_rc; this learned NOTHING, do not read it as healthy"
  elif [ -z "$_out" ]; then echo "    CANNOT CHECK — ${2#"$ROOT"/} printed nothing"; fi
}
# conflict_rows: every conflicts.tsv row as a table row, its layer named. A function, not inline in
# $(…): a case pattern's lone `)` inside a command substitution trips bash's parser.
conflict_rows() {
  ext_dirs conflicts.tsv | while IFS="$TAB" read -r label f; do
    [ -f "$f" ] || continue
    grep -v '^[[:space:]]*#' "$f" | grep -v '^[[:space:]]*$' | while IFS= read -r row; do
      case $row in
        *"$TAB"*) printf '| %s | %s (%s) |\n' "${row%%"$TAB"*}" "${row#*"$TAB"}" "$label" ;;
        *) printf '| %s | CANNOT CHECK — this row of %s has no TAB between the file and the rule |\n' "$row" "${f#"$ROOT"/}" ;;
      esac
    done
  done
}
# where LABEL: the suffix a module's or the local layer's heading carries.
where() { case $1 in local) echo " (local)" ;; *) echo " (${1})" ;; esac; }

case ${1:-} in
  health)
    for h in .claude/health/*.sh; do
      [ -f "$h" ] && section "- Health: $(basename "$h" .sh)" "$ROOT/$h"
    done
    ext_files health .sh | while IFS="$TAB" read -r label f; do
      section "- Health: $(basename "$f" .sh)$(where "$label")" "$f"
    done
    ;;
  context)
    [ -n "${2:-}" ] || { echo "usage: extensions.sh context <skill>" >&2; exit 2; }
    ext_files "context.d/$2" .sh | while IFS="$TAB" read -r label f; do
      section "- $(basename "$f" .sh)$(where "$label"):" "$f"
    done
    ;;
  fragments)
    [ -n "${2:-}" ] || { echo "usage: extensions.sh fragments <skill>" >&2; exit 2; }
    # A SKILL.md is a whole skill a module ships (enable.sh installs it), never a fragment of one.
    frags=$(ext_files "skills/$2" .md | grep -v '/SKILL\.md$')
    [ -n "$frags" ] && printf '%s\n' "$frags" | while IFS="$TAB" read -r label f; do
      echo "#### From the ${label} layer: $(basename "$f")"
      echo
      cat "$f"
      echo
    done
    [ -n "$frags" ] || echo "None. (A project adds steps here with .claude/local/skills/$2/<name>.md; an enabled module with skills/$2/<name>.md.)"
    ;;
  conflicts)
    # file TAB what to do on conflict; `#` comments and blank lines skipped.
    rows=$(conflict_rows)
    if [ -n "$rows" ]; then echo "| file | on conflict |"; echo "|---|---|"; printf '%s\n' "$rows"
    else echo "None."; fi
    ;;
  list)
    rc=0
    probs=$(modules_problems)
    if [ -n "$probs" ]; then printf '%s\n' "$probs" | sed 's/^/FAIL /'; rc=1; fi
    on=$(modules_enabled)
    echo "modules enabled: ${on:-none}" | tr '\n' ' ' | sed 's/ $//'; echo
    for p in $MODULE_POINTS; do
      case $p in
        context.d | skills) ext_dirs "$p" | while IFS="$TAB" read -r label dir; do
            for sub in "$dir"/*/; do [ -d "$sub" ] && for f in "$sub"*; do [ -f "$f" ] && echo "$label $p/$(basename "$sub")/$(basename "$f")"; done; done
          done ;;
        conflicts.tsv) ext_dirs "$p" | while IFS="$TAB" read -r label f; do echo "$label $p"; done ;;
        *) ext_files "$p" .sh | while IFS="$TAB" read -r label f; do echo "$label $p/$(basename "$f")"; done ;;
      esac
    done
    exit "$rc"
    ;;
  *)
    sed -n '2,/^# read as a healthy/p' "$0" | sed 's/^# \{0,1\}//' >&2
    exit 2
    ;;
esac
exit 0
