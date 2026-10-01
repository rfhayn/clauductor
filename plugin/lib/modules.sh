# modules.sh: the loader for the operating model's extension points. Sourced (never run) after
# conf.sh, by the merge guard, the context scripts, checks/run.sh and .claude/extensions.sh, so
# every host finds the same parts in the same order.
#
# Two kinds of layer contribute (.claude/modules/README.md, .claude/local/README.md):
#   a MODULE   .claude/modules/<name>/, on only while project.conf's MODULES names it. Shipped
#              modules are the framework's (install refreshes them); a project may vendor its own
#              under the same directory.
#   the LOCAL  .claude/local/, the project's own layer: always on, and install and update never
#   layer      write a file there (doc tier).
# Both offer the same points:
#   guard.d/*.sh                    extra pr-merge-guard rules (exit 0 allow, stdout = advisories;
#                                   exit 2 block, stderr = why; anything else blocks)
#   context.d/<skill>/*.sh          extra sections in session-start's and session-close's context
#   health/*.sh                     extra session-start health lines (.claude/health/README.md)
#   checks/*.sh                     extra process checks, run by checks/run.sh
#   skills/<skill>/*.md             fragments appended to a template skill's instructions
#   conflicts.tsv                   rows appended to session-close's shared-file conflict table
#   roadmap.d/*.sh                  rules over the parsed change queue, run by roadmap_queue
#                                   (lib/conf.sh): stdin = the --tsv rows; exit 0 accepts; anything
#                                   else makes the queue UNKNOWN, stdout naming each refused row
# Order: modules in MODULES order, then the local layer; within a directory, by file name.

# The modules the model ships. In the plugin build this path is the plugin's own modules/.
MODULES_FW="$CLAUDUCTOR_FW/modules"
# The project's own modules. Spelt with the quote before /modules on purpose: the plugin build
# rewrites "$ROOT/.claude/modules" to the plugin's copy, and this one must stay the repository's.
MODULES_OWN="$ROOT/.claude"/modules
LOCAL_DIR="$ROOT/.claude/local"
# The points a module.conf may name in `enables`. A part on disk that its module does not
# declare, or a declared one that is missing, is a finding (checks/modules.sh).
MODULE_POINTS="guard.d context.d health checks skills conflicts.tsv roadmap.d"

# modules_enabled: one enabled module name per line, deduplicated, MODULES order. The two legacy
# switches still turn their modules on: PROPOSALS="openspec" and REVIEW_PAGE="artifact".
modules_enabled() {
  _me=${MODULES:-}
  [ "${PROPOSALS:-}" = openspec ] && _me="$_me openspec"
  [ "${REVIEW_PAGE:-}" = artifact ] && _me="$_me review-page"
  # shellcheck disable=SC2086
  [ -n "$(printf '%s' "$_me" | tr -d ' ')" ] && printf '%s\n' $_me | awk 'NF && !seen[$0]++'
  return 0
}

# module_dir NAME: the module's directory (shipped first, then the project's), or exit 1.
module_dir() {
  case $1 in '' | *[!a-z0-9-]* | -*) return 1 ;; esac
  if [ -f "$MODULES_FW/$1/module.conf" ]; then printf '%s\n' "$MODULES_FW/$1"
  elif [ -f "$MODULES_OWN/$1/module.conf" ]; then printf '%s\n' "$MODULES_OWN/$1"
  else return 1; fi
}

# module_meta DIR KEY: a lowercase metadata key of DIR/module.conf (name, requires, enables).
module_meta() {
  sed -n "s/^$2=\"\\([^\"]*\\)\".*/\\1/p" "$1/module.conf" 2>/dev/null | head -n 1
}

# modules_defaults: each enabled module's default keys (UPPERCASE="value" lines of its
# module.conf), set only where project.conf set nothing; then the legacy switches follow the
# list, so a check that reads PROPOSALS or REVIEW_PAGE sees the module that is on. A line that
# needs a shell to read ($, backtick, backslash) is not applied.
modules_defaults() {
  for _md in $(modules_enabled); do
    _mdd=$(module_dir "$_md") || continue
    while IFS= read -r _ml; do
      [ -n "$_ml" ] || continue
      _mk=${_ml%%=*}
      eval "_mcur=\${$_mk:-}"
      # shellcheck disable=SC2154
      [ -n "$_mcur" ] || eval "$_ml"
    done <<EOF
$(grep -E '^[A-Z][A-Z0-9_]*="[^"$`\\]*"[[:space:]]*$' "$_mdd/module.conf" 2>/dev/null)
EOF
    case $_md in openspec) PROPOSALS=openspec ;; review-page) REVIEW_PAGE=artifact ;; esac
  done
  return 0
}

