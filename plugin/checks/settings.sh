#!/bin/sh
STRIDAL_FW=$(cd "$(dirname "$0")/.." && pwd) # stridal plugin: the plugin root (framework/internal/plugin)
# Agent least privilege (OWASP LLM01/LLM06; item 10 of the change process): .claude/settings.json
# denies Claude the project's secrets and the few commands no session should run, and turns the Bash
# sandbox on. A deny rule is enforced by Claude Code whatever the permission mode, and the sandbox
# confines what a command can write and reach at the OS level: both are structure, not prose
# (*Reach is set by structure, not by the prompt*).
#
# Fails when: the deny list or the sandbox is missing; a rule from the required set below is gone;
# a deny rule is not `Tool(...)` with a tool Claude Code knows (a misspelt rule denies nothing);
# the sandbox is not enabled, or has no network allowlist. Add your own rules freely; remove one of
# the required set only by editing this list in a PR that says why. A tool that fails in the
# sandbox is loosened per machine in .claude/settings.local.json (sandbox.excludedCommands,
# sandbox.network.allowedDomains merge across scopes), never by turning the sandbox off here.
# `~/.config/gh` is deliberately not denied: a sandboxed script's gh must read hosts.yml to find
# its keyring account, so gh's token belongs in a keyring (docs/playbook.md; OPS-75). The keys are Claude Code's
# (code.claude.com/docs/en/permissions, /docs/en/sandboxing; checked 2026-09-30).
. "$(dirname "$0")/lib.sh"
need jq

