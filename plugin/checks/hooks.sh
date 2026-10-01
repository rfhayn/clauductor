#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The PreToolUse and UserPromptSubmit hooks, as payload → exit-code tables, in BOTH directions.
# A guard falsified only in the blocking direction can block legitimate work; one falsified only
# in the allowing direction can block nothing and still look correct. Every row runs the real
# hook against a throwaway git repository, never this one, so the result is about the hook's
# contract ("is this path tracked?") rather than what this repo contains today.
. "$(dirname "$0")/lib.sh"
need git jq

H="$CLAUDUCTOR_FW/hooks"
d=$(scratch)

# ── syntax first: a hook that does not parse exits non-zero, which READS AS "blocked" and so
#    satisfies every BLOCK row below while blocking every Bash call in real use.
for h in "$H"/*.sh "$H"/lib/*.sh; do
  if sh -n "$h" 2>/dev/null; then ok "parses: $(basename "$h")"; else fail "does not parse: $h"; fi
done
# (A stray apostrophe inside one of no-blind-source-rewrite.sh's single-quoted awk programs
# either breaks the parse above or changes what awk runs, which the ALLOW and BLOCK rows below
# both catch.)

# ── no-blind-source-rewrite ─────────────────────────────────────────────────────────────
R="$d/repo"; new_repo "$R"
mkdir -p "$R/src/lib"
echo 'export const x = 1;' > "$R/src/lib/app.ts"
printf 'all:\n' > "$R/Makefile"
echo '# doc' > "$R/README.md"
git -C "$R" add src Makefile README.md && git -C "$R" commit -qm seed
echo 'scratch' > "$R/src/lib/scratch.ts"            # untracked, beside a tracked file
git -C "$R" worktree add -q "$d/wt" 2>/dev/null     # a second worktree of the same repo
T=src/lib/app.ts

nb() {  # nb WANT LABEL COMMAND [CWD]
  rc=0; payload "$3" "${4:-$R}" | (cd "${4:-$R}" && sh "$H/no-blind-source-rewrite.sh") >/dev/null 2>&1 || rc=$?
  expect_rc "$1" "$rc" "no-blind-source-rewrite $( [ "$1" = 2 ] && echo blocks || echo allows ): $2"
}
nb 2 "sed -i" "sed -i '' 's/old/new/' $T"
nb 2 "perl -pi" "perl -pi -e 's/old/new/' $T"
nb 2 "perl -0pi" "perl -0pi -e 's/old/new/' $T"
nb 2 "perl -i.bak" "perl -i.bak -pe 's/a/b/' $T"
nb 2 "sed -E -i" "sed -E -i '' 's/a/b/' $T"
nb 2 "sed --in-place" "sed --in-place 's/a/b/' $T"
nb 2 "ruby -0pi" "ruby -0pi -e 'gsub(/a/, \"b\")' $T"
nb 2 "gsed" "gsed -i s/a/b/ $T"
nb 2 "a path-qualified sed" "/usr/bin/sed -i '' 's/a/b/' $T"
nb 2 "-i after the script" "sed -e 's/a/b/' -i '' $T"
nb 2 "python read-modify-write" "python3 -c \"import pathlib; p=pathlib.Path('$T'); p.write_text(p.read_text().replace('a','b'))\""
nb 2 "python heredoc" "python3 <<'PY'
import pathlib
p = pathlib.Path('$T')
p.write_text(p.read_text().replace('a', 'b'))
PY"
nb 2 "an absolute path" "sed -i '' s/a/b/ $R/$T"
nb 2 "a tracked file with no extension" "sed -i '' s/a/b/ Makefile"
nb 2 "cd then a relative path" "cd src && sed -i '' s/a/b/ lib/app.ts"
nb 2 "a glob" "sed -i '' s/a/b/ src/lib/*.ts"
nb 2 "xargs (fails closed)" "git ls-files | xargs sed -i '' s/a/b/"
nb 2 "a variable (fails closed)" "F=$T; sed -i '' s/a/b/ \"\$F\""
nb 2 "command substitution (fails closed)" "sed -i '' s/a/b/ \$(git ls-files)"
nb 2 "sh -c wrapping a cd" "sh -c 'cd src && sed -i \"\" s/a/b/ lib/app.ts'"
nb 2 "env wrapper" "env LC_ALL=C sed -i '' s/a/b/ $T"
nb 2 "a path into another worktree" "sed -i '' s/a/b/ $d/wt/$T"
nb 0 "a read-only python preview" "python3 -c \"print(open('$T').read().replace('a','b'))\""
nb 0 "grep" "grep -rn foo $T"
nb 0 "sed without -i" "sed -E 's/old/new/' $T"
nb 0 "perl -ne" "perl -ne 'print if /x/' $T"
nb 0 "an untracked scratch file" "sed -i '' s/a/b/ src/lib/scratch.ts"
nb 0 "a file under /tmp" "sed -i '' s/a/b/ /tmp/checks-scratch.txt"
nb 0 "markdown" "sed -i '' s/a/b/ README.md"
nb 0 "a commit message naming the hazard" "git commit -m \"guard now catches sed -i and perl -0pi on $T\""
nb 0 "a commit heredoc naming the hazard" "git status --short && git commit -F - <<'MSG'
Python's .replace() rewrote $T; sed -i too
MSG"
nb 0 "a python heredoc writing only a doc" "python3 <<'PY'
import pathlib
body = '''mentions $T and .replace( and sed -i'''
pathlib.Path('README.md').write_text(body)
PY"
nb 0 "echo prose naming sed -i" "echo never use sed -i on source"
nb 0 "git log --grep for the spelling" "git log --oneline --grep 'sed -i'"
nb 0 "an empty command" ""
rc=0; printf '' | sh "$H/no-blind-source-rewrite.sh" >/dev/null 2>&1 || rc=$?
expect_rc 0 "$rc" "no-blind-source-rewrite allows an empty payload rather than breaking every Bash call"

# Without jq it must fail CLOSED for a command it polices, and stay open for anything else.
mkdir -p "$d/nojq"
for t in sh cat git awk sed grep tr sort cut xargs mktemp dirname basename wc head tail pwd printf; do
  p=$(command -v "$t" 2>/dev/null) && ln -sf "$p" "$d/nojq/$t"
done
nojq() {
  rc=0; payload "$2" "$R" | (cd "$R" && PATH="$d/nojq" "$d/nojq/sh" "$H/no-blind-source-rewrite.sh") >/dev/null 2>&1 || rc=$?
  expect_rc "$1" "$rc" "no-blind-source-rewrite without jq: $3"
}
nojq 2 "sed -i '' s/a/b/ $T" "blocks a command it could police"
nojq 0 "git status" "allows a command it could not"

# ── worktree-hook-drift ─────────────────────────────────────────────────────────────────
git init -q --bare "$d/origin.git"
A="$d/main"; new_repo "$A"
mkdir -p "$A/.claude/hooks" "$A/.claude/lib"
cp "$H/worktree-hook-drift.sh" "$A/.claude/hooks/"
cp "$CLAUDUCTOR_FW/lib/conf.sh" "$A/.claude/lib/"
echo '{}' > "$A/.claude/settings.json"
git -C "$A" add .claude && git -C "$A" commit -qm hooks
git -C "$A" remote add origin "$d/origin.git"
git -C "$A" push -q origin HEAD:main 2>/dev/null
git -C "$A" fetch -q origin

spawn='{"tool_name":"Agent","tool_input":{"prompt":"x","isolation":"worktree"}}'
plain='{"tool_name":"Agent","tool_input":{"prompt":"mentions \"isolation\": \"worktree\""}}'
drift() {  # drift WANT LABEL PAYLOAD
  out=$(printf '%s' "$3" | sh "$A/.claude/hooks/worktree-hook-drift.sh" 2>/dev/null); rc=$?
  expect_rc "$1" "$rc" "worktree-hook-drift: $2"
}
drift 0 "hooks match origin/main: allowed" "$spawn"

# -b: the bare origin's HEAD follows the runner's init.defaultBranch (master on CI), not main, so a
# plain clone checks out nothing and the commit below silently never reaches origin.
B="$d/other"; git clone -q -b main "$d/origin.git" "$B" 2>/dev/null
git -C "$B" config user.email c@example.com; git -C "$B" config user.name c
echo '# a fix merged elsewhere' >> "$B/.claude/hooks/worktree-hook-drift.sh"
git -C "$B" commit -qam "fix a hook" && git -C "$B" push -q origin HEAD:main 2>/dev/null
git -C "$A" fetch -q origin
[ "$(git -C "$A" rev-parse HEAD)" != "$(git -C "$A" rev-parse origin/main)" ] && ok "fixture: origin/main has a hook commit the main checkout lacks" || fail "fixture: the hook fix never reached origin/main; the drift case below learns nothing"
drift 2 "main checkout lacks a merged hook commit: blocked" "$spawn"
drift 0 "a spawn without worktree isolation: allowed" "$plain"

git -C "$A" merge -q --ff-only origin/main
echo '# my unmerged hook work' >> "$A/.claude/hooks/worktree-hook-drift.sh"
out=$(printf '%s' "$spawn" | sh "$A/.claude/hooks/worktree-hook-drift.sh" 2>/dev/null); rc=$?
expect_rc 0 "$rc" "worktree-hook-drift: a checkout only AHEAD of origin/main: allowed"
case "$out" in *additionalContext*) ok "worktree-hook-drift: ...with a note on the channel that reaches Claude" ;; *) fail "worktree-hook-drift: ahead-only gave no additionalContext note" ;; esac

# ── focus-staleness ────────────────────────────────────────────────────────────────────
FH="$d/home"; mkdir -p "$FH"
fs() { (cd "$R" && HOME="$FH" sh "$H/focus-staleness.sh" </dev/null); }
case "$(fs)" in *"focus may be stale"*) ok "focus-staleness nudges when no focus is set" ;; *) fail "focus-staleness: no nudge with no focus file" ;; esac
(cd "$R" && HOME="$FH" sh "$CLAUDUCTOR_FW/status-write.sh" "fresh focus" >/dev/null)
case "$(fs)" in "") ok "focus-staleness is silent right after status-write" ;; *) fail "focus-staleness nudged on a fresh focus: $(fs)" ;; esac
case "$(cd "$R" && HOME="$FH" FOCUS_STALE_SECONDS=-1 sh "$H/focus-staleness.sh" </dev/null)" in *"stale"*) ok "focus-staleness nudges past the age limit" ;; *) fail "focus-staleness: no nudge past the age limit" ;; esac

# ── format ─────────────────────────────────────────────────────────────────────────────
rc=0; printf '{"tool_input":{"file_path":"%s"}}' "$R/$T" | sh "$H/format.sh" >/dev/null 2>&1 || rc=$?
expect_rc 0 "$rc" "format.sh is a silent no-op with FORMAT_CMD unset"

# ── conf.sh missing (B13) ──────────────────────────────────────────────────────────────
# A `.` of a missing file exits 2 under dash, which Claude Code reads as a BLOCK: every hook that
# reads the config tests for it first. Run each hook from a checkout with no .claude/lib, under sh
# and, where installed, under dash (Ubuntu's sh).
N="$d/noconf"; new_repo "$N"; mkdir -p "$N/.claude/hooks"
for h in focus-staleness format worktree-hook-drift; do cp "$H/$h.sh" "$N/.claude/hooks/"; done
for shell in sh dash; do
  command -v "$shell" >/dev/null 2>&1 || continue
  for h in focus-staleness format worktree-hook-drift; do
    rc=0; printf '%s' "$spawn" | (cd "$N" && "$shell" "$N/.claude/hooks/$h.sh") >/dev/null 2>&1 || rc=$?
    expect_rc 0 "$rc" "[$shell] $h.sh with conf.sh missing does not block (exit 0)"
  done
done
for h in "$H"/*.sh; do
  grep -q 'lib/conf.sh"$' "$h" || continue
  awk '/^[[:space:]]*\. .*lib\/conf\.sh"$/ { if (!guarded) bad = 1 } /-f .*lib\/conf\.sh"/ { guarded = 1 } END { exit bad }' "$h" \
    && ok "$(basename "$h") tests for conf.sh before sourcing it" || fail "$(basename "$h") sources conf.sh without testing it exists first (a dash exit 2 reads as a block)"
done

# ── Registration: every hook settings.json registers LAUNCHES, from a subdirectory too ──────────
# Every row above runs a hook by its absolute path, so it tests the HOOK and never the
# REGISTRATION. Claude Code runs a command hook in the session's CURRENT directory, which follows
# its `cd`: a hook registered by a relative path (`sh .claude/hooks/x.sh`) fails to launch from a
# subdirectory, and an exit that is neither 0 nor 2 does not block, so the guard fails OPEN and
# says nothing (Standing Tee #322: the merge guard was off this way). So this reads the
# registration from settings.json itself (the authority, not a list typed here), runs each command
# string as Claude Code does (`sh -c` with CLAUDE_PROJECT_DIR set) from a subdirectory and from the
# root of a throwaway copy, and asks each for its OWN proof that it ran: exit code alone is not it,
# since dash exits 2 when it cannot open a script, the very code a guard blocks with.
#
# THE TABLE: one row per registered hook script (script, exit, proof, payload, why), and a row for
# a script that is not registered fails too, so a new hook cannot be registered without saying how
# it proves it launched. A project that registers its own hook adds its row to
# .claude/local/hook-expectations.tsv, the same five tab-separated columns. proof is `out:<ERE>`
# (stdout and stderr together), `file:<path>` (it must exist after) or `exit` (the exit code and no
# launch error: only for a hook with nothing else to show). @REPO@ in a payload or path is the
# throwaway project; @TRACKED@ a tracked file in it.
SETTINGS="$ROOT/.claude/settings.json"
if [ ! -f "$SETTINGS" ]; then
  fail "no .claude/settings.json: the hooks are registered nowhere, so none of them runs"
  finish
fi
regs=$(jq -r '(.hooks // {}) | to_entries[] | .key as $e | .value[] | .hooks[]? | select(.type == "command") | [$e, .command] | @tsv' "$SETTINGS" 2>/dev/null) \
  || { fail "settings.json does not parse, so no hook it registers can be checked (or run)"; finish; }
sl=$(jq -r '.statusLine.command // empty' "$SETTINGS")
# Every .claude/ path, and every script path, in a hook or status-line command goes through
# CLAUDE_PROJECT_DIR: `sh scripts/hooks/guard.sh` fails to launch from a subdirectory just the same.
SCRIPT_ERE='[A-Za-z0-9_./${}"-]*[A-Za-z0-9_.-]+\.(sh|bash|js|mjs|cjs|py|rb|ts)'
relative() {  # relative CMD: prints CMD if a .claude/ or script path in it is not reached through CLAUDE_PROJECT_DIR
  { printf '%s\n' "$1" | grep -oE '[^[:space:]]*\.claude/' | sed 's|\.claude/$||'
    printf '%s\n' "$1" | grep -oE "(^|[[:space:]])$SCRIPT_ERE" | sed 's/^[[:space:]]*//; s|[^/]*$||'; } \
  | while IFS= read -r pre; do
    case $pre in /*) continue ;; esac  # an absolute path launches from anywhere
    printf '%s\n' "$pre" | grep -qE '^"?\$\{?CLAUDE_PROJECT_DIR(:-[^}]*)?\}?"?/' || { printf '%s\n' "$1"; break; }
  done
}
printf '%s\n%s\n' "$(printf '%s\n' "$regs" | cut -f2)" "$sl" | grep -v '^$' > "$d/cmds"
bad=$(while IFS= read -r cmd; do relative "$cmd"; done < "$d/cmds" | sort -u)
[ -z "$bad" ] && ok "every .claude/ path a hook or the status line names goes through \$CLAUDE_PROJECT_DIR" \
  || printf '%s\n' "$bad" | while IFS= read -r b; do echo "FAIL registered relative to the session's cwd, so it cannot launch from a subdirectory: $b"; done
[ -z "$bad" ] || _fails=$((_fails + 1))

TAB=$(printf '\t')
# Installed as a plugin, the model's own hooks are registered by the plugin (its hooks/hooks.json,
# which clauductor's packager generates from the template's settings.json and tests): this
# project's settings.json then registers only the project's own hooks, held to their local rows.
plugin=""
[ -n "${CLAUDUCTOR_FW:-}" ] && [ -f "$CLAUDUCTOR_FW/hooks/hooks.json" ] && plugin=1
cat > "$d/core.tsv" <<'EOF'
pr-merge-guard.sh	2	out:BLOCKED by pr-merge-guard: '--auto' merges before checks settle	{"tool_name":"Bash","tool_input":{"command":"gh pr merge 1 --auto"}}	rule 1 refuses --auto before it asks GitHub anything
no-blind-source-rewrite.sh	2	out:BLOCKED by no-blind-source-rewrite	{"tool_name":"Bash","tool_input":{"command":"sed -i '' 's/a/b/' @TRACKED@"}}	an in-place sed on a tracked file
worktree-hook-drift.sh	0	out:worktree-hook-drift: CANNOT CHECK	{"tool_name":"Agent","tool_input":{"prompt":"p","isolation":"worktree"}}	a worktree spawn with no origin/main to compare: allowed, and says so
focus-staleness.sh	0	out:focus may be stale	{"prompt":"hello"}	no focus file yet: it nudges
format.sh	0	file:@REPO@/formatted.stamp	{"tool_name":"Write","tool_input":{"file_path":"@TRACKED@"}}	FORMAT_CMD runs on the file just written
EOF
if [ -n "$plugin" ]; then : > "$d/expect.tsv"; else cp "$d/core.tsv" "$d/expect.tsv"; fi
[ -f "$ROOT/.claude/local/hook-expectations.tsv" ] && grep -v '^[[:space:]]*#' "$ROOT/.claude/local/hook-expectations.tsv" | grep -v '^[[:space:]]*$' >> "$d/expect.tsv"

# The throwaway project: a COPY of this project's .claude (the real bytes), one tracked file, a
# formatter that leaves a stamp, and no origin.
P="$d/reg"; new_repo "$P"; mkdir -p "$P/.claude" "$P/src/sub" "$P/home"
for e in "$ROOT"/.claude/* "$ROOT"/.claude/.[!.]*; do
  [ -e "$e" ] || continue
  case $(basename "$e") in worktrees|settings.local.json) continue ;; esac
  cp -R "$e" "$P/.claude/"
done
echo 'export const x = 1;' > "$P/src/tracked.ts"
git -C "$P" add src/tracked.ts && git -C "$P" commit -qm seed
P=$(cd "$P" && pwd -P)
printf '#!/bin/sh\ntouch "%s/formatted.stamp"\n' "$P" > "$P/fmt.sh"
{ cat "$ROOT/.claude/project.conf" 2>/dev/null; printf '\nFORMAT_CMD="sh %s/fmt.sh"\nFORMAT_EXT="ts"\n' "$P"; } > "$P/.claude/project.conf"

# Every registered command that runs a script file, wherever it lives and whatever its language.
launching=$(printf '%s\n' "$regs" | grep -E "(^|[[:space:]])$SCRIPT_ERE" || true)
n=$(printf '%s\n' "$launching" | grep -c . || true)
if [ -n "$plugin" ]; then ok "the model's hooks are the plugin's ($(jq '[.hooks[][].hooks[]] | length' "$CLAUDUCTOR_FW/hooks/hooks.json") in its hooks.json); settings.json registers $n of this project's own"
else [ "$n" -ge 3 ] && ok "settings.json registers $n hook command(s) (non-vacuity)" || fail "settings.json registers only $n hook command(s): the table below would pass vacuously"; fi
script_of() { printf '%s\n' "$1" | grep -oE "(^|[[:space:]])$SCRIPT_ERE" | head -1 | sed 's/^[[:space:]]*//; s|.*/||'; }
printf '%s\n' "$launching" | while IFS="$TAB" read -r ev cmd; do [ -n "$cmd" ] && script_of "$cmd"; done | sort -u > "$d/registered"
cut -f1 "$d/expect.tsv" | sort -u > "$d/rows"
for s in $(comm -23 "$d/registered" "$d/rows"); do fail "$s is registered in settings.json but has no expectation row (add one: checks/hooks.sh, or .claude/local/hook-expectations.tsv)"; done
for s in $(comm -13 "$d/registered" "$d/rows"); do fail "$s has an expectation row but settings.json does not register it (a guard that is never registered guards nothing)"; done
[ -z "$(comm -3 "$d/registered" "$d/rows")" ] && ok "every registered hook script has exactly one expectation row: $(tr '\n' ' ' < "$d/registered")"

