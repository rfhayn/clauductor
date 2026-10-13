#!/bin/sh
# The quota reading (OPS-67): the status line saves Claude Code's rate limits to
# ~/.clauductor/quota-reading.json on every refresh that carries them, with or without the panel,
# and lib/quota.sh reads it back. Each case runs the real status line under a scratch HOME (never
# the real one) and no panel:
#   - a status-line JSON with rate_limits writes the file, in the panel's shape (state.Quota);
#   - a later one that leaves a window out keeps that window (as the panel's foldQuota does);
#   - one with no rate_limits leaves the old file untouched;
#   - the file is 0600, and is replaced by a rename, never rewritten in place;
#   - a HOME where the file cannot be written still prints the line, exit 0, nothing on stderr;
#   - the reader reports a window whose reset has passed, or that has no reset time, as no
#     reading: never the old number (today's bug: a 9-day-old quota.json read as current).
. "$(dirname "$0")/lib.sh"
need jq

d=$(scratch)
new_repo "$d/repo"
git -C "$d/repo" commit -q --allow-empty -m init
sl="$ROOT/.claude/statusline.sh"
now=$(date +%s)
future=$((now + 3600)); past=$((now - 60)); later=$((now + 7200))

# status HOME JSON: run the status line; prints "<exit> <stderr bytes>" then the line.
status() {
  out=$(printf '%s' "$2" | HOME="$1" sh "$sl" 2> "$d/err"); rc=$?
  printf '%s %s\n%s\n' "$rc" "$(wc -c < "$d/err" | tr -d ' ')" "$out"
}
# settle FILE: wait (up to 5s) for the backgrounded write to land, and for no temp file beside it.
settle() {
  i=0
  while [ "$i" -lt 50 ]; do
    [ -f "$1" ] && [ -z "$(find "$(dirname "$1")" -name '.quota-reading.*' 2>/dev/null)" ] && return 0
    sleep 0.1; i=$((i + 1))
  done
  return 1
}
in_json() {  # in_json RATE_LIMITS_JSON (or "" for none)
  if [ -n "$1" ]; then
    jq -cn --arg c "$d/repo" --argjson r "$1" '{session_id:"s", cwd:$c, context_window:{used_percentage:30}, rate_limits:$r}'
  else
    jq -cn --arg c "$d/repo" '{session_id:"s", cwd:$c, context_window:{used_percentage:30}}'
  fi
}

# ── 1. A refresh with rate_limits writes the file ───────────────────────────────────────────────
H="$d/home1"; mkdir -p "$H"; F="$H/.clauductor/quota-reading.json"
rl=$(jq -cn --argjson f "$future" --argjson l "$later" \
  '{five_hour:{used_percentage:12.5, resets_at:$f}, seven_day:{used_percentage:40, resets_at:$l}, nimbus:{used_percentage:3, resets_at:null},
    broken:"yes", no_pct:{resets_at:1}, "five-hour":{used_percentage:1}, ("k" * 65):{used_percentage:1},
    frac:{used_percentage:1, resets_at:1.5}, str:{used_percentage:1, resets_at:"soon"}}')
res=$(status "$H" "$(in_json "$rl")")
[ "$(echo "$res" | head -1)" = "0 0" ] && ok "the status line exits 0, nothing on stderr" || fail "status line: $(echo "$res" | head -1)"
case "$(echo "$res" | sed -n 2p)" in *"30% ctx"*) ok "...and prints its line" ;; *) fail "no line: $res" ;; esac
if settle "$F"; then ok "a refresh with rate_limits writes ~/.clauductor/quota-reading.json"
else fail "no quota reading written at $F"; fi
got=$(jq -c '[.windows[] | [.key, .pct, .resetsAt]]' "$F" 2>/dev/null)
want=$(jq -cn --argjson f "$future" --argjson l "$later" '[["five_hour",12.5,$f],["nimbus",3,null],["seven_day",40,$l]]')
[ "$got" = "$want" ] && ok "...windows[].{key,pct,resetsAt} as the panel keeps them; bad keys, no pct, a non-integer reset dropped" || fail "windows: $got (want $want)"
jq -e '.windows[] | select(.key == "nimbus") | has("resetsAt") | not' "$F" >/dev/null 2>&1 && ok "...a null resets_at is left out, not saved as null" || fail "nimbus: $(jq -c '.windows' "$F" 2>/dev/null)"
at=$(jq -r '.at' "$F" 2>/dev/null)
[ "$at" -ge $((now * 1000)) ] 2>/dev/null && [ "$at" -le $(( ($(date +%s) + 1) * 1000)) ] && ok "...and when it was read (at, unix ms)" || fail "at: $at"
extra=$(jq -r '[keys[] | select(. != "windows" and . != "at")] | join(",")' "$F")
[ -z "$extra" ] && ok "...and nothing else (no session, account or token)" || fail "extra fields: $extra"

