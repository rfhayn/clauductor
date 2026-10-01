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
finish