printf '%s\n' "$launching" > "$d/launching"
while IFS="$TAB" read -r ev cmd; do
  [ -n "$cmd" ] || continue
  s=$(script_of "$cmd")
  row=$(awk -F'\t' -v s="$s" '$1 == s { print; exit }' "$d/expect.tsv")
  [ -n "$row" ] || continue
  want=$(printf '%s' "$row" | cut -f2); proof=$(printf '%s' "$row" | cut -f3)
  pl=$(printf '%s' "$row" | cut -f4 | sed "s|@TRACKED@|$P/src/tracked.ts|g; s|@REPO@|$P|g"); why=$(printf '%s' "$row" | cut -f5)
  for where in "$P/src/sub" "$P"; do
    label="$ev $s from $( [ "$where" = "$P" ] && echo 'the project root' || echo 'a subdirectory'): $why"
    rm -f "$P/formatted.stamp"
    rc=0; out=$(cd "$where" && printf '%s' "$pl" | jq -c --arg c "$where" '. + {cwd: $c}' | CLAUDE_PROJECT_DIR="$P" HOME="$P/home" sh -c "$cmd" 2>&1) || rc=$?
    if printf '%s' "$out" | grep -qE 'No such file|cannot open|not found'; then fail "$label: it did not launch: $(printf '%s' "$out" | head -2 | tr '\n' ' ')"; continue; fi
    [ "$rc" = "$want" ] || { fail "$label: exit $rc, want $want ($(printf '%s' "$out" | head -2 | tr '\n' ' '))"; continue; }
    case $proof in
      out:*) printf '%s' "$out" | grep -qE -- "${proof#out:}" && ok "$label" || fail "$label: exit $rc, but not its own proof '${proof#out:}' (it may never have run): $(printf '%s' "$out" | head -2 | tr '\n' ' ')" ;;
      file:*) f=$(printf '%s' "${proof#file:}" | sed "s|@REPO@|$P|g"); [ -e "$f" ] && ok "$label" || fail "$label: exit $rc, but ${proof#file:} was not made (it may never have run)" ;;
      exit) ok "$label (exit code only)" ;;
      *) fail "$s: proof '$proof' is not out:<ERE>, file:<path> or exit" ;;
    esac
  done
