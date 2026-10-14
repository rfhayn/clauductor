#!/bin/sh
# The PreToolUse and UserPromptSubmit hooks, as payload → exit-code tables, in BOTH directions.
# A guard falsified only in the blocking direction can block legitimate work; one falsified only
# in the allowing direction can block nothing and still look correct. Every row runs the real
# hook against a throwaway git repository, never this one, so the result is about the hook's
# contract ("is this path tracked?") rather than what this repo contains today.
. "$(dirname "$0")/lib.sh"
. "$ROOT/.claude/lib/refs.sh"
need git jq

H="$ROOT/.claude/hooks"
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
mkdir -p "$R/tools"
cat > "$R/tools/codemod.py" <<'PY'
import pathlib
p = pathlib.Path('src/lib/app.ts')
p.write_text(p.read_text().replace('x', 'y'))
PY
echo 'dist/' > "$R/.gitignore"
for p in a b c; do mkdir -p "$R/packages/$p/src"; echo 'export const x = 1;' > "$R/packages/$p/src/index.ts"; done
git -C "$R" add src tools packages Makefile README.md .gitignore && git -C "$R" commit -qm seed
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

# A SCRIPT FILE is code too (#88): the command names only the file, and the rewrite is inside it.
# Untracked scripts in the repository or a temp dir, each the same hazard as an inline row above.
cp "$R/tools/codemod.py" "$R/rewrite.py"
sed "s|src/lib/app.ts|$R/$T|" "$R/rewrite.py" > "$d/rewrite-tmp.py"
{ printf '#!/usr/bin/env python3\n'; cat "$R/rewrite.py"; } > "$R/fix.py"; chmod +x "$R/fix.py"
cat > "$R/resub.py" <<'PY'
import re
s = open('src/lib/app.ts').read()
open('src/lib/app.ts', 'w').write(re.sub(r'x', 'y', s))
PY
cat > "$R/fix.js" <<'JS'
const fs = require('fs');
const f = 'src/lib/app.ts';
fs.writeFileSync(f, fs.readFileSync(f, 'utf8').replace('x', 'y'));
JS
printf "sed -i '' s/x/y/ src/lib/app.ts\n" > "$R/sub.sh"
# -0pi, not -pi: the literal `perl -pi` floor would catch -pi with no file read at all.
printf '#!/usr/bin/perl -0pi\ns/x/y/;\n' > "$R/x.pl"
cat > "$R/preview.py" <<'PY'
print(open('src/lib/app.ts').read().replace('x', 'y'))
PY
sed 's|src/lib/app.ts|src/lib/scratch.ts|' "$R/rewrite.py" > "$R/scratch.py"
{ cat "$R/fix.js"; awk 'BEGIN { for (i = 0; i < 2000; i++) print "// padding padding padding padding padding padding padding" }'; } > "$R/big.js"
nb 2 "python3 on a script file" "python3 rewrite.py"
nb 2 "a script file in a temp dir" "python3 $d/rewrite-tmp.py"
nb 2 "a script run directly" "./fix.py"
nb 2 "a script run through env" "env python3 rewrite.py"
nb 2 "re.sub in a script file" "python3 -u resub.py"
nb 2 "node on a script file" "node fix.js"
nb 2 "sh on a script file" "sh sub.sh"
nb 2 "perl with the script's own -pi switches" "perl x.pl $T"
nb 0 "a script file that only reads" "python3 preview.py"
nb 0 "a script file rewriting an untracked file" "python3 scratch.py"
nb 0 "a TRACKED script (reviewed code, not a throwaway)" "python3 tools/codemod.py"
nb 0 "a script file that does not exist" "python3 missing.py"
nb 0 "grep reading a script file" "grep -n replace rewrite.py"
nb 0 "a script file over the size bound (a program, not a throwaway)" "node big.js"
# Other ways a shell or an interpreter is handed the file.
nb 2 "a script sourced with ." ". ./sub.sh"
nb 2 "a script sourced with source" "source sub.sh"
nb 2 "a script on stdin by redirect" "python3 < rewrite.py"
nb 2 "a script on stdin as -" "python3 - < rewrite.py"
nb 2 "a script piped in by cat" "cat rewrite.py | python3"
nb 2 "python3 -m running a module in the cwd" "python3 -m rewrite"
# A check-only run executes nothing.
nb 0 "bash -n" "bash -n sub.sh"
nb 0 "sh -n" "sh -n sub.sh"
nb 0 "node --check" "node --check fix.js"
nb 0 "node -c" "node -c fix.js"
nb 0 "perl -c" "perl -c x.pl $T"
# Only a literal or glob target that resolves to a TRACKED file is judged in a script's text:
# what the script cannot name (a variable, a cd to one) is not refused, unlike on the command line.
printf 'm=$(mktemp -d)\ncp src/lib/app.ts "$m/app.ts"\nsed -i "" s/x/y/ "$m/app.ts"\n' > "$d/mut.sh"
printf 'sed -i "" "s/^version=.*/version=$1/" "$VERSION_FILE"\n' > "$R/bump.sh"
printf 'PROFILE="$HOME/.profile"\ncd "$HOME"\nsed -i "" /nvm/d "$PROFILE"\n' > "$d/install.sh"
nb 0 "a scratch mutation script editing a copy" "sh $d/mut.sh"
nb 0 "a script editing a file its argument names" "sh bump.sh 1.2"
nb 0 "a script editing a file under a variable cd" "sh $d/install.sh"
# Ignored build output is a program, not a throwaway script.
mkdir -p "$R/dist"; cp "$R/fix.js" "$R/dist/cli.js"
nb 0 "gitignored build output" "node dist/cli.js"
nb 0 "gitignored build output through npm exec" "npm exec -- node dist/cli.js"
# An editor shelled out to inside a string: the shell scan reads the string as one word, so the
# paths the script names decide, as for a .replace( script.
cat > "$R/shellout.py" <<'PY'
import os
os.system("sed -i '' s/x/y/ src/lib/app.ts")
PY
cat > "$R/shellout.js" <<'JS'
require('child_process').execSync("sed -i '' s/x/y/ src/lib/app.ts");
JS
nb 2 "a python script shelling out to sed -i" "python3 shellout.py"
nb 2 "a node script shelling out to sed -i" "node shellout.js"
# A script's cds compose with the command's only while they fit the 32-base cap; past it, each
# resolves against the command's directories alone, and nothing is refused for the script's cds.
cds='cd a\ncd b\ncd c\ncd d\ncd e\ncd f\n'
printf "${cds}sed -i '' s/x/y/ \"\$TMPDIR/notes.txt\"\n" > "$d/cds.sh"
printf "${cds}sed -i '' s/x/y/ $R/$T\n" > "$d/cds-abs.sh"
nb 0 "a script with many cds editing a temp file" "sh $d/cds.sh"
nb 2 "a script with many cds editing tracked source by absolute path" "sh $d/cds-abs.sh"
# python3 -m pkg runs pkg/__main__.py.
mkdir -p "$R/pkg"; cp "$R/rewrite.py" "$R/pkg/__main__.py"
nb 2 "python3 -m running a package in the cwd" "python3 -m pkg"
# Only a lone file piped by a single |: these are the cat-pipe rule's edges, both known limits.
nb 0 "cat of two files piped in (a known limit)" "cat preview.py rewrite.py | python3"
nb 0 "cat then || (not a pipe)" "cat rewrite.py || python3"
# A flag cluster ending in o takes the next word as its value.
nb 2 "bash -euo pipefail on a script file" "bash -euo pipefail sub.sh"
nb 2 "bash -euo pipefail -c" "bash -euo pipefail -c \"cd src && sed -i '' s/a/b/ lib/app.ts\""
nb 2 "bash --rcfile FILE on a script file" "bash --rcfile /dev/null sub.sh"
# A script that only MENTIONS an in-place editor (a comment, a grep pattern) is not one that runs
# it from a string: the editor-in-a-string rule needs a non-shell script with an exec call.
printf 'grep -rn "sed -i" src/lib/app.ts\n' > "$R/greps.sh"
printf '# sed -i is banned here\ncp src/lib/app.ts "$TMPDIR/app.ts"\n' > "$R/copies.sh"
printf "# never use sed -i on source\nprint(open('src/lib/app.ts').read())\n" > "$R/reads.py"
nb 0 "a shell script grepping for sed -i in tracked source" "sh greps.sh"
nb 0 "a shell script copying tracked source under a sed -i comment" "sh copies.sh"
nb 0 "a python script reading tracked source under a sed -i comment" "python3 reads.py"
printf 'exec > "$TMPDIR/harness.log"\ngrep -n "sed -i" src/lib/app.ts\n' > "$R/harness.sh"
nb 0 "a shell script with an exec, grepping for sed -i in tracked source" "sh harness.sh"
# The same harness is a shell script however it is started: by its path, sourced, or by a perl
# that hands a non-perl shebang to the interpreter it names.
nb 0 "the harness run by its path" "./harness.sh"
nb 0 "the harness sourced" ". ./harness.sh"
{ printf '#!/bin/sh\n'; cat "$R/harness.sh"; } > "$R/shtool"
nb 0 "a shell-shebang harness run by perl" "perl shtool"
# An editor the script's own scan sees (backticks) is judged by its operands, not by the paths
# the script names: this one edits only a temp file, though it reads tracked source.
cat > "$R/ticks.pl" <<'PL'
open(my $f, '<', 'src/lib/app.ts'); print <$f>;
system("true");
`sed -i '' s/x/y/ "$ENV{TMPDIR}/notes.txt"`;
PL
nb 0 "a perl script whose backtick sed -i edits a temp file" "perl ticks.pl"
nb 2 "bash --rcfile FILE -c" "bash --rcfile /dev/null -c \"cd src && sed -i '' s/a/b/ lib/app.ts\""
# A multi-package codemod: its cds pass the cap when composed, and each one still resolves.
printf "cd packages/%s && sed -i '' s/x/y/ src/index.ts && cd ../..\n" a b c > "$R/codemod.sh"
nb 2 "a multi-package codemod script" "sh codemod.sh"
# Past the cap, each script cd still resolves against the command's directories (linear).
printf "${cds}cd packages/a && sed -i '' s/x/y/ src/index.ts\n" > "$R/pastcap.sh"
nb 2 "a script past the cd cap editing tracked source by a relative path" "sh pastcap.sh"
# Collapsing x/.. keeps composed cds under the cap: packages/c is reached only by composing two.
printf "cd packages && cd a && cd .. && cd b && cd .. && cd c && sed -i '' s/x/y/ src/index.ts\n" > "$R/compose.sh"
nb 2 "a script whose cds compose under the cap once .. collapses" "sh compose.sh"
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
nojq 2 "python3 rewrite.py" "blocks a script-file run it could police"
nojq 0 "git status" "allows a command it could not"