REQUIRED_DENY='Read(./.env)
Read(./.env.*)
Edit(./.env)
Edit(./.env.*)
Read(~/.ssh/**)
Read(~/.aws/**)
Bash(git push --force *)
Bash(git push -f *)
Bash(tmux *kill-server*)
Bash(cmd.exe *)
Bash(powershell.exe *)
Bash(pwsh.exe *)
Bash(wsl.exe *)
Bash(explorer.exe *)
Bash(clip.exe *)
Bash(notepad.exe *)
Bash(/mnt/c/*)'
# Windows interop (WSL2): WSL hands a launch of cmd.exe, powershell.exe or anything under /mnt/c/ to
# the Windows host over a Unix socket, so it runs OUTSIDE the sandbox unless the optional seccomp
# filter blocks that socket (code.claude.com/docs/en/sandboxing, WSL2 notes; checked 2026-10-01),
# and sandbox.autoAllowBashIfSandboxed would approve it without a prompt. The deny rules above stop
# the forms Claude writes (a deny rule is respected even in auto-allow mode); the seccomp filter,
# which docs/playbook.md makes required on WSL2, is the boundary for the rest. Listing any of them in
# sandbox.excludedCommands would run them unsandboxed by design, so that fails.
# /mnt/c/ only, not /mnt/: a rule has no character class for "a drive letter", a deny rule cannot
# be undone by a narrower allow, and /mnt/ also holds ordinary Linux mounts. C: holds Windows
# itself; programs on other drives are left to the seccomp filter. `code` (VS Code's WSL shim) is
# NOT denied: on macOS and Linux `code <file>` is the plain editor launcher, not interop, and on
# WSL2 its launch of the Windows editor is covered by the seccomp filter.
INTEROP_RE='(^|[ /])(cmd|powershell|pwsh|wsl|explorer|clip|notepad)\.exe|^/mnt/c/'
KNOWN_TOOLS=" Read Edit Write MultiEdit NotebookEdit Bash WebFetch WebSearch Agent Task Skill Glob Grep ${CHECK_EXTRA_TOOLS:-} "

check_settings() {  # check_settings FILE: prints ok/FAIL lines
  f=$1
  jq -e . "$f" >/dev/null 2>&1 || { echo "FAIL $f is missing or not valid JSON"; return; }
  if ! jq -e '.permissions.deny | type == "array" and length > 0' "$f" >/dev/null 2>&1; then
    echo "FAIL settings.json has no permissions.deny list"
  else
    printf '%s\n' "$REQUIRED_DENY" | while IFS= read -r r; do
      jq -e --arg r "$r" '.permissions.deny | index($r)' "$f" >/dev/null && echo "ok   denies $r" || echo "FAIL permissions.deny lacks $r"
    done
    jq -r '.permissions.deny[]' "$f" | while IFS= read -r r; do
      t=$(printf '%s' "$r" | sed -n 's/^\([A-Za-z]*\)(.*)$/\1/p')
      [ -z "$t" ] && t=$(printf '%s' "$r" | grep -E '^[A-Za-z]+$')
      case "$t" in mcp__*) continue ;; esac
      case "$KNOWN_TOOLS" in *" ${t:-?} "*) ;; *) echo "FAIL deny rule '$r' is not Tool(...) with a known tool (typo? else CHECK_EXTRA_TOOLS)" ;; esac
    done
  fi
  if ! jq -e '.sandbox | type == "object"' "$f" >/dev/null 2>&1; then
    echo "FAIL settings.json has no sandbox settings"
  else
    jq -e '.sandbox.enabled == true' "$f" >/dev/null && echo "ok   the Bash sandbox is enabled" || echo "FAIL sandbox.enabled is not true (loosen a tool in .claude/settings.local.json instead: sandbox.excludedCommands)"
    jq -e '(.sandbox.network.allowedDomains | type == "array" and length > 0)' "$f" >/dev/null \
      && echo "ok   the sandbox's network allowlist names $(jq '.sandbox.network.allowedDomains | length' "$f") domain(s)" \
      || echo "FAIL sandbox.network.allowedDomains is missing or empty: a sandboxed command's network would be unbounded or prompt every time"
    jq -e '.sandbox.allowUnsandboxedCommands | type == "boolean"' "$f" >/dev/null \
      && echo "ok   sandbox.allowUnsandboxedCommands is set ($(jq .sandbox.allowUnsandboxedCommands "$f"))" \
      || echo "FAIL sandbox.allowUnsandboxedCommands is not set: say whether a command that fails in the sandbox may retry outside it (through the permission flow)"
    _x=$(jq -r '(.sandbox.excludedCommands // [])[]' "$f" | grep -E "$INTEROP_RE")
    [ -z "$_x" ] && echo "ok   no Windows interop command (cmd.exe, powershell.exe, pwsh.exe, wsl.exe, /mnt/...) is excluded from the sandbox" \
      || echo "FAIL sandbox.excludedCommands runs Windows interop outside the sandbox: $(printf '%s' "$_x" | tr '\n' ' ')"
    # The gate and gate-wait run outside the sandbox (OPS-72): their process checks use `ps`, tmux
    # and loopback, which the sandbox refuses. An entry lifts the sandbox only for the script run
    # ALONE, bare or with arguments, so both forms are listed: the receipt runs the gate bare, and a
    # lane runs either with `-C <worktree>` (OPS-161). gate-wait.sh is where install put it (gate_wait_path, conf.sh):
    # beside the template's runner, but at scripts/ci/ beside a project's own runner.
    # Both as install writes them: the gate as a command (gate_cmd), gate-wait where it put it.
    # An empty GATE is the template's gate: build-change runs scripts/ci/gate.sh then (`c.gate ||`),
    # and the settings an install wrote before it emptied still name that (OPS-87). Only empty: any
    # other value, absolute or outside the repo included, build-change runs as given.
    _gt=$(gate_cmd "$GATE"); [ -n "$_gt" ] || _gt=scripts/ci/gate.sh
    for _g in "$_gt" "$(gate_wait_path)"; do
      for _p in "$_g" "$_g *"; do
        jq -e --arg p "$_p" '(.sandbox.excludedCommands // []) | index($p)' "$f" >/dev/null \
          && echo "ok   sandbox.excludedCommands lists '$_p'" \
          || echo "FAIL sandbox.excludedCommands lacks '$_p': build-change's gate runs it sandboxed, where its process checks fail whatever the code did"
      done
    done
  fi
}

# ── Every gate command an instruction gives runs outside the sandbox (OPS-161) ───────────────
# An exclusion lifts the sandbox only for a command that IS the script: `P` exactly, `P *` with
# arguments, and nothing chained, piped or redirected around it. A subagent lane's Bash starts in the
# main checkout, so `cd <worktree> && <gate>` was the natural instruction, and it ran the gate
# sandboxed, where `ps`, git's caches and the loopback name fail whatever the code does. The lane
# form is `<gate> -C <worktree>`. This reads the Markdown instructions (AGENTS.md, CLAUDE.md, the
# skills' and agents' files, the playbook and the conventions) in UNITS: a paragraph's code spans,
# read across its line breaks as Markdown renders them, a table row's, or a code block, read as
# commands unless its label (any case) names prose or another language (text, diff, json, go, …);
# its `#` comments and `$ ` prompts dropped. Two rules:
#   1. each span or block line that names the gate or gate-wait matches an entry of settings.json's
#      own list, `<placeholder>`s filled with a path, so an instruction and the exclusion cannot part;
#   2. no unit both changes directory (cd, pushd) and names the gate or gate-wait without -C. A
#      deliberate over-read with no shell grammar (ADR-0012 rejected chasing each form): a cd on the
#      line before, a backslash continuation or a subshell is caught the same way.
# A counter-example is exempt only when `<!-- gate-check: example -->` stands on a line of its own
# before its paragraph or block, and the exemption covers that one unit. build-change's gate prompts
# are JavaScript, held to its LONE rule by checks/build-change.sh instead.
GATE_EXAMPLE='<!-- gate-check: example -->'
excluded() {  # excluded SETTINGS CMD: CMD runs outside the sandbox by SETTINGS' excludedCommands
  case $2 in *'&'* | *'|'* | *';'* | *'>'* | *'<'* | *'$('* | *'`'* | *'
'*) return 1 ;; esac
  jq -r '(.sandbox.excludedCommands // [])[]' "$1" | {
    while IFS= read -r _e; do
      case $_e in
        *' *') case $2 in "${_e% \*} "?*) exit 0 ;; esac ;;
        *) [ "$2" = "$_e" ] && exit 0 ;;
      esac
    done
    exit 1
  }
}
# gate_units FILE...: `C<TAB>file<TAB>unit<TAB>command` for each span or block line, and
# `U<TAB>file<TAB>unit<TAB>the unit's commands joined by " ; "`, for every unit not marked an example.
gate_units() {
  awk -v example="$GATE_EXAMPLE" '
    # A tab is a space to the shell, and the field separator here: kept, it would cut the command.
    function add(s) { gsub(/\t/, " ", s); c[++n] = s; j = j (j == "" ? "" : " ; ") s }
    function out(i) {
      if (unit) {
        if (pending) pending = 0
        else if (n > 0) { u++; for (i = 1; i <= n; i++) print "C\t" file "\t" u "\t" c[i]; print "U\t" file "\t" u "\t" j }
      }
      n = 0; j = ""; unit = 0
    }
    function flush(l) {
      if (para == "") return
      l = para; para = ""; unit = 1
      while (match(l, /`[^`]+`/)) { add(substr(l, RSTART + 1, RLENGTH - 2)); l = substr(l, RSTART + RLENGTH) }
      out()
    }
    FNR == 1 { flush(); out(); file = FILENAME; fence = ""; pending = 0 }
    fence != "" {
      l = $0; sub(/^[ \t]+/, "", l)
      if (substr(l, 1, 3) == fence) { out(); fence = ""; next }
      if (!cmds) next
      gsub(/[ \t]+$/, "", l); sub(/^\$ /, "", l)
      if (l == "" || l ~ /^#/) next
      sub(/[ \t]+#.*/, "", l); add(l); next
    }
    /^[ \t]*(```|~~~)/ {
      flush(); l = $0; sub(/^[ \t]+/, "", l); fence = substr(l, 1, 3)
      info = tolower(l); sub(/^[`~]+[ \t]*/, "", info); sub(/[ \t{].*/, "", info)
      # Fail closed: a block reads as commands unless its label names something else.
      cmds = (info !~ /^(text|txt|plaintext|diff|json|yaml|yml|toml|go|js|javascript|ts|typescript|python|py|html|css|md|markdown)$/); unit = 1; next
    }
    { l = $0; gsub(/^[ \t]+|[ \t]+$/, "", l) }
    l == example { flush(); pending = 1; next }
    l == "" { flush(); next }
    l ~ /^\|/ { flush(); para = l; flush(); next }
    { para = para (para == "" ? "" : " ") l }
    END { flush(); out() }
  ' "$@" 2>/dev/null
}
check_lane_gates() {  # check_lane_gates SETTINGS FILE...: prints ok/FAIL lines
  _s=$1; shift
  _gt=$(gate_cmd "$GATE"); [ -n "$_gt" ] || _gt=scripts/ci/gate.sh
  _re=$(printf '%s\n%s\n' "$_gt" "$(gate_wait_path)" | sed 's/[].[\*^$]/\\&/g' | paste -sd'|' -)
  _names="(^|[[:space:];&|(])(\\./)?($_re)([[:space:];&|)]|\$)"
  # FILE<TAB>TEXT: the tab is the only one, so a pattern after it reads TEXT alone.
  _infield="	(.*[[:space:];&|(])?(\\./)?($_re)([[:space:];&|)]|\$)"
  _how="Give the lane form (\`<gate> -C <worktree>\`), never cd before the gate or pipe it. To quote one as what NOT to do, put $GATE_EXAMPLE on its own line before its paragraph or block"
  gate_units "$@" > "$d/units"
  awk -F'\t' '$1 == "C" { print $2 "\t" $4 }' "$d/units" | grep -E "$_infield" > "$d/gatecmds"
  _n=0
  while IFS='	' read -r _f _c; do
    _n=$((_n + 1))
    _x=$(printf '%s' "$_c" | sed 's|<[A-Za-z_-]*>|/abs/lane|g')
    excluded "$_s" "$_x" || echo "FAIL ${_f#"$ROOT"/}: \`$_c\` matches no sandbox.excludedCommands entry: run as written, the gate runs sandboxed and its process checks fail. $_how"
  done < "$d/gatecmds"
  awk -F'\t' '$1 == "U" { print $2 "\t" $4 }' "$d/units" | grep -E "$_infield" | while IFS='	' read -r _f _u; do
    printf '%s\n' "$_u" | grep -qE '(^|[[:space:];&|(])(cd|pushd)([[:space:]]|$)' || continue
    # What names the gate once every `<gate> -C` is taken out: a gate this unit runs where it cd'd.
    printf '%s\n' "$_u" | sed -E "s#(\\./)?($_re) -C##g" | grep -qE "$_names" || continue
    echo "FAIL ${_f#"$ROOT"/}: a paragraph or block changes directory and names the gate or gate-wait without -C ($(printf '%s' "$_u" | cut -c1-240)): a lane following it runs the gate sandboxed. $_how"
  done
  echo "ok   $_n gate command(s) in the instructions checked against sandbox.excludedCommands"
  grep -q ' -C ' "$d/gatecmds" && echo "ok   an instruction gives the lane form, the gate with -C <worktree>" \
    || echo "FAIL no instruction gives the lane form (\`<gate> -C <worktree>\`): a subagent lane is left to cd into its worktree, and its gate runs sandboxed"
}

