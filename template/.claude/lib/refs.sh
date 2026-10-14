# refs.sh: sourced (never run). The readers that pull references out of the model's sources, in
# one place, so the checks that hold them and the map that draws them (.claude/map.sh) cannot
# read one source two ways (OPS-40, D11). Two copies of one parser disagree silently, which is
# the rule roadmap-queue.sh exists for. checks/refs.sh holds the checks that read through here.
#
# Each reader's output format is its contract:
#   refs_named_paths FILE…        FILE TAB LINE TAB PATH, per .claude/… or ${CLAUDE_PLUGIN_ROOT}/…
#                                 path ending .sh .js .awk or .json (checks/skills.sh, map.sh)
#   refs_hook_registrations FILE  EVENT TAB COMMAND, per command hook a settings.json registers;
#                                 exits non-zero when the file does not parse (checks/hooks.sh)
#   refs_hook_scripts COMMAND     each script path in a hook command, one per line
#   refs_project_dir_prefix       the ERE of a path's project-root prefix ($CLAUDE_PROJECT_DIR/ in
#                                 each spelling a hook command may use), for grep -E or sed -E
#   refs_table_rows AGENTS_MD     LINE TAB ROW, per row of the table under "### What executes each
#                                 rule", header and separator excluded; ROW is the line, byte for
#                                 byte (checks/agents-md-budget.sh, map.sh)

# .claude/settings.local.json is left out here, not after the call: it is per machine and
# gitignored, named only as the place to loosen. A plugin project's skills and agents name the
# model as ${CLAUDE_PLUGIN_ROOT}/…; a caller that wants only the project's paths keeps .claude/….
refs_named_paths() {
  for _rf in "$@"; do
    [ -f "$_rf" ] || continue
    grep -noE '(\.claude|\$\{CLAUDE_PLUGIN_ROOT\})/[A-Za-z0-9_./-]+\.(sh|js|awk|json)' "$_rf" \
      | REFS_FILE=$_rf awk '{ i = index($0, ":"); p = substr($0, i + 1)
          if (p != ".claude/settings.local.json") printf "%s\t%s\t%s\n", ENVIRON["REFS_FILE"], substr($0, 1, i - 1), p }'
  done
}

refs_hook_registrations() {
  jq -r '(.hooks // {}) | to_entries[] | .key as $e | .value[] | .hooks[]? | select(.type == "command") | [$e, .command] | @tsv' "$1" 2>/dev/null
}

# The command is split into words at blanks and shell operators first (; | & ( ) < > and
# backtick), then a word is a script only if its extension ENDS it (a closing quote aside):
# settings.json, tsconfig.json or tools.shell_guard is not one. A word that starts with a scheme
# (https://…/install.sh, npm:pkg/x.ts) is a URL, not a path. checks/hooks.sh's table tests it.
refs_hook_scripts() {
  printf '%s\n' "$1" | tr -s ' \t;|&()<>`' '\n\n\n\n\n\n\n\n\n\n' \
    | grep -E '^["'"'"']?[A-Za-z0-9_./${}:"-]*[A-Za-z0-9_.-]+\.(sh|bash|js|mjs|cjs|py|rb|ts)["'"'"']?$' \
    | grep -vE '^["'"'"']?[A-Za-z][A-Za-z0-9+.-]*:' | sed 's/^'"'"'//; s/["'"'"']$//' || true
}

# "$CLAUDE_PROJECT_DIR"/, $CLAUDE_PROJECT_DIR/, ${CLAUDE_PROJECT_DIR}/, "${CLAUDE_PROJECT_DIR}"/
# and the status line's "${CLAUDE_PROJECT_DIR:-.}"/. checks/hooks.sh's relative-path cases test it.
refs_project_dir_prefix() {
  printf '%s\n' '^"?\$\{?CLAUDE_PROJECT_DIR(:-[^}]*)?\}?"?/'
}

# Counted from the third line under the heading: the header and the separator are not rows.
refs_table_rows() {
  awk '/^### What executes each rule/{p=1; next} p && /^#/{p=0} p && /^\|/ { if (++n > 2) printf "%d\t%s\n", NR, $0 }' "$1"
}
