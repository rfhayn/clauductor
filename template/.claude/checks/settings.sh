#!/bin/sh
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
    # ALONE, bare or with arguments, so both forms are listed: the receipt runs the gate bare and
    # gate-wait takes no arguments. gate-wait.sh is where install put it (gate_wait_path, conf.sh):
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
finish
