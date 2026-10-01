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
# sandbox.network.allowedDomains merge across scopes), never by turning the sandbox off here. The keys are Claude Code's
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
Bash(tmux *kill-server*)'
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
  fi
}

# Self-test: the template's settings pass; each broken shape fails.
st="$ROOT/.claude/settings.json"
d=$(scratch)
selftest() {  # selftest WANT LABEL JQ-FILTER
  jq "$3" "$st" > "$d/s.json"
  n=$(check_settings "$d/s.json" | grep -c '^FAIL')
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

check_settings "$st" > "$d/real"
cat "$d/real"; _fails=$((_fails + $(grep -c '^FAIL' "$d/real")))
finish
