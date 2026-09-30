#!/bin/sh
# The status line posts its stdin to the panel ONLY while ~/.clauductor/panel/pid names a live
# process (a positive integer), and otherwise makes no call; in every case it prints its line and
# exits 0. The post is observed with a curl stand-in on PATH that records its arguments, so no
# server or network is needed. Also: status-write.sh writes the file the status line reads.
. "$(dirname "$0")/lib.sh"
need jq

d=$(scratch)
mkdir -p "$d/bin" "$d/repo"
cat > "$d/bin/curl" <<EOF
#!/bin/sh
cat >/dev/null
echo "\$*" >> "$d/posts"
EOF
chmod +x "$d/bin/curl"
new_repo "$d/repo"
git -C "$d/repo" commit -q --allow-empty -m init

input=$(jq -cn --arg c "$d/repo" '{session_id:"s", cwd:$c, context_window:{used_percentage:42.4}}')
sl="$ROOT/.claude/statusline.sh"

sleep 5 & live=$!
sh -c 'exit 0' & dead=$!; wait "$dead" 2>/dev/null

run() {  # run PID_CONTENT (or "-" for no pid file): prints "<posts> <line>"
  home="$d/home.$1"; rm -rf "$home"; mkdir -p "$home/.clauductor/panel"
  echo 4999 > "$home/.clauductor/panel/port"
  [ "$1" = "-" ] || printf '%s\n' "$1" > "$home/.clauductor/panel/pid"
  rm -f "$d/posts"
  line=$(printf '%s' "$input" | HOME="$home" PATH="$d/bin:$PATH" sh "$sl"); rc=$?
  sleep 1   # the post is backgrounded
  n=0; [ -f "$d/posts" ] && n=$(wc -l < "$d/posts" | tr -d ' ')
  echo "$rc $n $line"
}

set -- $(run "$live"); [ "$1" = 0 ] && [ "$2" = 1 ] && ok "posts once to a live panel" || fail "live panel: exit $1, $2 posts"
grep -q "127.0.0.1:4999/status" "$d/posts" 2>/dev/null && ok "posts to 127.0.0.1:<port>/status" || fail "post went elsewhere: $(cat "$d/posts" 2>/dev/null)"
for c in "$dead:a dead pid (stale files)" "-:no pid file" "-1:pid -1" "0:pid 0" "12 34:a pid that is not a number" "0123:a pid with a leading zero"; do
  p=${c%%:*}; why=${c#*:}
  set -- $(run "$p")
  [ "$1" = 0 ] && [ "$2" = 0 ] && ok "no post with $why" || fail "$why: exit $1, $2 posts"
done
kill "$live" 2>/dev/null

line=$(printf '%s' "$input" | HOME="$d/home.-" PATH="$d/bin:$PATH" sh "$sl")
case "$line" in *"42% ctx"*) ok "renders the context %: $line" ;; *) fail "no context % in: $line" ;; esac
case "$line" in *"[main]"*) ok "falls back to [branch] with no focus set" ;; *) fail "no [branch] fallback in: $line" ;; esac

( cd "$d/repo" && HOME="$d/home.-" sh "$ROOT/.claude/status-write.sh" "doing the thing" >/dev/null )
line=$(printf '%s' "$input" | HOME="$d/home.-" PATH="$d/bin:$PATH" sh "$sl")
case "$line" in *"doing the thing"*) ok "shows the focus status-write.sh set" ;; *) fail "focus not shown: $line" ;; esac
finish