# Self-test: the template's settings pass; each broken shape fails.
st="$ROOT/.claude/settings.json"
d=$(scratch)
selftest() {  # selftest WANT LABEL JQ-FILTER [GATE_RUN] [GATE]: the last two, for a project whose gate moved
  jq "$3" "$st" > "$d/s.json"
  n=$(GATE_RUN=${4:-$GATE_RUN}; GATE=${5-$GATE}; check_settings "$d/s.json" | grep -c '^FAIL')
  if [ "$1" = pass ] && [ "$n" -eq 0 ]; then ok "self-test: $2 passes"
  elif [ "$1" = fail ] && [ "$n" -gt 0 ]; then ok "self-test: $2 fails"
  else fail "self-test: $2 ($n FAIL line(s), want $1)"; fi
}
selftest pass "the template's settings" '.'
selftest fail "no deny list" 'del(.permissions.deny)'
selftest fail "a required deny rule removed" '.permissions.deny -= ["Read(./.env)"]'
selftest fail "a misspelt deny rule" '.permissions.deny += ["Raed(./secrets/**)"]'
selftest fail "no sandbox" 'del(.sandbox)'
selftest fail "the sandbox turned off" '.sandbox.enabled = false'
selftest fail "no network allowlist" 'del(.sandbox.network)'
selftest fail "allowUnsandboxedCommands unset" 'del(.sandbox.allowUnsandboxedCommands)'
selftest fail "Windows interop no longer denied (cmd.exe)" '.permissions.deny -= ["Bash(cmd.exe *)"]'
selftest fail "Windows paths no longer denied (/mnt/c/)" '.permissions.deny -= ["Bash(/mnt/c/*)"]'
selftest fail "explorer.exe no longer denied" '.permissions.deny -= ["Bash(explorer.exe *)"]'
selftest fail "clip.exe excluded from the sandbox" '.sandbox.excludedCommands += ["clip.exe *"]'
selftest pass "an ordinary Linux mount (/mnt/data) excluded from the sandbox" '.sandbox.excludedCommands += ["/mnt/data/tool *"]'
selftest fail "powershell.exe excluded from the sandbox" '.sandbox.excludedCommands += ["powershell.exe *"]'
selftest fail "a /mnt/c binary excluded from the sandbox" '.sandbox.excludedCommands += ["/mnt/c/Windows/System32/cmd.exe *"]'
selftest fail "gate-wait no longer excluded from the sandbox" '.sandbox.excludedCommands -= ["scripts/ci/gate-wait.sh *"]'
selftest fail "the bare gate no longer excluded from the sandbox" '.sandbox.excludedCommands -= ["scripts/ci/gate.sh"]'
# Where gate-wait.sh is depends on the runner, as install placed it (RenderSettings renders these).
selftest pass "a project's own runner (GATE_RUN=bin/ci.sh): gate-wait stays at scripts/ci/" \
  '.sandbox.excludedCommands |= map(sub("^scripts/ci/run-local\\.sh"; "bin/ci.sh"))' bin/ci.sh