# ── no-background-gate ──────────────────────────────────────────────────────────────────
G=scripts/ci/gate.sh
bgp() {  # bgp AGENT COMMAND [BACKGROUND] [TIMEOUT]: a Bash payload, a subagent's when AGENT is set
  jq -cn --arg a "$1" --arg c "$2" --arg b "${3:-}" --arg t "${4:-}" '{tool_name:"Bash", tool_input:{command:$c}}
    | if $a == "" then . else .agent_id = $a end
    | if $b == "" then . else .tool_input.run_in_background = ($b == "true") end
    | if $t == "" then . else .tool_input.timeout = ($t | tonumber) end'
}
nbg() {  # nbg WANT LABEL PAYLOAD [PATH]
  rc=0; printf '%s' "$3" | (cd "$R" && PATH="${4:-$PATH}" sh "$H/no-background-gate.sh") >/dev/null 2>&1 || rc=$?
  expect_rc "$1" "$rc" "no-background-gate $( [ "$1" = 2 ] && echo blocks || echo allows ): $2"
}
nbg 2 "a subagent's gate with run_in_background" "$(bgp a1 "$G" true 600000)"
nbg 2 "a subagent's foreground gate at the default timeout (the tool backgrounds it at 120 s)" "$(bgp a1 "$G")"
nbg 2 "a subagent's foreground gate under 600000 ms" "$(bgp a1 "$G --quick" false 300000)"
nbg 2 "a subagent's gate-wait with run_in_background" "$(bgp a1 "scripts/ci/gate-wait.sh" true 600000)"
nbg 2 "a subagent's run-local.sh behind cd, bash and env" "$(bgp a1 "cd sub && env X=1 bash ../scripts/ci/run-local.sh --quick")"
nbg 2 "a subagent's gate inside \$( )" "$(bgp a1 "out=\$($G); echo \"\$out\"")"
nbg 2 "the shell's & from a subagent, at timeout 600000" "$(bgp a1 "$G > log 2>&1 &" "" 600000)"
nbg 2 "the shell's & from the MAIN session (nothing waits for it)" "$(bgp "" "nohup $G --quick > log 2>&1 &")"
nbg 2 "( gate ) &" "$(bgp "" "( $G ) &")"
nbg 2 "bash -c 'gate &'" "$(bgp "" "bash -c '$G &'")"
nbg 0 "the main session's gate with run_in_background (it is woken when it ends)" "$(bgp "" "$G" true)"
nbg 0 "the main session's foreground gate at the default timeout" "$(bgp "" "$G")"
nbg 0 "a subagent's foreground gate at timeout 600000" "$(bgp a1 "$G --quick 2>&1 | tail -5" false 600000)"
nbg 0 "a subagent's foreground gate-wait at timeout 600000" "$(bgp a1 "scripts/ci/gate-wait.sh" "" 600000)"
nbg 0 "a subagent reading the gate in the background (cat)" "$(bgp a1 "cat $G" true)"
nbg 0 "a subagent's git log naming run-local.sh" "$(bgp a1 "git log --oneline -- scripts/ci/run-local.sh")"
nbg 0 "a commit message naming the gate and &" "$(bgp a1 "git commit -m \"run $G & wait for it\"")"
nbg 0 "a heredoc body naming the gate and &" "$(bgp a1 "cat > notes.txt <<'EOF'
$G &
EOF")"
nbg 0 "a syntax check of the gate (sh -n)" "$(bgp a1 "sh -n scripts/ci/gate-wait.sh")"
nbg 0 "a subagent's other background task" "$(bgp a1 "go test ./..." true)"
nbg 0 "a subagent's & on a command that is not a gate" "$(bgp a1 "sleep 1 & wait")"
# A gate is the configured PATH, not any script that shares its file name.
# Fixture commands, spelled through $CK so no line here is itself a path this check uses.
CK=.claude/checks
nbg 0 "a subagent's check script that is also named gate.sh" "$(bgp a1 "sh $CK/gate.sh")"
nbg 0 "the template's check named gate.sh" "$(bgp a1 "sh template/$CK/gate.sh")"
nbg 0 "the plugin's check named gate.sh, with &" "$(bgp "" "plugin/checks/gate.sh &")"
nbg 2 "the gate by an absolute path" "$(bgp a1 "\"\$CLAUDE_PROJECT_DIR\"/$G" true)"
nbg 2 "the gate as ./scripts/ci/gate.sh" "$(bgp a1 "./$G --quick")"
# Arithmetic is not a background job, and a heredoc inside \$( ) does not end it early.
nbg 0 "\$((3 & 1)) beside a foreground gate at 600000" "$(bgp a1 "echo \$((3 & 1)); $G" "" 600000)"
nbg 0 "a heredoc inside \$( ) whose body has ')' and then names the gate with &" "$(bgp "" "x=\$(cat <<'EOF'
)
$G &
EOF
)")"
# Without a working awk reader or jq, the shell's & still counts.
nbg 2 "without jq: the main session's gate with &" "$(bgp "" "$G &")" "$d/nojq"
nbg 0 "an empty payload" ""
# The gate is the project's GATE and GATE_RUN (project.conf, read from CLAUDE_PROJECT_DIR as the plugin does).
mkdir -p "$d/own-gate/.claude"; printf 'GATE="tools/check.sh"\nGATE_RUN="tools/run-all.sh"\n' > "$d/own-gate/.claude/project.conf"
for c in "tools/check.sh" "tools/run-all.sh --quick" "tools/gate-wait.sh"; do
  rc=0; bgp a1 "$c" true | (cd "$R" && CLAUDE_PROJECT_DIR="$d/own-gate" sh "$H/no-background-gate.sh") >/dev/null 2>&1 || rc=$?
  expect_rc 2 "$rc" "no-background-gate blocks a subagent's background '$c' where project.conf names tools/check.sh and tools/run-all.sh"
