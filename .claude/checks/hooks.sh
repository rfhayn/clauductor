#!/bin/sh
# The PreToolUse and UserPromptSubmit hooks, as payload → exit-code tables, in BOTH directions.
# A guard falsified only in the blocking direction can block legitimate work; one falsified only
# in the allowing direction can block nothing and still look correct. Every row runs the real
# hook against a throwaway git repository, never this one, so the result is about the hook's
# contract ("is this path tracked?") rather than what this repo contains today.
. "$(dirname "$0")/lib.sh"
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
printf 'm=$(mktemp -d)\ncp src/lib/app.ts "$m/app.ts"\nsed -i "" s/x/y/ "$m/app.ts"\n' > "$d/mut.sh"  # mktemp-in-tmpdir: fixture text the hook reads, never run
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
for t in sh cat git awk sed grep tr sort cut xargs mktemp dirname basename wc head tail pwd printf; do  # mktemp-in-tmpdir: a list of tools, not a call
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
nbg 2 "a subagent lane's gate -C <worktree> with run_in_background (OPS-161)" "$(bgp a1 "$G -C /abs/lane --quick" true 600000)"
nbg 2 "a subagent lane's gate-wait -C <worktree> under 600000 ms" "$(bgp a1 "scripts/ci/gate-wait.sh -C /abs/lane" false 300000)"
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

# ── no-silent-wait ──────────────────────────────────────────────────────────────────────
# Payloads in the shapes Claude Code 2.1.302 sent (OPS-90's probe): background_tasks is the
# SESSION's, so a subagent stop lists the main session's shell too; a task's output file sits in
# <dirname scratchpad_dir>/tasks and ends "[exited with code N]" or "[killed]".
W="$d/wait"; new_repo "$W"; S="$d/sess"; mkdir -p "$S/scratchpad" "$S/tasks"
WS="$W/.git/clauductor-wait"
swp() {  # swp EVENT SESSION AGENT [JSON to merge]: a payload for no-silent-wait
  _x=${4:-}; [ -n "$_x" ] || _x='{}'
  jq -cn --arg e "$1" --arg s "$2" --arg a "$3" --arg sp "$S/scratchpad" --argjson x "$_x" \
    '{hook_event_name:$e, session_id:$s, scratchpad_dir:$sp} | if $a == "" then . else .agent_id = $a end | . + $x'
}
nsw() {  # nsw WANT LABEL PAYLOAD [PATH]: run it, keep its stderr in $nsw_out
  rc=0; nsw_out=$(printf '%s' "$3" | (cd "$W" && CLAUDE_PROJECT_DIR="$W" PATH="${4:-$PATH}" NO_SILENT_WAIT_MAX_REFUSALS="${NSW_MAX:-20}" sh "$H/no-silent-wait.sh") 2>&1) || rc=$?
  # Exit 2 alone is no proof: dash exits 2 when it cannot open the script.
  [ "$1" = 2 ] && [ "$rc" = 2 ] && case $nsw_out in *"BLOCKED by no-silent-wait"*) ;; *) rc="2 without its refusal text";; esac
  expect_rc "$1" "$rc" "no-silent-wait $( [ "$1" = 2 ] && echo blocks || echo allows ): $2"
}
running() { printf 'partial output\n' > "$S/tasks/$1.output"; }
ended() { printf 'done\n\n[exited with code %s]\n' "${2:-0}" > "$S/tasks/$1.output"; }
bt() { printf '{"background_tasks":[{"id":"%s","type":"shell","status":"%s","command":"make check"}]}' "$1" "${2:-running}"; }
bash_bg='{"tool_name":"Bash","tool_input":{"command":"make check","timeout":300000},"tool_response":{"stdout":"","backgroundTaskId":"t1","timedOutAfterMs":300000}}'
nsw 0 "a subagent's Bash call that went to the background is recorded" "$(swp PostToolUse s1 a1 "$bash_bg")"
grep -qx t1 "$WS/s1/tasks/a1" 2>/dev/null && ok "no-silent-wait recorded task t1 for agent a1" || fail "no-silent-wait: no record of a1's task t1 in $WS/s1/tasks/a1"
running t1
nsw 2 "a subagent's stop while the command it started runs" "$(swp SubagentStop s1 a1 "$(bt t1)")"
case $nsw_out in *"$S/tasks/t1.output"*FOREGROUND*|*FOREGROUND*"$S/tasks/t1.output"*) ok "no-silent-wait: the refusal names the task's output file and says to wait in the foreground" ;;
  *) fail "no-silent-wait: the refusal does not name $S/tasks/t1.output or the foreground wait: $nsw_out" ;; esac
nsw 0 "a subagent's stop when the running shell is the MAIN session's (session-wide list)" "$(swp SubagentStop s1 a2 "$(bt t1)")"
nsw 0 "a subagent's stop once its task is listed as completed" "$(swp SubagentStop s1 a1 "$(bt t1 completed)")"
nsw 0 "a subagent's stop once its task is no longer listed" "$(swp SubagentStop s1 a1 '{"background_tasks":[]}')"
[ -e "$WS/s1/tasks/a1" ] && fail "no-silent-wait: an allowed stop left a1's task record behind" || ok "no-silent-wait: an allowed stop clears the agent's record"
nsw 0 "a subagent's Monitor is recorded" "$(swp PostToolUse s1 a3 '{"tool_name":"Monitor","tool_response":{"taskId":"m1","persistent":false}}')"
nsw 2 "a subagent's stop while its Monitor runs" "$(swp SubagentStop s1 a3 "$(bt m1)")"
nsw 0 "the main session's background Bash is recorded as its own" "$(swp PostToolUse s1 "" "$bash_bg")"
[ "$(ls "$WS/s1/tasks" 2>/dev/null | LC_ALL=C sort | tr '\n' ' ')" = "_main a3 " ] && grep -qx t1 "$WS/s1/tasks/_main" \
  && ok "no-silent-wait records the main session's task as _main, no agent's" || fail "no-silent-wait: tasks/ holds $(ls "$WS/s1/tasks" 2>/dev/null | tr '\n' ' ')"
nsw 0 "a subagent's foreground Bash that finished is not recorded" "$(swp PostToolUse s1 a4 '{"tool_name":"Bash","tool_response":{"stdout":"ok"}}')"
[ -e "$WS/s1/tasks/a4" ] && fail "no-silent-wait recorded a call that made no background task" || ok "no-silent-wait records nothing for a call that made no background task"
nsw 0 "a stop with nothing recorded" "$(swp SubagentStop s1 a4 "$(bt t1)")"
# A subagent's Agent call goes to the background (it has no foreground choice interactively), and
# the report goes to whoever is still awake: with the launcher's turn over, the main session.
agent_bg='{"tool_name":"Agent","tool_input":{"run_in_background":true},"tool_response":{"isAsync":true,"status":"async_launched","agentId":"ab12"}}'
abt() { printf '{"background_tasks":[{"id":"ab12","type":"subagent","status":"%s","description":"review the diff"}]}' "$1"; }
nsw 0 "a subagent's background Agent call is recorded" "$(swp PostToolUse s1 a11 "$agent_bg")"
# Before the launched agent's SubagentStart is recorded, it has not ended: the stop is refused.
nsw 2 "a subagent's stop before the agent it launched has started" "$(swp SubagentStop s1 a11 "$(abt running)")"
nsw 0 "the agent it launched starts" "$(swp SubagentStart s1 ab12)"
nsw 2 "a subagent's stop while the agent it launched runs" "$(swp SubagentStop s1 a11 "$(abt running)")"
case $nsw_out in *"Wait for each agent"*"$WS/s1/ended/ab12"*"Re-run the wait"*"while the agent still runs"*"no longer runs"*"end your turn"*) ok "no-silent-wait: the refusal says to wait for the agent's end marker, to re-run the wait while it runs, and to end the turn once it no longer does" ;;
  *) fail "no-silent-wait: the refusal does not name $WS/s1/ended/ab12 as the agent's wait, or the re-run: $nsw_out" ;; esac