# modules_problems: one line per enabled module that cannot load: unknown, a name that differs
# from its directory, or a module it requires that is not on. Empty when every one loads.
modules_problems() {
  _mp_on=$(modules_enabled)
  for _mp in $_mp_on; do
    if ! _mpd=$(module_dir "$_mp"); then
      echo "MODULES names '$_mp', but there is no .claude/modules/$_mp/module.conf"; continue
    fi
    _mpn=$(module_meta "$_mpd" name)
    [ "$_mpn" = "$_mp" ] || echo "module $_mp: module.conf says name=\"$_mpn\", not \"$_mp\""
    for _mpr in $(module_meta "$_mpd" requires); do
      printf '%s\n' "$_mp_on" | grep -qx -- "$_mpr" || echo "module $_mp requires module $_mpr, which MODULES does not enable"
    done
  done
}

# ext_dirs POINT: "<label>\t<dir>" for each existing POINT directory (or file, for conflicts.tsv),
# enabled modules first, then the local layer. Label: "module <name>" or "local".
ext_dirs() {
  for _ed in $(modules_enabled); do
    _edd=$(module_dir "$_ed") || continue
    [ -e "$_edd/$1" ] && printf 'module %s\t%s\n' "$_ed" "$_edd/$1"
  done
  [ -e "$LOCAL_DIR/$1" ] && printf 'local\t%s\n' "$LOCAL_DIR/$1"
  return 0
}

# ext_files POINT SUFFIX: "<label>\t<file>" for each POINT/*SUFFIX file, in ext_dirs order.
ext_files() {
  ext_dirs "$1" | while IFS="$(printf '\t')" read -r _el _edir; do
    [ -d "$_edir" ] || continue
    for _ef in "$_edir"/*"$2"; do
      [ -f "$_ef" ] && printf '%s\t%s\n' "$_el" "$_ef"
    done
  done
}

# shebang_interp FILE: the command line that runs FILE, from its #! line: `#!/usr/bin/env bash`
# is bash, `#!/bin/bash -e` is /bin/bash -e, no #! line is sh. An absolute interpreter that is not
# there falls back to its name on PATH. A bash script is never forced through sh (dash on Ubuntu
# fails on bash-only syntax, and a health line that fails reads as a broken subject).
shebang_interp() {
  _sl=$(head -n 1 "$1" 2>/dev/null | tr -d '\r')
  case $_sl in '#!'*) ;; *) echo sh; return 0 ;; esac
  # shellcheck disable=SC2086
  set -- ${_sl#??}
  case ${1:-} in */env | env) shift; [ "${1:-}" = -S ] && shift ;; esac
  [ $# -gt 0 ] || { echo sh; return 0; }
  _si=$1; shift
  case $_si in /*) [ -x "$_si" ] || _si=${_si##*/} ;; esac
  echo "$_si${*:+ $*}"
}

# shebang_parse FILE: 0 when FILE parses under its own interpreter (sh -n, bash -n, …); an
# interpreter that is not a shell cannot be asked, and passes here. 1 = does not parse, 127 = its
# interpreter is not installed.
shebang_parse() {
  _pi=$(shebang_interp "$1")
  _pb=${_pi%% *}
  command -v "$_pb" >/dev/null 2>&1 || return 127
  case ${_pb##*/} in
    sh | bash | dash | ksh | zsh | mksh | ash) "$_pb" -n "$1" >/dev/null 2>&1 ;;
    *) return 0 ;;
  esac
}

# run_shebang FILE [ARGS]: run FILE with the interpreter its #! line names (shebang_interp).
# Exit 127, with a CANNOT CHECK line, when that interpreter is not installed.
run_shebang() {
  _rf=$1; shift
  _ri=$(shebang_interp "$_rf")
  _rb=${_ri%% *}
  if ! command -v "$_rb" >/dev/null 2>&1; then
    echo "CANNOT CHECK — $_rf needs $_rb (its #! line), which is not installed"
    return 127
  fi
  # shellcheck disable=SC2086
  $_ri "$_rf" "$@"
}

# with_timeout SECONDS COMMAND [ARGS]: run an EXTERNAL command (not a function: a function runs in
# a subshell whose children a signal does not reach), TERM it after SECONDS and KILL it 2 s
# later. Its status, or 143/137 when it was stopped. The command keeps the caller's stdin: an
# asynchronous command's stdin is /dev/null in a non-interactive shell unless redirected
# explicitly, hence the 0<&0.
with_timeout() {
  _ws=$1; shift
  "$@" 0<&0 &
  _wp=$!
  (
    trap 'exit 0' TERM
    _wn=0
    while [ "$_wn" -lt "$_ws" ]; do sleep 1; _wn=$((_wn + 1)); kill -0 "$_wp" 2>/dev/null || exit 0; done
    kill -TERM "$_wp" 2>/dev/null; sleep 2; kill -KILL "$_wp" 2>/dev/null
  ) </dev/null >/dev/null 2>&1 &
  _ww=$!
  wait "$_wp"; _wrc=$?
  kill "$_ww" 2>/dev/null; wait "$_ww" 2>/dev/null
  return "$_wrc"
}
