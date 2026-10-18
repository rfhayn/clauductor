#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The status line posts its stdin to the panel ONLY while ~/.clauductor/panel/pid names a live
# process (a positive integer), and otherwise makes no call; in every case it prints its line and
# exits 0. The post is observed with a curl stand-in on PATH that records its arguments, so no
# server or network is needed. Also: status-write.sh writes the file the status line reads.
. "$(dirname "$0")/lib.sh"
need jq

d=$(scratch)
mkdir -p "$d/bin" "$d/repo"
# The stand-in takes two seconds before it records, longer than the old fixed wait: a run() that
# guesses how long to wait, rather than waiting for the post to finish, fails here every time
# instead of only under load. It records into its case's HOME, and while $d/hold exists it holds
# on (the deadline case below).
cat > "$d/bin/curl" <<EOF
#!/bin/sh
cat >/dev/null
while [ -f "$d/hold" ]; do sleep 0.1; done
sleep 2
echo "\$*" >> "\$HOME/posts"
EOF
chmod +x "$d/bin/curl"
new_repo "$d/repo"
git -C "$d/repo" commit -q --allow-empty -m init

input=$(jq -cn --arg c "$d/repo" '{session_id:"s", cwd:$c, context_window:{used_percentage:42.4}}')
sl="$CLAUDUCTOR_FW/statusline.sh"

# The live panel is this check's own shell: alive for as long as any case runs, where a `sleep 5`
# was one more guess at how long the case would take.
live=$$
sh -c 'exit 0' & dead=$!; wait "$dead" 2>/dev/null

# The post is backgrounded, so posts are counted only once everything the status line started has
# exited. A fixed `sleep 1` guessed instead, and a guess can lose under a loaded gate ("0 posts",
# OPS-73). The status line runs holding fd 3, the write end of a pipe, and everything it starts
# inherits it (the post's `>/dev/null 2>&1` leaves fd 3 alone), so the reader sees end-of-file
# only after the last of them exits: a post that was made has landed, and a "no post" case has
# none still on its way. The deadline (10s, times CLAUDUCTOR_TEST_SLOW) only keeps a stray holder
# of fd 3 from hanging the check, and reaching it fails the case. So that it also frees the
# check's caller, the case runs with its output and errors both on /dev/null: run.sh reads a check
# through `$(sh "$f" 2>&1)`, and a holder that kept the check's stderr would keep the gate waiting
# past the deadline. Each case has its own directory, so a holder still running past its deadline
# writes into its own case's files, never the next one's.
ticks=$(awk -v f="${CLAUDUCTOR_TEST_SLOW:-1}" 'BEGIN { f += 0; if (!(f >= 1)) f = 1; printf "%d", 100 * f }')
run() {  # run PID_CONTENT (or "-" for no pid file): prints "<exit> <posts> <case dir> <line>"
  c=$(mktemp -d "$d/case.XXXXXX"); home="$c/home"; mkdir -p "$home/.clauductor/panel"
  echo 4999 > "$home/.clauductor/panel/port"
  [ "$1" = "-" ] || printf '%s\n' "$1" > "$home/.clauductor/panel/pid"
  ( printf '%s' "$input" | { HOME="$home" PATH="$d/bin:$PATH" sh "$sl" 3>&1 > "$c/line"; echo $? > "$c/rc"; } \
      | cat > /dev/null; : > "$c/eof" ) > /dev/null 2>&1 &
  i=0; while [ ! -f "$c/eof" ] && [ "$i" -lt "$ticks" ]; do sleep 0.1; i=$((i + 1)); done
  rc=$(cat "$c/rc" 2>/dev/null); [ -f "$c/eof" ] || rc="still-running-after-$((ticks / 10))s"
  n=0; [ -f "$home/posts" ] && n=$(wc -l < "$home/posts" | tr -d ' ')
  echo "$rc $n $c $(cat "$c/line" 2>/dev/null)"
}