selftest pass "the template's runner moved (GATE_RUN=ci/run-local.sh): gate-wait moved with it" \
  '.sandbox.excludedCommands |= map(sub("^scripts/ci/(?<f>run-local|gate-wait)\\.sh"; "ci/\(.f).sh"))' ci/run-local.sh
selftest fail "the template's runner moved, gate-wait's exclusion left at scripts/ci/" '.' ci/run-local.sh
# A runner at the repository root puts gate-wait at ./gate-wait.sh (path.Dir is "."), as Go writes it.
selftest pass "a runner at the repository root (GATE_RUN=run-local.sh): gate-wait at ./" \
  '.sandbox.excludedCommands |= map(sub("^scripts/ci/gate-wait\\.sh"; "./gate-wait.sh"))' run-local.sh
selftest pass "a runner at the repository root (GATE_RUN=./run-local.sh): gate-wait at ./" \
  '.sandbox.excludedCommands |= map(sub("^scripts/ci/gate-wait\\.sh"; "./gate-wait.sh"))' ./run-local.sh
# Install writes the paths CLEANED (path.Clean), so the check reads GATE and GATE_RUN the same way.
selftest pass "GATE written ./scripts/ci/gate.sh: the settings name it scripts/ci/gate.sh" '.' '' ./scripts/ci/gate.sh
# A gate at the repository root keeps ./ (a bare gate.sh would be looked up on PATH), as install writes it.
selftest pass "a gate at the repository root (GATE=./gate.sh): the settings name ./gate.sh" \
  '(.sandbox.excludedCommands, .permissions.allow) |= map(sub("scripts/ci/gate\\.sh"; "./gate.sh"))' '' ./gate.sh
