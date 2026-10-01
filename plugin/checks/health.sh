#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# Health lines run per their own #! line (P1.8, B4): session-start runs them through
# `.claude/extensions.sh health`, which must never force a bash script through sh (dash on Ubuntu,
# where bash-only syntax is a syntax error and the line reads as a broken subject). Fixtures:
#   - a bash-only line (`[[ ]]`, and it refuses to run in POSIX mode, which is what bash invoked as
#     `sh` is, so the fixture fails on macOS too if it is forced through sh);
#   - a line whose #! names an interpreter only this check provides (proves the #! is honoured, on
#     any platform);
#   - a line with no #! (runs under sh), one whose interpreter is missing (CANNOT CHECK, by name),
#     and one that exits non-zero (CANNOT CHECK, never silence).
# Run with the host shell AND, where it is installed, under dash itself.
. "$(dirname "$0")/lib.sh"
need git bash

d=$(scratch)
R="$d/p"; new_repo "$R"
mkdir -p "$R/.claude/lib" "$R/.claude/health" "$d/bin"
cp "$CLAUDUCTOR_FW/lib/conf.sh" "$CLAUDUCTOR_FW/lib/modules.sh" "$R/.claude/lib/"
cp "$CLAUDUCTOR_FW/extensions.sh" "$R/.claude/"
cat > "$R/.claude/health/bashonly.sh" <<'EOF'
#!/usr/bin/env bash
if [[ -o posix ]]; then echo "FAILED forced through sh (bash in POSIX mode)"; exit 0; fi
arr=(one two); echo "OK bash-only line ran under bash (${#arr[@]} items)"
EOF
printf '#!/usr/bin/env fakeinterp\nanything\n' > "$R/.claude/health/fake.sh"
printf '#!/bin/sh\necho "fakeinterp ran $1"\n' > "$d/bin/fakeinterp"; chmod +x "$d/bin/fakeinterp"
printf 'echo "OK plain line"\n' > "$R/.claude/health/plain.sh"
printf '#!/usr/bin/env no-such-interpreter-here\necho never\n' > "$R/.claude/health/missing.sh"
printf 'echo "half a verdict"\nexit 4\n' > "$R/.claude/health/broken.sh"

for shell in sh dash; do
  command -v "$shell" >/dev/null 2>&1 || { ok "SKIPPED under $shell: not installed (CI's Ubuntu runner has it as sh)"; continue; }
  out=$(cd "$R" && PATH="$d/bin:$PATH" "$shell" .claude/extensions.sh health 2>&1)
  case "$out" in *"OK bash-only line ran under bash (2 items)"*) ok "[$shell] a bash health line runs under bash, not forced through sh" ;; *) fail "[$shell] bash-only line: $(printf '%s' "$out" | grep -A1 bashonly)" ;; esac
  case "$out" in *"fakeinterp ran $R/.claude/health/fake.sh"*) ok "[$shell] a line runs under the interpreter its #! names" ;; *) fail "[$shell] #! not honoured: $(printf '%s' "$out" | grep -A1 'Health: fake')" ;; esac
  case "$out" in *"OK plain line"*) ok "[$shell] a line with no #! runs under sh" ;; *) fail "[$shell] plain line: $out" ;; esac
  case "$out" in *"CANNOT CHECK — $R/.claude/health/missing.sh needs no-such-interpreter-here"*) ok "[$shell] a missing interpreter is CANNOT CHECK, by name" ;; *) fail "[$shell] missing interpreter: $(printf '%s' "$out" | grep -A1 'Health: missing')" ;; esac
  case "$out" in *"half a verdict"*"CANNOT CHECK — .claude/health/broken.sh exited 4"*) ok "[$shell] a line that exits non-zero keeps its output and says CANNOT CHECK" ;; *) fail "[$shell] broken line: $(printf '%s' "$out" | grep -A2 'Health: broken')" ;; esac
done
finish