set -- $(run "$live"); [ "$1" = 0 ] && [ "$2" = 1 ] && ok "posts once to a live panel" || fail "live panel: exit $1, $2 posts"
posts="$3/home/posts"
grep -q "127.0.0.1:4999/status" "$posts" 2>/dev/null && ok "posts to 127.0.0.1:<port>/status" || fail "post went elsewhere: $(cat "$posts" 2>/dev/null)"
for c in "$dead:a dead pid (stale files)" "-:no pid file" "-1:pid -1" "0:pid 0" "12 34:a pid that is not a number" "0123:a pid with a leading zero"; do
  p=${c%%:*}; why=${c#*:}
  set -- $(run "$p")
  [ "$1" = 0 ] && [ "$2" = 0 ] && ok "no post with $why" || fail "$why: exit $1, $2 posts"
done

# The deadline frees the check's caller, not only the check (review of #147). The post is held
# past a one-second deadline until $d/hold goes, six seconds on (times CLAUDUCTOR_TEST_SLOW), and
# the case is read the way run.sh reads a check, `$(… 2>&1)`: that read must come back at the
# deadline, well before the release. With the case's errors left on the check's stderr, it came
# back only at the release.
rel=$(awk -v f="${CLAUDUCTOR_TEST_SLOW:-1}" 'BEGIN { f += 0; if (!(f >= 1)) f = 1; printf "%d", 6 * f }')
: > "$d/hold"
( sleep "$rel"; rm -f "$d/hold" ) > /dev/null 2>&1 &
t0=$(date +%s)
out=$(ticks=10; run "$live" 2>&1)
took=$(( $(date +%s) - t0 ))
case "$out" in still-running-after-1s*) ok "a case still running at its deadline fails" ;; *) fail "a case held past its deadline reported: $out" ;; esac
[ "$took" -lt $((rel - 2)) ] && ok "the deadline frees the check's reader (${took}s, release at ${rel}s)" \
  || fail "the check's reader waited ${took}s for a case with a one-second deadline (release at ${rel}s)"
mkdir -p "$d/home.-"

line=$(printf '%s' "$input" | HOME="$d/home.-" PATH="$d/bin:$PATH" sh "$sl")
case "$line" in *"42% ctx"*) ok "renders the context %: $line" ;; *) fail "no context % in: $line" ;; esac
case "$line" in *"[main]"*) ok "falls back to [branch] with no focus set" ;; *) fail "no [branch] fallback in: $line" ;; esac

( cd "$d/repo" && HOME="$d/home.-" sh "$CLAUDUCTOR_FW/status-write.sh" "doing the thing" >/dev/null )
line=$(printf '%s' "$input" | HOME="$d/home.-" PATH="$d/bin:$PATH" sh "$sl")
case "$line" in *"doing the thing"*) ok "shows the focus status-write.sh set" ;; *) fail "focus not shown: $line" ;; esac

# The focus lives in the repository, not under HOME: the sandbox lets a command write the
# repository and refuses ~/.claude, so a focus kept there was never set from a sandboxed session
# (OPS-75). A read-only HOME stands in for the sandbox.
mkdir -p "$d/ro"; chmod 555 "$d/ro"
( cd "$d/repo" && HOME="$d/ro" sh "$CLAUDUCTOR_FW/status-write.sh" "sandboxed focus" >/dev/null 2>&1 )
# The file itself, not the exit code: the old script exited 0 whatever happened to its write.
ff=$(cd "$d/repo" && HOME="$d/ro" ROOT="$ROOT" sh -c '. "$CLAUDUCTOR_FW/lib/conf.sh" && focus_file main')
case "$ff" in "$d/ro"/*) fail "the focus file is under HOME: $ff" ;; esac
[ "$(cat "$ff" 2>/dev/null)" = "sandboxed focus" ] && ok "status-write sets the focus with HOME unwritable" || fail "status-write did not write $ff with HOME unwritable"
line=$(printf '%s' "$input" | HOME="$d/ro" PATH="$d/bin:$PATH" sh "$sl")
case "$line" in *"sandboxed focus"*) ok "shows a focus set with HOME unwritable" ;; *) fail "focus set with HOME unwritable not shown: $line" ;; esac
chmod 755 "$d/ro"

# A focus that cannot be written is an error, never "focus set": a plain file where the focus's
# folder belongs blocks the write.
fdir=$(cd "$d/repo" && HOME="$d/home.-" ROOT="$ROOT" sh -c '. "$CLAUDUCTOR_FW/lib/conf.sh" && dirname "$(focus_file blocked)"')
rm -rf "$fdir"; mkdir -p "$(dirname "$fdir")"; : > "$fdir"
rc=0; out=$(cd "$d/repo" && HOME="$d/home.-" sh "$CLAUDUCTOR_FW/status-write.sh" "lost" blocked 2>&1) || rc=$?
[ "$rc" != 0 ] && ok "status-write exits non-zero when it cannot write" || fail "status-write exited 0 on a failed write: $out"
case "$out" in *"focus set"*) fail "status-write said 'focus set' on a failed write: $out" ;; *) ok "status-write does not claim a failed write" ;; esac
rm -f "$fdir"
finish