# A launcher whose agent was stopped (TaskStop: a terminal status, and no marker) may end (#156).
nsw 0 "another subagent launches the same agent" "$(swp PostToolUse s1 a14 "$agent_bg")"
nsw 0 "a subagent's stop once the agent it launched was stopped (no marker)" "$(swp SubagentStop s1 a14 "$(abt killed)")"
case $nsw_out in *"Wait for each command"*) fail "no-silent-wait: an agent-only refusal tells the agent to wait on a command's output file" ;; *) ok "no-silent-wait: an agent-only refusal gives no command wait" ;; esac
nsw 0 "the launched agent's own stop (the list names it, but it started nothing)" "$(swp SubagentStop s1 ab12 "$(abt running)")"
{ [ ! -e "$WS/s1/agents/ab12" ] && [ -e "$WS/s1/ended/ab12" ]; } && ok "no-silent-wait: the launched agent's stop removes its record and writes its end marker" \
  || fail "no-silent-wait: after the launched agent's stop, agents/ab12 exists=$([ -e "$WS/s1/agents/ab12" ] && echo y || echo n), ended/ab12 exists=$([ -e "$WS/s1/ended/ab12" ] && echo y || echo n)"
# background_tasks can lag the agent's own stop: its end marker decides.
nsw 0 "the subagent's stop once the agent it launched ended, though still listed 'running'" "$(swp SubagentStop s1 a11 "$(abt running)")"
nsw 0 "a resumed agent starts again" "$(swp SubagentStart s1 ab12)"
[ -e "$WS/s1/ended/ab12" ] && fail "no-silent-wait: a resumed agent kept its end marker" || ok "no-silent-wait: a resumed agent's start clears its end marker"
nsw 0 "the resumed agent's stop" "$(swp SubagentStop s1 ab12 "$(abt completed)")"
# Its end marker is now stale: a new launch of it clears the marker, so a launcher ending its turn
# before a SubagentStart (never probed for a resumed agent) is still held (#156).
nsw 0 "a subagent resumes that agent in the background" "$(swp PostToolUse s1 a13 "$agent_bg")"
nsw 2 "the launcher's stop while the resumed agent runs, its old end marker notwithstanding" "$(swp SubagentStop s1 a13 "$(abt running)")"
nsw 0 "a foreground Agent call that completed is not recorded" "$(swp PostToolUse s1 a12 '{"tool_name":"Agent","tool_response":{"status":"completed","agentId":"cd34"}}')"
[ -e "$WS/s1/tasks/a12" ] && fail "no-silent-wait recorded a foreground Agent call" || ok "no-silent-wait records nothing for a foreground Agent call"
# The output file's end line ends a task, whatever status background_tasks still shows.
nsw 0 "a subagent's task is recorded" "$(swp PostToolUse s1 a7 '{"tool_name":"Bash","tool_response":{"backgroundTaskId":"t5"}}')"
ended t5 1
nsw 0 "a subagent's stop once its task's file ends, though still listed 'running'" "$(swp SubagentStop s1 a7 "$(bt t5)")"
# A task that never ends (a server) holds a turn a bounded number of times, and says how to end it.
nsw 0 "a subagent's server is recorded" "$(swp PostToolUse s1 a8 '{"tool_name":"Bash","tool_response":{"backgroundTaskId":"srv"}}')"
running srv
NSW_MAX=2
nsw 2 "a stop while its server runs (refusal 1 of 2)" "$(swp SubagentStop s1 a8 "$(bt srv)")"
case $nsw_out in *"Refusal 1 of 2"*"by its PID"*"never by pattern"*) ok "no-silent-wait: the refusal counts, and says to end the command by its PID, never by pattern" ;; *) fail "no-silent-wait: the refusal does not count or say how to end the command by PID: $nsw_out" ;; esac
nsw 2 "the next stop (refusal 2 of 2)" "$(swp SubagentStop s1 a8 '{"stop_hook_active":true,"background_tasks":[{"id":"srv","status":"running"}]}')"
nsw 0 "the stop after the last refusal" "$(swp SubagentStop s1 a8 '{"stop_hook_active":true,"background_tasks":[{"id":"srv","status":"running"}]}')"
# A teammate never ends, so a released task must not hold its next idle all over again.
nsw 0 "a teammate's server is recorded" "$(swp PostToolUse s1 tm2 '{"tool_name":"Bash","tool_response":{"backgroundTaskId":"srv"}}')"
NSW_MAX=1
nsw 2 "a teammate going idle while its server runs (refusal 1 of 1)" "$(swp TeammateIdle s1 tm2)"
nsw 0 "the teammate's next idle, past the bound" "$(swp TeammateIdle s1 tm2)"
nsw 0 "the teammate's idle after that: the released server no longer holds it" "$(swp TeammateIdle s1 tm2)"
NSW_MAX=20
rm -f "$S/tasks/srv.output"
# A subagent's shell `&` makes no task at all: refused before it runs, unless the command waits.
shp() { jq -cn --arg a "$1" --arg c "$2" '{hook_event_name:"PreToolUse", session_id:"s1", tool_name:"Bash", tool_input:{command:$c}} | if $a == "" then . else .agent_id = $a end'; }
nsw 2 "a subagent's 'make check > log 2>&1 &'" "$(shp a1 'make check > log 2>&1 &')"
nsw 2 "a teammate's 'nohup go test ./... &'" "$(shp tm1 'nohup go test ./... &')"
nsw 0 "a subagent's 'make a & make b & wait'" "$(shp a1 'make a & make b & wait')"
nsw 2 "a subagent's 'sleep 100 & # wait' (a comment does not wait)" "$(shp a1 'sleep 100 & # wait')"
nsw 2 "a subagent's 'make a & echo please wait'" "$(shp a1 'make a & echo please wait')"
nsw 0 "a subagent's '&&', '2>&1' and \$((3 & 1))" "$(shp a1 'make a && make b 2>&1; echo $((3 & 1))')"
nsw 0 "a subagent's quoted '&'" "$(shp a1 'git commit -m "a & b"')"
nsw 0 "the main session's '&' (it is not a subagent)" "$(shp "" 'make check &')"
nsw 2 "without jq: a subagent's '&'" "$(shp a1 'make check &')" "$d/nojq"
nsw 0 "without jq: a subagent's '&&' and '2>&1'" "$(shp a1 'make a && make b 2>&1')" "$d/nojq"
nsw 0 "without jq: the main session's '&'" "$(shp "" 'make check &')" "$d/nojq"
nsw 0 "without jq: the main session's Bash naming AskUserQuestion" "$(shp "" 'grep -n AskUserQuestion settings.json')" "$d/nojq"
# Another session's records are pruned once stale; this session's never, however old.
mkdir -p "$WS/old/agents" "$WS/s1/agents" && : > "$WS/old/agents/x" && touch -t 202001010000 "$WS/old" "$WS/s1"
nsw 0 "a subagent starts in session s1" "$(swp SubagentStart s1 a10)"
[ -d "$WS/old" ] && fail "no-silent-wait kept another session's stale records" || ok "no-silent-wait pruned another session's stale records"
[ -f "$WS/s1/agents/a10" ] && [ -f "$WS/s1/tasks/a3" ] && ok "no-silent-wait kept this session's records, however old" || fail "no-silent-wait pruned the live session's records"
nsw 0 "that subagent's stop" "$(swp SubagentStop s1 a10 '{"background_tasks":[]}')"
# A teammate's idle has no background_tasks: the output file says whether the task ended.
nsw 0 "a teammate's background task is recorded" "$(swp PostToolUse s1 tm1 '{"tool_name":"Bash","tool_response":{"backgroundTaskId":"t2"}}')"
running t2
nsw 2 "a teammate going idle while its task runs (output file has no end line)" "$(swp TeammateIdle s1 tm1)"
ended t2 3
nsw 0 "a teammate going idle once its task's file ends '[exited with code 3]'" "$(swp TeammateIdle s1 tm1)"
# AskUserQuestion: the main session's question waits for the owner and reads nothing else meanwhile.
ask='{"tool_name":"AskUserQuestion","tool_input":{"questions":[{"question":"Queue OPS-89?"}]}}'
rm -f "$S/tasks"/*.output
nsw 0 "the main session's question with nothing running" "$(swp PreToolUse s2 "" "$ask")"
nsw 0 "a subagent starts" "$(swp SubagentStart s3 a5)"
nsw 2 "the main session's question while a subagent runs" "$(swp PreToolUse s3 "" "$ask")"
case $nsw_out in *owner-queue.md*PushNotification*) ok "no-silent-wait: the refusal says to use the owner queue or a push" ;; *) fail "no-silent-wait: the refusal does not name the owner queue and PushNotification: $nsw_out" ;; esac
nsw 0 "that subagent's stop with nothing of its own running" "$(swp SubagentStop s3 a5 '{"background_tasks":[]}')"
nsw 0 "the main session's question once the subagent has ended" "$(swp PreToolUse s3 "" "$ask")"
# The payload is built first: bash 3.2 (macOS sh) mangles \" inside "$(…)" inside "…".
for t in t3 t4; do
  tp=$(printf '{"tool_name":"Bash","tool_response":{"backgroundTaskId":"%s"}}' "$t")
  nsw 0 "the main session's background command $t is recorded" "$(swp PostToolUse s4 "" "$tp")"
done
running t3; running t4
nsw 2 "the main session's question while its background commands run" "$(swp PreToolUse s4 "" "$ask")"
ended t3; printf '\n[killed]\n' > "$S/tasks/t4.output"
# Only recorded tasks count: a foreground call that ran to its timeout can leave a file behind.
printf 'a foreground call that ran to its timeout\nFAIL\n' > "$S/tasks/left.output"
printf 'an agent transcript, never an end line\n' > "$S/transcript"; ln -sf "$S/transcript" "$S/tasks/a9.output"
nsw 0 "the main session's question once its commands ended or were killed, beside a leftover file nothing recorded" "$(swp PreToolUse s4 "" "$ask")"
nsw 0 "a subagent's own question (it returns to a session that is awake)" "$(swp PreToolUse s3 a6 "$ask")"
nsw 0 "another tool from the main session" "$(swp PreToolUse s3 "" '{"tool_name":"Bash","tool_input":{"command":"ls"}}')"
nsw 0 "an empty payload" ""
# Without jq it fails closed for what it polices: the question, and a subagent's stop ONCE.
nsw 2 "without jq: the main session's question" "$(swp PreToolUse s2 "" "$ask")" "$d/nojq"
nsw 2 "without jq: a subagent's first stop" "$(swp SubagentStop s1 a1 '{"stop_hook_active":false}')" "$d/nojq"
nsw 0 "without jq: a subagent's stop already refused once (stop_hook_active)" "$(swp SubagentStop s1 a1 '{"stop_hook_active":true}')" "$d/nojq"
nsw 0 "without jq: a teammate going idle (no flag would ever end a refusal)" "$(swp TeammateIdle s1 tm1)" "$d/nojq"
nsw 0 "without jq: a PostToolUse" "$(swp PostToolUse s1 a1 "$bash_bg")" "$d/nojq"

# ── cd-guard ────────────────────────────────────────────────────────────────────────────
# A build-change Workflow call recorded from a worktree holds it; the main session's `cd` out of
# it is refused, a `cd` within it, in a subshell or from elsewhere is not. Every BLOCK row also
# needs the refusal text: exit 2 alone is no proof (dash exits 2 when it cannot open the script).
C="$d/cdg"; new_repo "$C"; git -C "$C" commit -q --allow-empty -m seed
git -C "$C" worktree add -q -b build "$d/cdg-wt" 2>/dev/null; CW=$(cd "$d/cdg-wt" && pwd -P); mkdir -p "$CW/sub"
CS="$d/cdg-sess"; mkdir -p "$CS/scratchpad"
CH="$C/.git/clauductor-cd-guard"
# The payloads and the state file below have the shapes Claude Code 2.1.306 sent and wrote in the
# OPS-82 probe (2026-10-21): every payload carries transcript_path, and a run's state file is
# <transcript_path minus .jsonl>/workflows/<runId>.json, written only as the run ends.
CWF="$CS/s1/workflows"; mkdir -p "$CWF"
cgp() {  # cgp EVENT CWD [JSON to merge]: a payload for cd-guard, session s1, main session
  _x=${3:-}; [ -n "$_x" ] || _x='{}'
  jq -cn --arg e "$1" --arg c "$2" --arg tp "$CS/s1.jsonl" --argjson x "$_x" \
    '{hook_event_name:$e, session_id:"s1", transcript_path:$tp, cwd:$c} + $x'
}
cdb() { cgp PreToolUse "${2:-$CW}" "$(jq -cn --arg c "$1" '{tool_name:"Bash", tool_input:{command:$c}}')"; }
wf() {  # wf TOOL_INPUT [TASK_ID] [RUN_ID] [WORKFLOW_NAME, "-" for none]: the Workflow call's PostToolUse, as it returns async_launched
  cgp PostToolUse "$CW" "$(jq -cn --argjson i "$1" --arg t "${2:-wf1}" --arg r "${3:-wf_r1}" --arg n "${4:-build-change}" '{tool_name:"Workflow", tool_input:$i,
    tool_response:({status:"async_launched", taskId:$t, taskType:"local_workflow", runId:$r, summary:"x"} + (if $n == "-" then {} else {workflowName:$n} end))}')"
}
wfstate() {  # wfstate RUN_ID STATUS: the state file as a run ends; STATUS "-garbled" writes one cut short
  case $2 in
    -garbled) printf '{"runId":"%s","status":"compl' "$1" ;;
    *) jq -cn --arg r "$1" --arg s "$2" '{runId:$r, taskId:"t", workflowName:"build-change", status:$s, summary:"x",
         startTime:0, durationMs:1, agentCount:1, totalTokens:0, totalToolCalls:0, logs:[], result:null, workflowProgress:[]}' ;;
  esac > "$CWF/$1.json"
}
stop() {  # stop TASK_ID...: the session's Stop, its background_tasks listing each id as a running workflow
  cgp Stop "$CW" "$(printf '%s\n' "$@" | jq -cRn '{stop_hook_active:false, last_assistant_message:"x", session_crons:[],
    background_tasks:[inputs | select(. != "") | {id:., type:"workflow", status:"running", description:"x", name:"build-change"}]}')"
}
cdg() {  # cdg WANT LABEL PAYLOAD [PATH]
  rc=0; cdg_out=$(printf '%s' "$3" | (cd "$C" && CLAUDE_PROJECT_DIR="$C" HOME="$d/cdg-home" PATH="${4:-$PATH}" sh "$H/cd-guard.sh") 2>&1) || rc=$?
  [ "$1" = 2 ] && [ "$rc" = 2 ] && case $cdg_out in *"BLOCKED by cd-guard"*) ;; *) rc="2 without its refusal text";; esac
  expect_rc "$1" "$rc" "cd-guard $( [ "$1" = 2 ] && echo blocks || echo allows ): $2"
}
cdg 0 "a cd out with no build running" "$(cdb "cd $d && ls")"
cdg 0 "another workflow's call records no hold" "$(wf '{"name":"review-changes"}' wf0 wf_r0 review-changes)"
[ -e "$CH/s1/holds/wf0" ] && fail "cd-guard recorded a hold for a workflow that is not build-change" || ok "cd-guard records no hold for a workflow that is not build-change"
cdg 0 "build-change's Workflow call is recorded" "$(wf '{"name":"build-change","args":{"change":"x"}}')"
[ "$(head -n 1 "$CH/s1/holds/wf1" 2>/dev/null)" = "$CW" ] && ok "cd-guard holds the worktree the build started from ($CW)" || fail "cd-guard: holds/wf1 starts '$(head -n 1 "$CH/s1/holds/wf1" 2>/dev/null)', want $CW"
cdg 2 "the session's cd to the main checkout" "$(cdb "cd $C")"
case $cdg_out in *"rm $CH/s1/holds/wf1"*) ok "cd-guard: the refusal names the hold file to remove" ;; *) fail "cd-guard: the refusal does not name $CH/s1/holds/wf1: $cdg_out" ;; esac
cdg 2 "a cd inside a compound command (the 2026-10-17 slip)" "$(cdb "git status && cd $d/cdg-sess/scratchpad && ls")"
cdg 2 "cd .. from the worktree's root" "$(cdb "cd ..")"
cdg 2 "a bare cd (home)" "$(cdb "cd")"
cdg 2 "cd ~/x" "$(cdb "cd ~/x")"
cdg 2 "pushd out" "$(cdb "pushd /")"
cdg 2 "builtin cd out" "$(cdb "builtin cd /")"
cdg 2 "time cd out (a keyword: the cd runs in this shell)" "$(cdb "time cd /")"
cdg 2 "a cd to a variable (unresolvable)" "$(cdb "cd \"\$TMPDIR\"")"
cdg 2 "cd - (unresolvable)" "$(cdb "cd -")"
cdg 2 "in, then out through a relative path" "$(cdb "cd sub && cd ../..")"
cdg 2 "from a subdirectory of the worktree" "$(cdb "cd /" "$CW/sub")"
cdg 0 "a cd within the worktree" "$(cdb "cd sub && ls")"
cdg 0 "a cd to the worktree's own root by its absolute path" "$(cdb "cd $CW && git status")"
cdg 0 "a cd to a directory not made yet, inside" "$(cdb "mkdir -p new/x && cd new/x")"
# A worktree nested inside the held one is another checkout: moving there sends the run's agents to it.
git -C "$C" worktree add -q -b nested "$CW/.claude/worktrees/x" 2>/dev/null
cdg 2 "a cd into a worktree nested inside the held one" "$(cdb "cd .claude/worktrees/x")"
cdg 2 "a cd into a directory made first inside the nested worktree" "$(cdb "mkdir -p .claude/worktrees/x/new && cd .claude/worktrees/x/new")"
cdg 0 "a cd into the directory that holds it, still this checkout" "$(cdb "cd .claude/worktrees")"
git -C "$C" worktree remove --force "$CW/.claude/worktrees/x" 2>/dev/null; rm -rf "$CW/.claude"
# A target not made yet is resolved through what exists of it: a symlink out of the worktree (to a
# directory in no checkout) takes `mkdir -p lnk/new` and `lnk/../esc` outside it; one that stays in does not.
mkdir -p "$d/cdg-out/target"; ln -s "$d/cdg-out/target" "$CW/lnk"; ln -s sub "$CW/lnk-in"
cdg 2 "a cd through a symlink out, into a directory made first" "$(cdb "mkdir -p lnk/new && cd lnk/new")"
cdg 2 "a cd through a symlink out and up, into a directory made first" "$(cdb "mkdir -p lnk/../esc && cd lnk/../esc")"
cdg 0 "a cd through a symlink that stays inside, into a directory made first" "$(cdb "mkdir -p lnk-in/new && cd lnk-in/new")"
# Each target is read two ways, and a cd passes only if BOTH stay in the held checkout: as the shell's
# own cd reads it (`..` taken off the typed path first) and through its symlinks first.
mkdir -p "$CW/a/b/c"; ln -s a/b/c "$CW/deep"
cdg 2 "cd deep/../.. (the shell takes .. first: the worktree's parent)" "$(cdb "cd deep/../..")"
cdg 2 "a cd back through a symlink out (new/../lnk)" "$(cdb "mkdir -p new && cd new/../lnk")"
cdg 2 "cd lnk/.. (the shell stays in, but through the symlink it leaves)" "$(cdb "cd lnk/..")"
cdg 0 "cd deep/.. (the two readings differ, both inside)" "$(cdb "cd deep/..")"
cdg 0 "cd lnk-in/.. (both readings inside)" "$(cdb "cd lnk-in/..")"
rm -rf "$CW/lnk" "$CW/lnk-in" "$CW/deep" "$CW/a"
cdg 0 "a cd out inside a subshell" "$(cdb "( cd $C && git log -1 )")"
cdg 0 "a cd out inside \$( )" "$(cdb "x=\$(cd $C && pwd)")"
cdg 0 "a cd out inside sh -c" "$(cdb "sh -c 'cd / && ls'")"
cdg 0 "a mention of cd (echo, a commit message)" "$(cdb "echo cd / && git commit -m 'never cd /'")"
cdg 0 "git -C instead of a cd" "$(cdb "git -C $C log -1")"
cdg 0 "a cd out from a session already outside the worktree" "$(cdb "cd /" "$C")"
cdg 0 "a subagent's cd out (build-change's agents cd into the root themselves)" "$(cgp PreToolUse "$CW" "$(jq -cn --arg c "cd /" '{tool_name:"Bash", agent_id:"a1", tool_input:{command:$c}}')")"
cdg 0 "another session's cd out" "$(cdb "cd /" | jq -c '.session_id = "s2"')"
# A run named by its script path or its inline meta holds too, with no workflowName in the result
# ("-"); and a re-run or resume through the script Claude Code saved for an earlier run
# (…/workflows/scripts/build-change-wf_<id>.js: 6 of 16 real calls on 2026-10-21), by its path alone
# and by the result's workflowName alone.
cdg 0 "a resume through build-change's saved script is recorded" "$(wf '{"args":{"change":"x","resume":2},"scriptPath":"/h/.claude/projects/p/s/workflows/scripts/build-change-wf_7190f406-97d.js"}' wf7 wf_r7 -)"
cdg 0 "a run the result names build-change is recorded" "$(wf '{"scriptPath":"/h/.claude/projects/p/s/workflows/scripts/x.js"}' wf8 wf_r8)"
cdg 0 "another workflow's saved script records no hold" "$(wf '{"scriptPath":"/h/.claude/projects/p/s/workflows/scripts/review-changes-wf_1a.js"}' wf9 wf_r9 review-changes)"
[ -f "$CH/s1/holds/wf7" ] && [ -f "$CH/s1/holds/wf8" ] && [ ! -e "$CH/s1/holds/wf9" ] \
  && ok "cd-guard records a run through build-change's saved script, or one the result names build-change, and not another workflow's" \
  || fail "cd-guard: holds/ is $(ls "$CH/s1/holds" 2>/dev/null | tr '\n' ' '), want wf7 and wf8 beside wf1, and no wf9"
rm -f "$CH/s1/holds/wf7" "$CH/s1/holds/wf8"
cdg 0 "build-change by scriptPath is recorded" "$(wf '{"scriptPath":".claude/workflows/build-change.js"}' wf2 wf_r2 -)"
cdg 0 "build-change as an inline script is recorded" "$(wf "$(jq -cn '{script:"export const meta = { name: '"'"'build-change'"'"', description: \"x\" }"}')" wf3 wf_r3 -)"
[ -f "$CH/s1/holds/wf2" ] && [ -f "$CH/s1/holds/wf3" ] && ok "cd-guard records build-change named by its scriptPath and by its inline meta" || fail "cd-guard: holds/ is $(ls "$CH/s1/holds" 2>/dev/null | tr '\n' ' ')"
rm -f "$CH/s1/holds/wf2" "$CH/s1/holds/wf3"
# A hold ends when its run's state file says completed, failed or killed. Anything else keeps it: no
# state file (the run is going), another status, a file that does not parse.
[ "$(sed -n '2,3p' "$CH/s1/holds/wf1" 2>/dev/null | tr '\n' '|')" = "task wf1|state $CWF/wf_r1.json|" ] \
  && ok "cd-guard records the run's task id and its state file beside the root" \
  || fail "cd-guard: holds/wf1 is '$(tr '\n' '|' < "$CH/s1/holds/wf1" 2>/dev/null)', want the root, task wf1, state $CWF/wf_r1.json"
cdg 2 "while the run has written no state file (it is still going)" "$(cdb "cd /")"
for s in running paused -garbled; do
  wfstate wf_r1 "$s"
  cdg 2 "while the run's state file $( [ "$s" = -garbled ] && echo 'does not parse' || echo "says '$s'")" "$(cdb "cd /")"
done
[ -f "$CH/s1/holds/wf1" ] && ok "cd-guard keeps the hold through a missing, running, unknown or garbled state file" || fail "cd-guard: a run that has not ended lost its hold"
rm -f "$CH/s1/holds/wf1" "$CWF/wf_r1.json"
for s in completed failed killed; do
  cdg 0 "a $s run's Workflow call is recorded" "$(wf '{"name":"build-change"}' "wf-$s" "wf_$s")"
  cdg 2 "a $s run's hold, before its state file is written" "$(cdb "cd /")"
  wfstate "wf_$s" "$s"
  cdg 0 "once the run's state file says '$s'" "$(cdb "cd /")"
  [ -e "$CH/s1/holds/wf-$s" ] && fail "cd-guard: a $s run's hold was left behind" || ok "cd-guard: a $s run's hold is removed"
done
# The session's Stop lists its running tasks in background_tasks: a hold whose task is no longer
# listed is dropped. With no list at all (a Claude Code that sends none), every hold stays.
cdg 0 "build-change's call is recorded (wf5)" "$(wf '{"name":"build-change"}' wf5 wf_r5)"
cdg 0 "Stop while background_tasks lists the run" "$(stop b0kat3se2 wf5)"
[ -f "$CH/s1/holds/wf5" ] && ok "cd-guard: Stop keeps a hold whose task background_tasks still lists" || fail "cd-guard: Stop dropped a hold whose task is still running"
cdg 0 "Stop with no background_tasks" "$(cgp Stop "$CW" '{"stop_hook_active":false}')"
[ -f "$CH/s1/holds/wf5" ] && ok "cd-guard: Stop with no background_tasks keeps every hold" || fail "cd-guard: Stop with no background_tasks dropped a hold"
cdg 2 "the hold both Stops kept" "$(cdb "cd /")"
cdg 0 "without jq: Stop never blocks" "$(stop)" "$d/nojq"
cdg 0 "Stop once background_tasks no longer lists the run" "$(stop b0kat3se2)"
[ -e "$CH/s1/holds/wf5" ] && fail "cd-guard: Stop left the hold of a task no longer running" || ok "cd-guard: Stop drops a hold whose task background_tasks no longer lists"
cdg 0 "a cd out once Stop dropped the hold" "$(cdb "cd /")"
cdg 0 "a hold re-recorded with no task id" "$(cgp PostToolUse "$CW" '{"tool_name":"Workflow","tool_input":{"name":"build-change"},"tool_response":{}}')"
ls "$CH/s1/holds" 2>/dev/null | grep -q '^run-' && ok "cd-guard keys a hold with no task id by time" || fail "cd-guard: no run-* hold in $(ls "$CH/s1/holds" 2>/dev/null | tr '\n' ' ')"
cdg 0 "Stop with nothing running" "$(stop)"
ls "$CH/s1/holds" 2>/dev/null | grep -q '^run-' && ok "cd-guard: Stop leaves a hold with no task id (nothing to match it by)" || fail "cd-guard: Stop dropped a hold with no task id"
cdg 2 "a hold with no task id still holds" "$(cdb "cd /")"
cdg 2 "without jq: a cd while a hold is recorded" "$(cdb "cd /")" "$d/nojq"
cdg 0 "without jq: a command with no cd" "$(cdb "git status")" "$d/nojq"
# The crash: claude killed mid-run writes no state file and sends no Stop, so only the age ends it.
cdg 0 "a run's call is recorded, and then claude is killed" "$(wf '{"name":"build-change"}' wf6 wf_r6)"
touch -t 200001010000 "$CH/s1/holds"/run-*
cdg 2 "a crashed run's hold, inside CD_GUARD_MAX_HOURS" "$(cdb "cd /")"
touch -t 200001010000 "$CH/s1/holds/wf6"
cdg 0 "past CD_GUARD_MAX_HOURS" "$(cdb "cd /")"
[ -z "$(ls "$CH/s1/holds" 2>/dev/null)" ] && ok "cd-guard: the age backstop removes a crashed run's hold and one with no task id" || fail "cd-guard: past CD_GUARD_MAX_HOURS, holds/ still has $(ls "$CH/s1/holds" | tr '\n' ' ')"
cdg 0 "build-change's call is recorded again" "$(wf '{"name":"build-change"}' wf4 wf_r4)"
git -C "$C" worktree remove --force "$CW" 2>/dev/null
cdg 0 "once the held worktree is gone" "$(cdb "cd /")"

# ── catch-up-guard ──────────────────────────────────────────────────────────────────────
# With catch-up on, a git command that moves the main checkout (pull, merge, rebase, reset, switch,
# update-ref, ...) aimed there is refused: from the main checkout, a subdirectory of it, or a lane
# that aims there by -C, cd, chdir, eval, sh -c, GIT_DIR or an alias, through any wrapper or
# spelling. A lane's own catch-up merge (after a cd into it, too), a mention, merge-pr's
# squash-merge and a pull with catch-up off are not. A stand-in clauductor answers `panel catch-up` from a state file, and only for the
# project the file names, so a row passes only if the hook asks about the main checkout.
M="$d/cug"; new_repo "$M"; git -C "$M" commit -q --allow-empty -m seed; mkdir -p "$M/sub" "$M/.claude/worktrees"
git -C "$M" worktree add -q -b lane "$M/.claude/worktrees/lane" 2>/dev/null; ML="$M/.claude/worktrees/lane"
git -C "$M" config alias.up 'pull --ff-only'; git -C "$M" config alias.st status
mkdir -p "$d/cug-bin"
cat > "$d/cug-bin/clauductor" <<EOF
#!/bin/sh
# panel catch-up --project P: the state file's word when P is the project it names, else off
[ "\$1 \$2 \$3" = "panel catch-up --project" ] || exit 3
read -r want state code < "$d/cug-state"
[ "\$4" = "\$want" ] || { echo off; exit 0; }
echo "\$state"; exit "\${code:-0}"
EOF
chmod +x "$d/cug-bin/clauductor"
cu() { printf '%s %s %s\n' "$1" "$2" "${3:-0}" > "$d/cug-state"; }  # cu PROJECT WORD [EXIT]
# PATHs with no real clauductor: tools only (cug-noc), and the same less jq beside the stand-in.
mkdir -p "$d/cug-noc" "$d/cug-nojq"
for t in sh cat git awk sed grep tr sort cut head tail pwd printf dirname basename jq; do
  p=$(command -v "$t" 2>/dev/null) || continue
  ln -sf "$p" "$d/cug-noc/$t"; [ "$t" = jq ] || ln -sf "$p" "$d/cug-nojq/$t"
done
ln -sf "$d/cug-bin/clauductor" "$d/cug-nojq/clauductor"
cug() {  # cug WANT LABEL COMMAND CWD [PATH]
  rc=0; cug_out=$(payload "$3" "$4" | (cd "$4" && PATH="${5:-$d/cug-bin:$PATH}" sh "$H/catch-up-guard.sh") 2>&1) || rc=$?
  [ "$1" = 2 ] && [ "$rc" = 2 ] && case $cug_out in *"BLOCKED by catch-up-guard"*) ;; *) rc="2 without its refusal text";; esac
  expect_rc "$1" "$rc" "catch-up-guard $( [ "$1" = 2 ] && echo blocks || echo allows ): $2"
}
cu "$M" on
cug 2 "a pull in the main checkout" "git pull --ff-only origin main" "$M"
cug 2 "a pull from a subdirectory of it (the session's cwd persists)" "git pull" "$M/sub"
cug 2 "a merge in the main checkout" "git merge origin/main" "$M"
cug 2 "a rebase in the main checkout" "git rebase origin/main" "$M"
cug 2 "a reset in the main checkout" "git reset --hard origin/main" "$M"
cug 2 "a pull after git's own options" "git -c color.ui=never --no-pager pull" "$M"
cug 2 "a pull by an alias" "git up" "$M"
cug 2 "a pull by an alias set on the command line" "git -c alias.x=pull x" "$M"
cug 2 "a subcommand named by a variable" 'git "$sub"' "$M"
cug 2 "git named by a variable" '$g pull' "$M"
cug 2 "a pull in a subshell" "(git pull)" "$M"
cug 2 "a pull after another command" "git status && git pull" "$M"
cug 2 "a lane's pull by -C and a variable" 'git -C "$main" pull' "$ML"
cug 2 "a lane's pull by -C and the main checkout's path" "git -C $M pull" "$ML"
cug 2 "a lane's pull by -C and a relative path" "git -C ../../.. pull" "$ML"
cug 2 "a lane's merge after cd" "cd \"$M\" && git merge origin/main" "$ML"
cug 2 "a lane's merge after zsh's chdir" "chdir $M; git merge origin/main" "$ML"
cug 2 "a lane's merge after an eval of split words" "x=c; y=d; eval \"\$x\$y\" $M; git merge origin/main" "$ML"
cug 2 "a pull an eval builds" 'c="git pull"; eval "$c"' "$ML"
cug 2 "a lane's pull by sh -c" "sh -c 'cd \"\$0\" && git pull' $M" "$ML"
cug 2 "a lane's merge by GIT_DIR and GIT_WORK_TREE" "GIT_DIR=$M/.git GIT_WORK_TREE=$M git merge origin/main" "$ML"
cug 2 "a lane's merge by --work-tree" "git --work-tree=$M --git-dir=$M/.git merge origin/main" "$ML"
cug 2 "a lane's pull by env -C" "env -C $M git pull" "$ML"
cug 2 "a lane's pull by pushd" "pushd $M >/dev/null && git pull" "$ML"
cug 2 "a lane's pull by -C and an unquoted \$( … )" 'git -C $(git worktree list --porcelain | sed -n "1s/^worktree //p") pull' "$ML"
cug 2 "a lane's pull by -C and backticks" 'git -C `git worktree list --porcelain | sed -n 1p` pull' "$ML"
cug 2 "a lane's pull by -C and a nested \$( … )" 'git -C $(dirname $(pwd)) pull' "$ML"
cug 2 "a pull through timeout" "timeout 60 git pull" "$M"
cug 2 "a pull through nice -n" "nice -n 5 git pull" "$M"
cug 2 "a pull through command -p" "command -p git pull" "$M"
cug 2 "a pull through sudo -u" "sudo -u rich git pull" "$M"
cug 2 "a pull through xargs -n1" "echo origin | xargs -n1 git pull" "$M"
cug 2 "a pull through a wrapper not on any list" "chronic git pull" "$M"
cug 2 "a pull spelled g\\it" 'g\it pull' "$M"
cug 2 "a pull spelled gi''t" "gi''t pull" "$M"
cug 2 "a pull spelled \"gi\"t" '"gi"t pull' "$M"
cug 2 "a switch -C onto main" "git switch -C main origin/main" "$M"
cug 2 "a checkout -B onto main" "git checkout -B main origin/main" "$M"
cug 2 "an update-ref of main" "git update-ref refs/heads/main origin/main" "$M"
cug 2 "a fetch with --update-head-ok" "git fetch --update-head-ok origin main:main" "$M"
cug 2 "a stash in the main checkout" "git stash" "$M"
cug 2 "a pull after a cd within the main checkout" "cd sub && git pull" "$M"
cug 2 "a pull after a cd that fails" "cd $d/no-such-dir; git pull" "$M"
cug 2 "a pull after a subshell's cd (this shell stays)" "(cd $ML && true); git pull" "$M"
cug 2 "a lane's merge after cd - (unknown)" "cd - && git merge origin/main" "$ML"
cug 2 "a lane's pull by -c core.worktree" "git -c core.worktree=$M pull" "$ML"
cug 2 "a lane's pull by GIT_CONFIG_PARAMETERS" "GIT_CONFIG_PARAMETERS=\"'core.worktree=$M'\" git pull" "$ML"
cug 2 "a lane's pull in a \$( … ) inside \${ … }" ": \${x:=\$(git -C $M pull)}" "$ML"
cug 2 "a lane's pull after a cd in a subshell into the main checkout" "(cd $M && git pull)" "$ML"
cug 2 "a lane's update-ref of a branch (refs are shared)" "git update-ref refs/heads/main HEAD" "$ML"
cug 2 "a lane's fetch -u into a branch (refs are shared)" "git fetch -u . HEAD:main" "$ML"
cug 2 "a reset by -C into the main checkout's git dir" "git -C $M/.git reset --soft HEAD" "$ML"
cug 2 "a pull by -C and xargs's placeholder" "echo $M | xargs -I{} git -C {} pull" "$ML"
cug 2 "a pull by -C and parallel's placeholder" "parallel git -C {} pull ::: $M" "$ML"
cug 2 "a cd to xargs's placeholder in sh -c" "echo $M | xargs -I{} sh -c 'cd {} && git pull'" "$ML"
cug 2 "a pull through find -execdir" "find $M -maxdepth 1 -name .git -execdir git pull \\;" "$ML"
cug 2 "a cd to a directory the command makes" "ln -s $M m && cd m && git pull" "$ML"
cug 2 "a pull after a cd that may not run" "[ -d $d/no-such-dir ] && cd $ML; git pull" "$M"
cug 2 "a pull after a cd under if" "if [ -d $d/no-such-dir ]; then cd $ML; fi; git pull" "$M"
cug 2 "a pull after a cd under while" "while false; do cd $ML; done; git pull" "$M"
cug 2 "a pull after a cd || skips (the pull still runs)" "true || cd $ML && git pull" "$M"
cug 2 "a pull after a cd in a case arm" "case x in y) cd $ML;; esac; git pull" "$M"
cug 2 "a pull after a case arm's cd in a subshell" "(case x in y) cd $ML;; esac; true); git pull" "$M"
cug 2 "a pull after a cd in a pipeline" "cd $ML | cat; git pull" "$M"
cug 2 "a pull after a cd sent to the background" "cd $ML & git pull" "$M"
cug 2 "a lane's pull after a bare pushd (the stack swaps)" "pushd; git pull" "$ML"
cug 2 "a lane's pull by fish -c" "fish -c \"cd $M; git pull\"" "$ML"
cug 2 "gh pr checkout in the main checkout" "gh pr checkout 5" "$M"
cug 2 "gh repo sync in the main checkout" "gh -R o/r repo sync" "$M"
cug 2 "without jq: a pull spelled g\\it" 'g\it pull' "$M" "$d/cug-nojq"
rc=0; printf '{"tool_name":"Bash","cwd":"%s","tool_input":{"command":"git pull"}' "$M" | (cd "$M" && PATH="$d/cug-bin:$PATH" sh "$H/catch-up-guard.sh") >/dev/null 2>&1 || rc=$?
expect_rc 2 "$rc" "catch-up-guard blocks: a pull in a payload jq cannot read (read raw)"
cug 0 "a lane's catch-up after cd into its own worktree" "cd $ML && git fetch origin main && git merge origin/main" "$ML"
cug 0 "a lane's catch-up by cd from the main checkout" "cd $ML && git fetch origin main && git merge origin/main" "$M"
cug 0 "a variable command with no git in the main checkout" 'for f in a b; do $f --version; done' "$M"
cug 0 "a variable command with no git in a lane" 'for f in a b; do $f --version; done' "$ML"
cug 0 "a lane's eval of an agent's environment, then a status" 'eval "$(ssh-agent -s)" && git status' "$ML"
cug 0 "a search of the docs for a pull" 'grep -rn "git pull" docs/' "$M"
cug 0 "a stash listing" "git stash list" "$M"
cug 0 "a status through timeout" "timeout 60 git status" "$M"
cug 0 "a fetch into a remote-tracking ref" "git fetch origin main:refs/remotes/origin/main" "$M"
cug 0 "a lane's merge after \${x:=\$( … )} (no computed command)" ': ${x:=$(pwd)}; git merge origin/main' "$ML"
cug 0 "a lane's catch-up after a fetch && cd from the main checkout" "git fetch origin main && cd $ML && git merge origin/main" "$M"
cug 0 "a lane's catch-up after cd || exit from the main checkout" "cd $ML || exit 1; git merge origin/main" "$M"
cug 0 "a lane's catch-up in a subshell from the main checkout" "(cd $ML && git merge origin/main)" "$M"
cug 0 "gh pr checkout in a lane" "gh pr checkout 5" "$ML"
cug 0 "gh pr view in the main checkout" "gh pr view 5 --json state" "$M"
cug 2 "without jq: a pull in the main checkout" "git pull" "$M" "$d/cug-nojq"
cug 0 "without jq: a command with no pull" "git status" "$M" "$d/cug-nojq"
cug 0 "merge-pr's squash-merge from the main checkout" "gh pr merge 5 --squash --delete-branch --body-file f" "$M"
cug 0 "step 5's confirmation with catch-up on" "git fetch origin main && git log -1 --oneline origin/main && git merge-base --is-ancestor abc origin/main" "$M"
cug 0 "a commit message naming a pull" "git commit -m 'merge the pull'" "$M"
cug 0 "an alias that moves nothing" "git st" "$M"
cug 0 "the lane's catch-up line (merge-pr step 2)" "git fetch origin main && git merge origin/main" "$ML"
cug 0 "a lane's pull of its own branch" "git pull --ff-only" "$ML"
cug 0 "a merge aimed at the lane by -C from the main checkout" "git -C $ML merge origin/main" "$M"
cug 0 "a merge in a repository that is not this one" "git -C $d/repo merge origin/main" "$M"
cug 0 "a command with no git" "ls -la" "$M"
rc=0; printf '{"tool_name":"Read","cwd":"%s","tool_input":{"file_path":"x"}}' "$M" | (cd "$M" && PATH="$d/cug-bin:$PATH" sh "$H/catch-up-guard.sh") >/dev/null 2>&1 || rc=$?
expect_rc 0 "$rc" "catch-up-guard allows a tool that is not Bash"
cu /elsewhere on
cug 0 "catch-up on for another project only" "git pull --ff-only origin main" "$M"
cu "$M" off
cug 0 "a pull in the main checkout with catch-up off (merge-pr step 5)" "git pull --ff-only origin main" "$M"
case $cug_out in *"catch-up-guard: catch-up is off for $M"*) ok "catch-up-guard says why it allowed a pull in the main checkout" ;; *) fail "catch-up-guard allowed a main-checkout pull without saying catch-up is off: $cug_out" ;; esac
cu "$M" on 1
cug 0 "a catch-up answer of on with a non-zero exit (counts as off)" "git pull" "$M"
cug 0 "no clauductor on PATH (catch-up counts as off)" "git pull --ff-only origin main" "$M" "$d/cug-noc"
case $cug_out in *"no clauductor on PATH"*) ok "catch-up-guard says it allowed with no clauductor to ask" ;; *) fail "catch-up-guard allowed with no clauductor and did not say so: $cug_out" ;; esac

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

# ── read-by-range ───────────────────────────────────────────────────────────────────────
RBR="$d/rbr"; mkdir -p "$RBR"
awk 'BEGIN{for(i=1;i<=401;i++) print "line " i}' > "$RBR/big.txt"
echo 'short' > "$RBR/small.txt"

rbr_read() {
  if [ -n "${2:-}" ]; then
    jq -cn --arg f "$1" --argjson lim "$2" '{tool_name:"Read", cwd:"'"$RBR"'", tool_input:{file_path:$f, limit:$lim}}'
  else
    jq -cn --arg f "$1" '{tool_name:"Read", cwd:"'"$RBR"'", tool_input:{file_path:$f}}'
  fi
}
rbr_bash() { jq -cn --arg c "$1" '{tool_name:"Bash", cwd:"'"$RBR"'", tool_input:{command:$c}}'; }
rbr() {
  rc=0; printf '%s' "$3" | sh "$H/read-by-range.sh" >/dev/null 2>&1 || rc=$?
  expect_rc "$1" "$rc" "read-by-range $( [ "$1" = 2 ] && echo blocks || echo allows ): $2"
}
rbr 2 "a whole Read of 401 lines" "$(rbr_read "$RBR/big.txt")"
rbr 0 "Read with a limit" "$(rbr_read "$RBR/big.txt" 50)"
rbr 2 "cat of that file" "$(rbr_bash "cat $RBR/big.txt")"
rbr 0 "cat piped to grep" "$(rbr_bash "cat $RBR/big.txt | grep x")"
rbr 0 "head -n 10" "$(rbr_bash "head -n 10 $RBR/big.txt")"
rbr 2 "head -n 500 of 401 lines" "$(rbr_bash "head -n 500 $RBR/big.txt")"
rbr 2 "tail -n 500 of 401 lines" "$(rbr_bash "tail -n 500 $RBR/big.txt")"
rbr 2 "sed -n span over 400 lines" "$(rbr_bash "sed -n '1,500p' $RBR/big.txt")"
rbr 0 "Read with only an offset" "$(jq -cn --arg f "$RBR/big.txt" '{tool_name:"Read", cwd:"'"$RBR"'", tool_input:{file_path:$f, offset:1}}')"
rbr 0 "Read with pages" "$(jq -cn --arg f "$RBR/big.txt" '{tool_name:"Read", cwd:"'"$RBR"'", tool_input:{file_path:$f, pages:"1"}}')"
rbr 0 "a missing file on Read" "$(rbr_read "$RBR/absent.txt")"
rbr 0 "an empty payload" ""

# ── format ─────────────────────────────────────────────────────────────────────────────
rc=0; printf '{"tool_input":{"file_path":"%s"}}' "$R/$T" | sh "$H/format.sh" >/dev/null 2>&1 || rc=$?
expect_rc 0 "$rc" "format.sh is a silent no-op with FORMAT_CMD unset"

# ── conf.sh missing (B13) ──────────────────────────────────────────────────────────────
# A `.` of a missing file exits 2 under dash, which Claude Code reads as a BLOCK: every hook that
# reads the config tests for it first. Run each hook from a checkout with no .claude/lib, under sh
# and, where installed, under dash (Ubuntu's sh).
N="$d/noconf"; new_repo "$N"; mkdir -p "$N/.claude/hooks"
for h in focus-staleness format worktree-hook-drift no-background-gate no-silent-wait cd-guard; do cp "$H/$h.sh" "$N/.claude/hooks/"; done
for shell in sh dash; do
  command -v "$shell" >/dev/null 2>&1 || continue
  for h in focus-staleness format worktree-hook-drift no-background-gate no-silent-wait cd-guard; do
    rc=0; printf '%s' "$spawn" | (cd "$N" && "$shell" "$N/.claude/hooks/$h.sh") >/dev/null 2>&1 || rc=$?
    expect_rc 0 "$rc" "[$shell] $h.sh with conf.sh missing does not block (exit 0)"
  done
done
for h in "$H"/*.sh; do
  grep -q 'lib/conf.sh"$' "$h" || continue
  awk '/^[[:space:]]*\. .*lib\/conf\.sh"$/ { if (!guarded) bad = 1 } /-f .*lib\/conf\.sh"/ { guarded = 1 } END { exit bad }' "$h" \
    && ok "$(basename "$h") tests for conf.sh before sourcing it" || fail "$(basename "$h") sources conf.sh without testing it exists first (a dash exit 2 reads as a block)"
done

# ── no forced worktree removal, no pattern kill (ST-14) ────────────────────────────────────────
wt() {
  rc=0; printf '%s' "$2" | sh "$H/no-worktree-force-remove.sh" >/dev/null 2>&1 || rc=$?
  expect_rc "$1" "$rc" "worktree-remove $( [ "$1" = 2 ] && echo blocks || echo allows ): $3"
}
wtp() { jq -cn --arg c "$1" '{tool_name:"Bash", tool_input:{command:$c}}'; }
wt 2 "$(wtp 'git worktree remove --force x')" "git worktree remove --force"
wt 2 "$(wtp 'git worktree remove x -f')" "a force flag after the path"
wt 0 "$(wtp 'git worktree remove x')" "a plain removal"
wt 0 "$(cd "$ROOT" && wtp 'sh .claude/machine-quiet.sh')" "machine-quiet.sh, whose own -f is not read"
wt 0 "$(wtp "echo 'git worktree remove --force x'")" "a quoted mention"
pk() {
  rc=0; printf '%s' "$2" | sh "$H/no-pattern-kill.sh" >/dev/null 2>&1 || rc=$?
  expect_rc "$1" "$rc" "pattern-kill $( [ "$1" = 2 ] && echo blocks || echo allows ): $3"
}
pk 2 "$(wtp '/usr/bin/pkill -f sleep')" "a path to pkill"
pk 2 "$(wtp 'pkill -f sleep')" "the bare name, so a missing wrapper still refuses"
pk 2 "$(wtp 'command -p pkill')" "command -p, which skips the wrapper"
pk 2 "$(wtp 'env PATH=/usr/bin:/bin pkill')" "a rebuilt PATH"
pk 0 "$(wtp 'kill 1234')" "kill by pid"
pk 0 "$(wtp 'echo pkill is refused')" "the name in an argument"
pk 0 "$(wtp 'grep /usr/bin/pkill /dev/null')" "a path mentioned as an argument"
pk 0 "$(wtp 'gh pr create --body "see /usr/bin/pkill"')" "a path inside a quoted argument"
rc=0; "$H/../bin/pkill" -f sleep >/dev/null 2>&1 || rc=$?
expect_rc 2 "$rc" "the pkill wrapper refuses every call"
rc=0; "$H/../bin/killall" sleep >/dev/null 2>&1 || rc=$?
expect_rc 2 "$rc" "the killall wrapper refuses every call"

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
regs=$(jq -r '(.hooks // {}) | to_entries[] | .key as $e | .value[] | .hooks[]? | select(.type == "command") | [$e, .command] | @tsv' "$SETTINGS" 2>/dev/null) \
  || { fail "settings.json does not parse, so no hook it registers can be checked (or run)"; finish; }
sl=$(jq -r '.statusLine.command // empty' "$SETTINGS")
# Every .claude/ path, and every script path, in a hook or status-line command goes through
# CLAUDE_PROJECT_DIR: `sh scripts/hooks/guard.sh` fails to launch from a subdirectory just the same.
# scripts CMD: each script path in CMD, one per line. The command is split into words at blanks and
# shell operators first (; | & ( ) < > and backtick), then a word is a script only if its extension
# ENDS it (a closing quote aside): settings.json, tsconfig.json or tools.shell_guard is not one.
# A word that starts with a scheme (https://…/install.sh, npm:pkg/x.ts) is a URL, not a path.
scripts() {
  printf '%s\n' "$1" | tr -s ' \t;|&()<>`' '\n\n\n\n\n\n\n\n\n\n' \
    | grep -E '^["'"'"']?[A-Za-z0-9_./${}:"-]*[A-Za-z0-9_.-]+\.(sh|bash|js|mjs|cjs|py|rb|ts)["'"'"']?$' \
    | grep -vE '^["'"'"']?[A-Za-z][A-Za-z0-9+.-]*:' | sed 's/^'"'"'//; s/["'"'"']$//' || true
}
relative() {  # relative CMD: prints CMD if a .claude/ or script path in it is not reached through CLAUDE_PROJECT_DIR
  { printf '%s\n' "$1" | grep -oE '[^[:space:]]*\.claude/' | sed 's|\.claude/$||'
    scripts "$1" | sed 's|[^/]*$||'; } \
  | while IFS= read -r pre; do
    case $pre in /*|\"/*|\'/*) continue ;; esac  # an absolute path launches from anywhere
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
no-background-gate.sh	2	out:BLOCKED by no-background-gate: a subagent started the gate with run_in_background	{"tool_name":"Bash","agent_id":"check","tool_input":{"command":"@GATE_WAIT@","run_in_background":true}}	a subagent's gate wait in the background
no-silent-wait.sh	0	file:@REPO@/.git/clauductor-wait/hooks-check/tasks/check	{"hook_event_name":"PostToolUse","session_id":"hooks-check","agent_id":"check","tool_name":"Bash","tool_response":{"backgroundTaskId":"b1"}}	a subagent's backgrounded command is recorded for its stop to read
worktree-hook-drift.sh	0	out:worktree-hook-drift: CANNOT CHECK	{"tool_name":"Agent","tool_input":{"prompt":"p","isolation":"worktree"}}	a worktree spawn with no origin/main to compare: allowed, and says so
cd-guard.sh	0	file:@REPO@/.git/clauductor-cd-guard/hooks-check/holds/check	{"hook_event_name":"PostToolUse","session_id":"hooks-check","tool_name":"Workflow","tool_input":{"name":"build-change"},"tool_response":{"taskId":"check"}}	a build-change run is recorded as holding the worktree it started in
catch-up-guard.sh	0	out:catch-up-guard: (no clauductor on PATH|catch-up is off)	{"tool_name":"Bash","tool_input":{"command":"git pull --ff-only origin main"}}	a pull in the main checkout, catch-up off or unaskable: allowed, and says so
read-by-range.sh	2	out:BLOCKED by read-by-range:	{"tool_name":"Read","tool_input":{"file_path":"@REPO@/big.txt"}}	a whole Read of a file over 400 lines
no-worktree-force-remove.sh	2	out:BLOCKED by no-worktree-force-remove:	{"tool_name":"Bash","tool_input":{"command":"git worktree remove --force x"}}	a forced worktree removal
no-pattern-kill.sh	2	out:BLOCKED by no-pattern-kill:	{"tool_name":"Bash","tool_input":{"command":"/usr/bin/pkill -f sleep"}}	a path to pkill, which skips the wrapper
pattern-kill-path.sh	0	out:pattern-kill-path:	{"hook_event_name":"SessionStart","session_id":"hooks-check"}	SessionStart puts .claude/bin first, or says it could not
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
awk 'BEGIN{for(i=1;i<=401;i++) print "line " i}' > "$P/big.txt"
git -C "$P" add src/tracked.ts big.txt && git -C "$P" commit -qm seed
P=$(cd "$P" && pwd -P)
printf '#!/bin/sh\ntouch "%s/formatted.stamp"\n' "$P" > "$P/fmt.sh"
{ cat "$ROOT/.claude/project.conf" 2>/dev/null; printf '\nFORMAT_CMD="sh %s/fmt.sh"\nFORMAT_EXT="ts"\n' "$P"; } > "$P/.claude/project.conf"

# Every registered command that runs a script file, wherever it lives and whatever its language.
launching=$(printf '%s\n' "$regs" | while IFS="$(printf '\t')" read -r ev cmd; do [ -n "$(scripts "$cmd")" ] && printf '%s\t%s\n' "$ev" "$cmd"; done)
n=$(printf '%s\n' "$launching" | grep -c . || true)
if [ -n "$plugin" ]; then ok "the model's hooks are the plugin's ($(jq '[.hooks[][].hooks[]] | length' "$CLAUDUCTOR_FW/hooks/hooks.json") in its hooks.json); settings.json registers $n of this project's own"
else [ "$n" -ge 3 ] && ok "settings.json registers $n hook command(s) (non-vacuity)" || fail "settings.json registers only $n hook command(s): the table below would pass vacuously"; fi
script_of() { scripts "$1" | head -1 | sed 's|.*/||'; }
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
    # A file proof made by an earlier launch would pass this one vacuously: remove it first.
    case $proof in file:*) rm -f "$(printf '%s' "${proof#file:}" | sed "s|@REPO@|$P|g")" ;; esac
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
for c in "sh \"\$CLAUDE_PROJECT_DIR\"/$D/hooks/x.sh" "sh \"\${CLAUDE_PROJECT_DIR:-.}\"/$D/statusline.sh" "sh \$CLAUDE_PROJECT_DIR/$D/x.sh" "node \"\$CLAUDE_PROJECT_DIR\"/scripts/hooks/x.mjs" "sh /opt/hooks/x.sh" "npx biome format --write" "jq -e . \"\$CLAUDE_PROJECT_DIR\"/$D/settings.json" "npx tsc -p tsconfig.json --noEmit" "curl -fsSL https://example.com/install.sh | sh" "sh \"/opt/hooks/x.sh\"" "sh '/opt/p/$D/x.sh'"; do
  [ -z "$(relative "$c")" ] && ok "the path rule passes '$c'" || fail "the path rule flagged '$c', which is correct"
done
# The table covers every registered script, not only .claude/hooks/*.sh.
[ "$(script_of "node \"\$CLAUDE_PROJECT_DIR\"/scripts/hooks/x.mjs --flag")" = x.mjs ] && [ "$(script_of "sh \"\$CLAUDE_PROJECT_DIR\"/x.sh")" = x.sh ] && [ "$(script_of "sh \"\$CLAUDE_PROJECT_DIR\"/g.sh>/dev/null")" = g.sh ] && [ "$(script_of "sh \"\${CLAUDE_PROJECT_DIR:-.}\"/scripts/hooks/guard.sh")" = guard.sh ] && [ -z "$(script_of 'npx biome format --write')" ] \
  && [ -z "$(script_of "jq -e . \"\$CLAUDE_PROJECT_DIR\"/$D/settings.json")" ] && [ -z "$(script_of 'npx tsc -p tsconfig.json')" ] && [ -z "$(script_of 'python3 -m tools.shell_guard')" ] \
  && ok "a registered script in any directory or language gets a row of its own (x.mjs); an inline command needs none" || fail "script_of: '$(script_of "node \"\$CLAUDE_PROJECT_DIR\"/scripts/hooks/x.mjs")'"
finish