done
# A project.conf that names its gate through $ROOT (an absolute path) still matches the relative spelling.
mkdir -p "$d/abs-gate/.claude"; printf 'GATE="$ROOT/tools/check.sh"\nGATE_RUN="$ROOT/tools/run-all.sh"\n' > "$d/abs-gate/.claude/project.conf"
rc=0; bgp "" "tools/check.sh &" | (cd "$R" && CLAUDE_PROJECT_DIR="$d/abs-gate" sh "$H/no-background-gate.sh") >/dev/null 2>&1 || rc=$?
expect_rc 2 "$rc" "no-background-gate blocks 'tools/check.sh &' where project.conf names GATE=\"\$ROOT/tools/check.sh\""
# A substitution that starts with a subshell is not arithmetic; a path with // or /./ is the same path.
nbg 2 "\$((gate) 2>&1): a command substitution of a subshell, not arithmetic" "$(bgp a1 "out=\$(($G --quick) 2>&1)")"
nbg 2 "scripts/ci//gate.sh &" "$(bgp "" "scripts/ci//gate.sh &")"
nbg 2 "scripts/ci/./gate.sh &" "$(bgp "" "scripts/ci/./gate.sh &")"
# Without jq it fails CLOSED for a subagent call naming a gate, and only then.
nbg 2 "without jq: a subagent call naming the gate" "$(bgp a1 "$G" "" 600000)" "$d/nojq"
nbg 0 "without jq: the main session's gate" "$(bgp "" "$G" true)" "$d/nojq"
nbg 0 "without jq: a subagent call naming no gate" "$(bgp a1 "git status" true)" "$d/nojq"