done < "$d/launching"

# The detector itself, both ways, on fixture commands (so a clean pass above is not vacuous).
# Fixture commands, spelled through $D so no line here is itself a path this check uses.
D=.claude
for c in "sh $D/hooks/x.sh" "bash ./$D/hooks/x.sh" "sh \$HOME/p/$D/hooks/x.sh" "sh scripts/hooks/guard.sh" "sh guard.sh"; do
  [ -n "$(relative "$c")" ] && ok "the path rule flags '$c'" || fail "the path rule passed '$c', which launches only from the root"
done
for c in "sh \"\$CLAUDE_PROJECT_DIR\"/$D/hooks/x.sh" "sh \"\${CLAUDE_PROJECT_DIR:-.}\"/$D/statusline.sh" "sh \$CLAUDE_PROJECT_DIR/$D/x.sh" "node \"\$CLAUDE_PROJECT_DIR\"/scripts/hooks/x.mjs" "sh /opt/hooks/x.sh" "npx biome format --write"; do
  [ -z "$(relative "$c")" ] && ok "the path rule passes '$c'" || fail "the path rule flagged '$c', which is correct"
done
# The table covers every registered script, not only .claude/hooks/*.sh.
[ "$(script_of "node \"\$CLAUDE_PROJECT_DIR\"/scripts/hooks/x.mjs --flag")" = x.mjs ] && [ -z "$(script_of 'npx biome format --write')" ] \
  && ok "a registered script in any directory or language gets a row of its own (x.mjs); an inline command needs none" || fail "script_of: '$(script_of "node \"\$CLAUDE_PROJECT_DIR\"/scripts/hooks/x.mjs")'"
finish