selftest fail "a gate at the repository root, excluded only as a bare gate.sh" \
  '(.sandbox.excludedCommands, .permissions.allow) |= map(sub("scripts/ci/gate\\.sh"; "gate.sh"))' '' ./gate.sh
selftest pass "GATE written scripts/ci//gate.sh, GATE_RUN ci/../scripts/ci/run-local.sh/" '.' ci/../scripts/ci/run-local.sh/ scripts/ci//gate.sh
# GATE="" runs the template's gate (build-change's fallback), so its exclusion is the one read (OPS-87).
selftest pass "GATE empty: the settings name scripts/ci/gate.sh, which build-change then runs" '.' '' ''
selftest fail "GATE empty, the default gate no longer excluded" '.sandbox.excludedCommands -= ["scripts/ci/gate.sh"]' '' ''
selftest fail "GATE absolute (/abs/gate.sh), the settings naming only the default gate" '.' '' /abs/gate.sh

check_settings "$st" > "$d/real"
cat "$d/real"; _fails=$((_fails + $(grep -c '^FAIL' "$d/real")))

# The lane's gate commands, self-tested at the template's paths whatever this project's are: each
# broken instruction fails, against settings that exclude both scripts bare and with arguments.
jq -n '{sandbox: {excludedCommands: ["git *", "scripts/ci/gate.sh", "scripts/ci/gate.sh *", "scripts/ci/gate-wait.sh", "scripts/ci/gate-wait.sh *"]}}' > "$d/lane.json"
lanes_at_template() {  # lanes_at_template SETTINGS FILE...: check_lane_gates with the template's gate paths
  (GATE=scripts/ci/gate.sh; GATE_RUN=scripts/ci/run-local.sh; check_lane_gates "$@")
}
lanetest() {  # lanetest pass|fail LABEL LINE...: instructions holding LINEs after the lane form
  _want=$1 _lbl=$2; shift 2
  { printf 'Lane: `scripts/ci/gate.sh -C <worktree>`, then `scripts/ci/gate-wait.sh -C <worktree>`.\n\n'
    printf '%s\n' "$@"; } > "$d/lane.md"
  n=$(lanes_at_template "$d/lane.json" "$d/lane.md" | grep -c '^FAIL')
  if [ "$_want" = pass ] && [ "$n" -eq 0 ]; then ok "lane self-test: $_lbl passes"
  elif [ "$_want" = fail ] && [ "$n" -gt 0 ]; then ok "lane self-test: $_lbl fails"
  else fail "lane self-test: $_lbl ($n FAIL line(s), want $_want)"; fi
}
lanetest pass "the lane form, a quick gate with -C, the bare gate" 'Run `scripts/ci/gate.sh -C <worktree> --quick` or `scripts/ci/gate.sh`.'
lanetest pass "a fenced block naming the gate, with a comment" '```bash' 'scripts/ci/gate.sh -C <worktree>   # the full gate' '```'
lanetest pass "a script of the same file name (tools/ci/gate.sh) is no gate" 'Run `cd x && sh tools/ci/gate.sh`.'
lanetest fail "a gate started by cd (S16's lanes)" 'Run `cd <worktree> && scripts/ci/gate.sh`.'
lanetest fail "a piped gate" 'Run `scripts/ci/gate.sh -C <worktree> | tail -n 40`.'
lanetest fail "a gate-wait started by cd" 'Then `cd <worktree>; scripts/ci/gate-wait.sh`.'
lanetest fail "a fenced cd && gate" '```' 'cd /abs/lane && scripts/ci/gate.sh' '```'
lanetest fail "a cd && gate span hard-wrapped across two lines" 'Run `cd <worktree> &&' '  scripts/ci/gate.sh` from the lane.'
lanetest pass "a fenced # comment naming a cd && gate" '```sh' '# not: cd <worktree> && scripts/ci/gate.sh' 'scripts/ci/gate.sh -C <worktree>' '```'
lanetest pass "a counter-example with the gate's path left out" 'Never `cd <worktree> && …`.'
lanetest pass "spans in two paragraphs are never joined" 'Run `cd <worktree>` first.' '' 'Then `scripts/ci/gate.sh`.'
lanetest pass "spans in two table rows are never joined" '| a | `cd <worktree>` |' '| b | `scripts/ci/gate-wait.sh` |'
lanetest fail "a table row that cds and names the gate" '| a | `cd <worktree>`, then `scripts/ci/gate.sh` |'
# A unit that changes directory and names the gate without -C fails, whatever the shell form.
lanetest fail "a block: cd on one line, the gate on the next" '```' 'cd <worktree>' 'scripts/ci/gate.sh' '```'
lanetest fail "a block: a backslash continuation after cd &&" '```bash' 'cd <worktree> && \' 'scripts/ci/gate.sh --quick' '```'
lanetest fail "a ~~~sh block: cd, then the gate" '~~~sh' 'cd /abs/lane' 'scripts/ci/gate.sh' '~~~'
lanetest fail "a paragraph: cd in one span, the gate in the next" 'Run `cd <worktree>`, then `scripts/ci/gate-wait.sh`.'
lanetest fail "a pushd before the gate" '```console' '$ pushd <worktree>' '$ scripts/ci/gate.sh' '```'
lanetest pass "a block that cds and runs the gate with -C" '```sh' 'cd <worktree>' 'scripts/ci/gate.sh -C <worktree>' '```'
lanetest pass "a counter-example marked $GATE_EXAMPLE (a block)" "$GATE_EXAMPLE" '```' 'cd <worktree> && scripts/ci/gate.sh' '```'
lanetest pass "a counter-example marked $GATE_EXAMPLE (a paragraph)" "$GATE_EXAMPLE" 'Never `cd <worktree> && scripts/ci/gate.sh`.'
lanetest fail "the mark covers one unit only" "$GATE_EXAMPLE" 'Never `cd <worktree> && scripts/ci/gate.sh`.' '' 'Run `cd <worktree> && scripts/ci/gate.sh`.'
lanetest fail "a tab between cd && and the gate" "Run \`cd <worktree> &&	scripts/ci/gate.sh\`."
lanetest fail "a \`\`\`Bash block (fail closed: a label not listed as prose reads as commands)" '```Bash' 'cd <worktree>' 'scripts/ci/gate.sh' '```'
lanetest pass "a \`\`\`Text block quoting gate-wait's hint (labels any case)" '```Text' 'NO RESULT: Start a gate in the FOREGROUND: cd <worktree> && scripts/ci/gate.sh (Bash timeout 600000).' '```'
lanetest fail "a \`\`\`shell-session block" '```shell-session' '$ cd <worktree> && scripts/ci/gate.sh' '```'
lanetest pass "a console block's lane form after its prompt" '```console' '$ scripts/ci/gate.sh -C <worktree>' '```'
lanetest pass "a text block quoting gate-wait's hint (not commands)" '```text' 'NO RESULT: Start a gate in the FOREGROUND: cd <worktree> && scripts/ci/gate.sh (Bash timeout 600000).' '```'
lanetest fail "the gate as ./scripts/ci/gate.sh (no entry names it)" 'Run `./scripts/ci/gate.sh --quick`.'
printf 'Run `scripts/ci/gate.sh`.\n' > "$d/nolane.md"
lanes_at_template "$d/lane.json" "$d/nolane.md" | grep -q '^FAIL no instruction gives the lane form' \
  && ok "lane self-test: instructions with no -C form fail" || fail "lane self-test: instructions with no -C form passed"
jq '.sandbox.excludedCommands -= ["scripts/ci/gate.sh *"]' "$d/lane.json" > "$d/nostar.json"
printf 'Lane: `scripts/ci/gate.sh -C <worktree>`.\n' > "$d/lane.md"
n=$(lanes_at_template "$d/nostar.json" "$d/lane.md" | grep -c '^FAIL')
[ "$n" -gt 0 ] && ok "lane self-test: the lane form fails once \`scripts/ci/gate.sh *\` leaves the settings" || fail "lane self-test: the lane form passed with no \`scripts/ci/gate.sh *\` entry"

# The real instructions: the Markdown agents load, the playbook and the conventions. The skills and
# agents are the model's copy: the project's, or the plugin's (STRIDAL_FW).
FW=${STRIDAL_FW:-$ROOT/.claude}
set -- "$FW"/skills/*/*.md "$FW"/agents/*.md
for f in "$ROOT/AGENTS.md" "$ROOT/CLAUDE.md" "$ROOT/docs/playbook.md" "$ROOT/docs/conventions.md"; do [ -f "$f" ] && set -- "$@" "$f"; done
check_lane_gates "$st" "$@" > "$d/lanes"
cat "$d/lanes"; _fails=$((_fails + $(grep -c '^FAIL' "$d/lanes")))
finish