# ── worktree-hook-drift ─────────────────────────────────────────────────────────────────
git init -q --bare "$d/origin.git"
A="$d/main"; new_repo "$A"
mkdir -p "$A/.claude/hooks" "$A/.claude/lib"
cp "$H/worktree-hook-drift.sh" "$A/.claude/hooks/"
cp "$ROOT/.claude/lib/conf.sh" "$A/.claude/lib/"
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
(cd "$R" && HOME="$FH" sh "$ROOT/.claude/status-write.sh" "fresh focus" >/dev/null)
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
for h in focus-staleness format worktree-hook-drift no-background-gate; do cp "$H/$h.sh" "$N/.claude/hooks/"; done
for shell in sh dash; do
  command -v "$shell" >/dev/null 2>&1 || continue
  for h in focus-staleness format worktree-hook-drift no-background-gate; do
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
# throwaway project; @TRACKED@ a tracked file in it; @GATE_WAIT@ gate-wait.sh beside GATE_RUN.
SETTINGS="$ROOT/.claude/settings.json"
if [ ! -f "$SETTINGS" ]; then
  fail "no .claude/settings.json: the hooks are registered nowhere, so none of them runs"
  finish
fi
# The registrations, the script paths in a command and the project-root prefix are read by the
# readers map.sh shares (lib/refs.sh); the tables at this file's foot test them there.
regs=$(refs_hook_registrations "$SETTINGS") \
  || { fail "settings.json does not parse, so no hook it registers can be checked (or run)"; finish; }