# ── 2. The file's mode, and an atomic replace ───────────────────────────────────────────────────
mode=$(ls -l "$F" | cut -c1-10)
[ "$mode" = "-rw-------" ] && ok "the file is 0600" || fail "the file's mode is $mode"
chmod 644 "$F"; ln "$F" "$d/link"; before=$(cat "$F")
rl2=$(jq -cn --argjson f "$future" '{five_hour:{used_percentage:20, resets_at:$f}}')
status "$H" "$(in_json "$rl2")" >/dev/null
i=0; while [ "$i" -lt 50 ] && [ "$(jq -r '.windows[0].pct' "$F" 2>/dev/null)" != 20 ]; do sleep 0.1; i=$((i + 1)); done
settle "$F"
[ "$(cat "$d/link")" = "$before" ] && [ "$(cat "$F")" != "$before" ] \
  && ok "the file is replaced by a rename (a hard link to the old one still holds the old reading)" \
  || fail "the file was rewritten in place, or not at all"
mode=$(ls -l "$F" | cut -c1-10)
[ "$mode" = "-rw-------" ] && ok "a replaced file is 0600 again" || fail "the replaced file's mode is $mode"
[ -z "$(find "$H/.clauductor" -name '.quota-reading.*')" ] && ok "no temp file is left behind" || fail "a temp file was left: $(ls -a "$H/.clauductor")"
got=$(jq -c '[.windows[] | [.key, .pct]]' "$F")
[ "$got" = '[["five_hour",20],["nimbus",3],["seven_day",40]]' ] && ok "a window the refresh left out is kept (the panel's per-window merge)" || fail "merge: $got"

# ── 3. No rate_limits: the old file stays ───────────────────────────────────────────────────────
# Its own seeded file, so the case cannot pass on a file that was never there; quota_save itself,
# in the foreground (the status line's only call), so no late write can slip past a fixed wait.
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/quota.sh"
H="$d/home-keep"; mkdir -p "$H/.clauductor"; F="$H/.clauductor/quota-reading.json"
printf '{"windows":[{"key":"five_hour","pct":7,"resetsAt":%s}],"at":1}\n' "$future" > "$F"
before=$(cat "$F"); ino=$(ls -i "$F" | awk '{print $1}')
for r in "" '{}' '7' '{"five_hour":{"resets_at":1}}'; do
  HOME="$H" quota_save "$(in_json "$r")"
  [ "$(cat "$F")" = "$before" ] && [ "$(ls -i "$F" | awk '{print $1}')" = "$ino" ] \
    && ok "rate_limits ${r:-absent}: the old file is untouched" || fail "rate_limits ${r:-absent} changed the file: $(cat "$F")"
done
# A temp file a killed refresh left is cleared by a later save; a fresh one (a refresh under way) is not.
touch "$H/.clauductor/.quota-reading.fresh"
: > "$H/.clauductor/.quota-reading.stale"; touch -t 202001010000 "$H/.clauductor/.quota-reading.stale"
HOME="$H" quota_save "$(in_json "$rl2")"
[ ! -e "$H/.clauductor/.quota-reading.stale" ] && [ -e "$H/.clauductor/.quota-reading.fresh" ] \
  && ok "a save clears a temp file older than 5 minutes, and leaves a fresh one" || fail "temp files: $(ls -a "$H/.clauductor")"
rm -f "$H/.clauductor/.quota-reading.fresh"

# ── 3b. The save never holds the line ───────────────────────────────────────────────────────────
# A save that takes 5s (a mktemp that sleeps) must not delay the printed line: the status line
# backgrounds it AND gives it no copy of the stdout Claude Code reads, or $(…) would wait for it.
H="$d/home-slow"; mkdir -p "$H" "$d/slowbin"
printf '#!/bin/sh\nsleep 5\nexec %s "$@"\n' "$(command -v mktemp)" > "$d/slowbin/mktemp"; chmod +x "$d/slowbin/mktemp"
t0=$(date +%s)
line=$(printf '%s' "$(in_json "$rl")" | HOME="$H" PATH="$d/slowbin:$PATH" sh "$sl" 2>/dev/null)
t1=$(date +%s)
case "$line" in *"30% ctx"*) [ $((t1 - t0)) -lt 3 ] && ok "a 5s save does not delay the line ($((t1 - t0))s)" || fail "the line waited $((t1 - t0))s for the save" ;;
  *) fail "slow save: no line: $line" ;; esac