sl=$(jq -r '.statusLine.command // empty' "$SETTINGS")
# Every .claude/ path, and every script path, in a hook or status-line command goes through
# CLAUDE_PROJECT_DIR: `sh scripts/hooks/guard.sh` fails to launch from a subdirectory just the same.
pdp=$(refs_project_dir_prefix)
relative() {  # relative CMD: prints CMD if a .claude/ or script path in it is not reached through CLAUDE_PROJECT_DIR
  { printf '%s\n' "$1" | grep -oE '[^[:space:]]*\.claude/' | sed 's|\.claude/$||'
    refs_hook_scripts "$1" | sed 's|[^/]*$||'; } \
  | while IFS= read -r pre; do
    case $pre in /*|\"/*|\'/*) continue ;; esac  # an absolute path launches from anywhere
    printf '%s\n' "$pre" | grep -qE "$pdp" || { printf '%s\n' "$1"; break; }
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
no-background-gate.sh	2	out:BLOCKED by no-background-gate: a subagent started the gate with run_in_background	{"tool_name":"Bash","agent_id":"check","tool_input":{"command":"@GATE_WAIT@","run_in_background":true}}	a subagent's gate wait in the background
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
launching=$(printf '%s\n' "$regs" | while IFS="$(printf '\t')" read -r ev cmd; do [ -n "$(refs_hook_scripts "$cmd")" ] && printf '%s\t%s\n' "$ev" "$cmd"; done)
n=$(printf '%s\n' "$launching" | grep -c . || true)
if [ -n "$plugin" ]; then ok "the model's hooks are the plugin's ($(jq '[.hooks[][].hooks[]] | length' "$CLAUDUCTOR_FW/hooks/hooks.json") in its hooks.json); settings.json registers $n of this project's own"
else [ "$n" -ge 3 ] && ok "settings.json registers $n hook command(s) (non-vacuity)" || fail "settings.json registers only $n hook command(s): the table below would pass vacuously"; fi
script_of() { refs_hook_scripts "$1" | head -1 | sed 's|.*/||'; }
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
  pl=$(printf '%s' "$row" | cut -f4 | sed "s|@TRACKED@|$P/src/tracked.ts|g; s|@REPO@|$P|g; s|@GATE_WAIT@|$(dirname "$GATE_RUN")/gate-wait.sh|g"); why=$(printf '%s' "$row" | cut -f5)
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
for c in "sh $D/hooks/x.sh" "bash ./$D/hooks/x.sh" "sh \$HOME/p/$D/hooks/x.sh" "sh scripts/hooks/guard.sh" "sh guard.sh" "sh scripts/hooks/guard.sh>/dev/null" "sh \"\$CLAUDE_PROJECT_DIR\"/a.sh scripts/b.sh" "sh scripts/x.sh\`echo\`" "sh \${X:-scripts}/guard.sh"; do
  [ -n "$(relative "$c")" ] && ok "the path rule flags '$c'" || fail "the path rule passed '$c', which launches only from the root"
done
for c in "sh \"\$CLAUDE_PROJECT_DIR\"/$D/hooks/x.sh" "sh \"\${CLAUDE_PROJECT_DIR:-.}\"/$D/statusline.sh" "sh \$CLAUDE_PROJECT_DIR/$D/x.sh" "sh \${CLAUDE_PROJECT_DIR}/$D/hooks/x.sh" "sh \"\${CLAUDE_PROJECT_DIR}\"/$D/hooks/x.sh" "node \"\$CLAUDE_PROJECT_DIR\"/scripts/hooks/x.mjs" "sh /opt/hooks/x.sh" "npx biome format --write" "jq -e . \"\$CLAUDE_PROJECT_DIR\"/$D/settings.json" "npx tsc -p tsconfig.json --noEmit" "curl -fsSL https://example.com/install.sh | sh" "sh \"/opt/hooks/x.sh\"" "sh '/opt/p/$D/x.sh'"; do
  [ -z "$(relative "$c")" ] && ok "the path rule passes '$c'" || fail "the path rule flagged '$c', which is correct"
done
# The table covers every registered script, not only .claude/hooks/*.sh.
[ "$(script_of "node \"\$CLAUDE_PROJECT_DIR\"/scripts/hooks/x.mjs --flag")" = x.mjs ] && [ "$(script_of "sh \"\$CLAUDE_PROJECT_DIR\"/x.sh")" = x.sh ] && [ "$(script_of "sh \"\$CLAUDE_PROJECT_DIR\"/g.sh>/dev/null")" = g.sh ] && [ "$(script_of "sh \"\${CLAUDE_PROJECT_DIR:-.}\"/scripts/hooks/guard.sh")" = guard.sh ] && [ -z "$(script_of 'npx biome format --write')" ] \
  && [ -z "$(script_of "jq -e . \"\$CLAUDE_PROJECT_DIR\"/$D/settings.json")" ] && [ -z "$(script_of 'npx tsc -p tsconfig.json')" ] && [ -z "$(script_of 'python3 -m tools.shell_guard')" ] \
  && ok "a registered script in any directory or language gets a row of its own (x.mjs); an inline command needs none" || fail "script_of: '$(script_of "node \"\$CLAUDE_PROJECT_DIR\"/scripts/hooks/x.mjs")'"
finish