# ── 4. A write that cannot happen still prints the line ─────────────────────────────────────────
for c in file ro; do
  H2="$d/home-$c"; mkdir -p "$H2"
  case $c in
    file) : > "$H2/.clauductor"; why="~/.clauductor is a file" ;;   # fails even for root
    ro) mkdir -p "$H2/.clauductor"; echo old > "$H2/.clauductor/quota-reading.json"; chmod 555 "$H2/.clauductor"; why="~/.clauductor is read-only" ;;
  esac
  res=$(status "$H2" "$(in_json "$rl")")
  case "$(echo "$res" | head -1) $(echo "$res" | sed -n 2p)" in
    "0 0 "*"30% ctx"*) ok "$why: the line still prints, exit 0, nothing on stderr" ;;
    *) fail "$why: $res" ;;
  esac
  sleep 0.5
  if [ "$c" = ro ]; then
    # Root writes through a read-only mode, so there the old file's survival is not this case's to show.
    if [ "$(cat "$H2/.clauductor/quota-reading.json")" = old ]; then ok "...and the old file is left in place"
    elif [ "$(id -u)" = 0 ]; then ok "...(as root a read-only directory is writable: the file case above holds the line)"
    else fail "$why: the old file changed"; fi
    chmod 755 "$H2/.clauductor"
  fi
done

# ── 5. The reader ───────────────────────────────────────────────────────────────────────────────
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/quota.sh"
H3="$d/home3"; mkdir -p "$H3/.clauductor"; R="$H3/.clauductor/quota-reading.json"
jq -cn --argjson f "$future" --argjson p "$past" --argjson at "$((now * 1000))" \
  '{windows:[{key:"five_hour",pct:91,resetsAt:$p},{key:"seven_day",pct:40,resetsAt:$f},{key:"seven_day_opus",pct:5}], at:$at}' > "$R"
out=$(HOME="$H3" quota_read)
want=$(printf 'five_hour\t-\t%s\nseven_day\t40\t%s\nseven_day_opus\t-\t-' "$past" "$future")
[ "$out" = "$want" ] && ok "quota_read: a past-reset window and one with no reset time read as no reading (-)" || fail "quota_read printed: $out"
stale=$(printf '%s\n' "$out" | awk -F '\t' '$2 == "91"')
[ -z "$stale" ] && ok "...never the stale number" || fail "quota_read reported a stale 91: $stale"
v=$(HOME="$H3" quota_pct seven_day); rc=$?
[ "$rc" = 0 ] && [ "$v" = 40 ] && ok "quota_pct seven_day: 40 (a current reading)" || fail "quota_pct seven_day: $v (exit $rc)"
v=$(HOME="$H3" quota_pct five_hour); rc=$?
[ "$rc" = 1 ] && [ -z "$v" ] && ok "quota_pct five_hour: no reading, exit 1 (its reset has passed)" || fail "quota_pct five_hour: '$v' (exit $rc)"
v=$(HOME="$H3" quota_pct nimbus); rc=$?
[ "$rc" = 1 ] && [ -z "$v" ] && ok "quota_pct of a window never read: exit 1" || fail "quota_pct nimbus: '$v' (exit $rc)"
v=$(HOME="$H3" QUOTA_NOW=$((future + 1)) quota_pct seven_day); rc=$?
[ "$rc" = 1 ] && [ -z "$v" ] && ok "QUOTA_NOW past seven_day's reset: no reading (the reader's clock is injectable)" || fail "QUOTA_NOW: '$v' (exit $rc)"
v=$(HOME="$H3" QUOTA_NOW=$((past - 1)) quota_pct five_hour); rc=$?
[ "$rc" = 0 ] && [ "$v" = 91 ] && ok "QUOTA_NOW before five_hour's reset: 91 counts" || fail "QUOTA_NOW early: '$v' (exit $rc)"
v=$(HOME="$H3" quota_at); rc=$?
[ "$rc" = 0 ] && [ "$v" = "$((now * 1000))" ] && ok "quota_at: when the reading was saved (unix ms)" || fail "quota_at: '$v' (exit $rc)"
v=$(HOME="$d/nohome" quota_at); rc=$?
[ "$rc" = 1 ] && [ -z "$v" ] && ok "quota_at with no file: exit 1" || fail "quota_at no file: '$v' (exit $rc)"
v=$(HOME="$d/nohome" quota_read); rc=$?
[ "$rc" = 1 ] && [ -z "$v" ] && ok "no file: quota_read prints nothing, exit 1" || fail "no file: '$v' (exit $rc)"
echo '{"windows":' > "$R"
v=$(HOME="$H3" quota_read 2>&1); rc=$?
[ "$rc" = 1 ] && [ -z "$v" ] && ok "a corrupt file: quota_read prints nothing, exit 1" || fail "corrupt file: '$v' (exit $rc)"
finish
