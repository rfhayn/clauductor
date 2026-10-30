#!/bin/sh
# The quota reading (OPS-67): the status line saves Claude Code's rate limits to
# ~/.stridal/quota-reading.json on every refresh that carries them, with or without the panel,
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
now=$(date +%s)  # wall-clock: the origin of the reset times, a fixture, and the floor of the reading time below
future=$((now + 3600)); past=$((now - 60)); later=$((now + 7200))

# status HOME JSON: run the status line; prints "<exit> <stderr bytes>" then the line.
status() {
  out=$(printf '%s' "$2" | HOME="$1" sh "$sl" 2> "$d/err"); rc=$?
  printf '%s %s\n%s\n' "$rc" "$(wc -c < "$d/err" | tr -d ' ')" "$out"
}
# settle FILE: wait (up to 30s, times STRIDAL_TEST_SLOW) for the backgrounded write to land, and
# for no temp file beside it. A write that lands never waits for the deadline.
landed() { [ -f "$1" ] && [ -z "$(find "$(dirname "$1")" -name '.quota-reading.*' 2>/dev/null)" ]; }
settle() { wait_for 30 landed "$1"; }
in_json() {  # in_json RATE_LIMITS_JSON (or "" for none)
  if [ -n "$1" ]; then
    jq -cn --arg c "$d/repo" --argjson r "$1" '{session_id:"s", cwd:$c, context_window:{used_percentage:30}, rate_limits:$r}'
  else
    jq -cn --arg c "$d/repo" '{session_id:"s", cwd:$c, context_window:{used_percentage:30}}'
  fi
}

# ── 1. A refresh with rate_limits writes the file ───────────────────────────────────────────────
H="$d/home1"; mkdir -p "$H"; F="$H/.stridal/quota-reading.json"
rl=$(jq -cn --argjson f "$future" --argjson l "$later" \
  '{five_hour:{used_percentage:12.5, resets_at:$f}, seven_day:{used_percentage:40, resets_at:$l}, nimbus:{used_percentage:3, resets_at:null},
    broken:"yes", no_pct:{resets_at:1}, "five-hour":{used_percentage:1}, ("k" * 65):{used_percentage:1},
    frac:{used_percentage:1, resets_at:1.5}, str:{used_percentage:1, resets_at:"soon"}, "nl_key\n":{used_percentage:1}}')
res=$(status "$H" "$(in_json "$rl")")
[ "$(echo "$res" | head -1)" = "0 0" ] && ok "the status line exits 0, nothing on stderr" || fail "status line: $(echo "$res" | head -1)"
case "$(echo "$res" | sed -n 2p)" in *"30% ctx"*) ok "...and prints its line" ;; *) fail "no line: $res" ;; esac
if settle "$F"; then ok "a refresh with rate_limits writes ~/.stridal/quota-reading.json"
else fail "no quota reading written at $F"; fi
got=$(jq -c '[.windows[] | [.key, .pct, .resetsAt]]' "$F" 2>/dev/null)
want=$(jq -cn --argjson f "$future" --argjson l "$later" '[["five_hour",12.5,$f],["nimbus",3,null],["seven_day",40,$l]]')
[ "$got" = "$want" ] && ok "...windows[].{key,pct,resetsAt} as the panel keeps them; bad keys, no pct, a non-integer reset dropped" || fail "windows: $got (want $want)"
jq -e '.windows[] | select(.key == "nimbus") | has("resetsAt") | not' "$F" >/dev/null 2>&1 && ok "...a null resets_at is left out, not saved as null" || fail "nimbus: $(jq -c '.windows' "$F" 2>/dev/null)"
at=$(jq -r '.at' "$F" 2>/dev/null)
[ "$at" -ge $((now * 1000)) ] 2>/dev/null && [ "$at" -le $(( ($(date +%s) + 1) * 1000)) ] && ok "...and when it was read (at, unix ms)" || fail "at: $at"  # wall-clock: at lies between two reads of the clock, which load only spreads
extra=$(jq -r '[keys[] | select(. != "windows" and . != "at")] | join(",")' "$F")
[ -z "$extra" ] && ok "...and nothing else (no session, account or token)" || fail "extra fields: $extra"

# ── 2. The file's mode, and an atomic replace ───────────────────────────────────────────────────
mode=$(ls -l "$F" | cut -c1-10)
[ "$mode" = "-rw-------" ] && ok "the file is 0600" || fail "the file's mode is $mode"
chmod 644 "$F"; ln "$F" "$d/link"; before=$(cat "$F")
rl2=$(jq -cn --argjson f "$future" '{five_hour:{used_percentage:20, resets_at:$f}}')
status "$H" "$(in_json "$rl2")" >/dev/null
pct20() { [ "$(jq -r '.windows[0].pct' "$F" 2>/dev/null)" = 20 ]; }
wait_for 30 pct20; settle "$F"
[ "$(cat "$d/link")" = "$before" ] && [ "$(cat "$F")" != "$before" ] \
  && ok "the file is replaced by a rename (a hard link to the old one still holds the old reading)" \
  || fail "the file was rewritten in place, or not at all"
mode=$(ls -l "$F" | cut -c1-10)
[ "$mode" = "-rw-------" ] && ok "a replaced file is 0600 again" || fail "the replaced file's mode is $mode"
[ -z "$(find "$H/.stridal" -name '.quota-reading.*')" ] && ok "no temp file is left behind" || fail "a temp file was left: $(ls -a "$H/.stridal")"
got=$(jq -c '[.windows[] | [.key, .pct]]' "$F")
[ "$got" = '[["five_hour",20],["nimbus",3],["seven_day",40]]' ] && ok "a window the refresh left out is kept (the panel's per-window merge)" || fail "merge: $got"

# ── 3. No rate_limits: the old file stays ───────────────────────────────────────────────────────
# Its own seeded file, so the case cannot pass on a file that was never there; quota_save itself,
# in the foreground (the status line's only call), so no late write can slip past a fixed wait.
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/quota.sh"
H="$d/home-keep"; mkdir -p "$H/.stridal"; F="$H/.stridal/quota-reading.json"
printf '{"windows":[{"key":"five_hour","pct":7,"resetsAt":%s}],"at":1}\n' "$future" > "$F"
before=$(cat "$F"); ino=$(ls -i "$F" | awk '{print $1}')
for r in "" '{}' '7' '{"five_hour":{"resets_at":1}}'; do
  HOME="$H" quota_save "$(in_json "$r")"
  [ "$(cat "$F")" = "$before" ] && [ "$(ls -i "$F" | awk '{print $1}')" = "$ino" ] \
    && ok "rate_limits ${r:-absent}: the old file is untouched" || fail "rate_limits ${r:-absent} changed the file: $(cat "$F")"
done
# A temp file a killed refresh left is cleared by a later save; a fresh one (a refresh under way) is not.
touch "$H/.stridal/.quota-reading.fresh"
: > "$H/.stridal/.quota-reading.stale"; touch -t 202001010000 "$H/.stridal/.quota-reading.stale"
HOME="$H" quota_save "$(in_json "$rl2")"
[ ! -e "$H/.stridal/.quota-reading.stale" ] && [ -e "$H/.stridal/.quota-reading.fresh" ] \
  && ok "a save clears a temp file older than 5 minutes, and leaves a fresh one" || fail "temp files: $(ls -a "$H/.stridal")"
rm -f "$H/.stridal/.quota-reading.fresh"
# A reset time written as a whole-number decimal (1700000000.0) fails the panel's *int64 decode:
# never saved as a decimal a shell `-gt` would choke on. Raw text, so no jq in between rewrites it.
HOME="$H" quota_save '{"rate_limits":{"dec":{"used_percentage":1,"resets_at":1700000000.0}}}'
grep -q '1700000000\.0' "$F" && fail "a decimal reset time was saved: $(cat "$F")" || ok "a reset time written as 1700000000.0 is never saved as a decimal"
# A reset time past int64 fails the panel's decode, and would read as current for ever.
HOME="$H" quota_save '{"rate_limits":{"huge":{"used_percentage":1,"resets_at":99999999999999999999}}}'
jq -e '[.windows[].key] | index("huge") | not' "$F" >/dev/null && ok "a reset time past int64 drops its window" || fail "a reset time past int64 was saved: $(cat "$F")"

# ── 3b. The save never holds the line ───────────────────────────────────────────────────────────
# A save that takes 6s (a mktemp that sleeps) must not delay the printed line: the status line
# backgrounds it AND gives it no copy of the stdout Claude Code reads, or $(…) would wait for it.
# A line that waited takes the whole 6s, so the bound is 6, never a guess at how long a loaded
# machine takes to print a line (2 s was one: OPS-168); then the slow save is waited out, so it
# never outlives the check or races the scratch directory's removal.
H="$d/home-slow"; mkdir -p "$H" "$d/slowbin"; save=6
printf '#!/bin/sh\nsleep %s\nexec %s "$@"\n' "$save" "$(command -v mktemp)" > "$d/slowbin/mktemp"; chmod +x "$d/slowbin/mktemp"  # mktemp-in-tmpdir: a stand-in that execs the real one, not a call
t0=$(date +%s)  # wall-clock: the slow save case starts; its bound is the length of the save
line=$(printf '%s' "$(in_json "$rl")" | HOME="$H" PATH="$d/slowbin:$PATH" sh "$sl" 2>/dev/null)
took=$(( $(date +%s) - t0 ))  # wall-clock: asserted below the length of the save, which a line that waited took whole
case "$line" in *"30% ctx"*) [ "$took" -lt "$save" ] && ok "a ${save}s save does not delay the line (${took}s)" || fail "the line waited ${took}s for the ${save}s save" ;;
  *) fail "slow save: no line: $line" ;; esac
settle "$H/.stridal/quota-reading.json" && ok "...and the slow save still lands" || fail "the slow save never landed"

# ── 4. A write that cannot happen still prints the line ─────────────────────────────────────────
for c in file ro; do
  H2="$d/home-$c"; mkdir -p "$H2"
  case $c in
    file) : > "$H2/.stridal"; why="~/.stridal is a file" ;;   # fails even for root
    ro) mkdir -p "$H2/.stridal"; echo old > "$H2/.stridal/quota-reading.json"; chmod 555 "$H2/.stridal"; why="~/.stridal is read-only" ;;
  esac
  res=$(status "$H2" "$(in_json "$rl")")
  case "$(echo "$res" | head -1) $(echo "$res" | sed -n 2p)" in
    "0 0 "*"30% ctx"*) ok "$why: the line still prints, exit 0, nothing on stderr" ;;
    *) fail "$why: $res" ;;
  esac
  if [ "$c" = ro ]; then
    # The status line's save is backgrounded, so the old file's survival is shown on quota_save
    # itself, in the foreground. Root writes through a read-only mode, so there it is not this
    # case's to show (the rename test in 2 catches an in-place write).
    HOME="$H2" quota_save "$(in_json "$rl")"
    if [ "$(cat "$H2/.stridal/quota-reading.json")" = old ]; then ok "...and the old file is left in place"
    elif [ "$(id -u)" = 0 ]; then ok "...(as root a read-only directory is writable: the file case above holds the line)"
    else fail "$why: the old file changed"; fi
    # The status line's own backgrounded save must be done before the mode is restored, or it
    # could write into scratch after the case is over.
    settle "$H2/.stridal/quota-reading.json"
    chmod 755 "$H2/.stridal"
  fi
done

# ── 5. The reader ───────────────────────────────────────────────────────────────────────────────
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/quota.sh"
H3="$d/home3"; mkdir -p "$H3/.stridal"; R="$H3/.stridal/quota-reading.json"
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

# ── 6. Its readers (OPS-167): the health line and the lane guard ──────────────────────────────────
# Both run from a fixture project whose project.conf sets the owner's clock and the threshold, on a
# pinned clock (QUOTA_NOW): 2026-10-29 19:00 UTC, 15:00 ET (EDT). The seven-day window resets on
# 2026-11-02 17:00 UTC, 12:00 ET after the clocks go back, so the line must follow TIME_ZONE.
# ROOT is passed as well: the plugin's copy of quota-gate.sh takes it from there, the template's
# from its own path; both land on the fixture.
P="$d/qproj"; mkdir -p "$P/.claude/lib" "$P/.claude/health"
cp "$ROOT/.claude/lib/conf.sh" "$ROOT/.claude/lib/quota.sh" "$ROOT/.claude/lib/modules.sh" "$P/.claude/lib/"
cp "$ROOT/.claude/quota-gate.sh" "$ROOT/.claude/extensions.sh" "$P/.claude/"
has_health=""
if [ -f "$ROOT/.claude/health/quota.sh" ]; then cp "$ROOT/.claude/health/quota.sh" "$P/.claude/health/"; has_health=1
else ok "SKIPPED the health line: this project has no .claude/health/quota.sh (deleted; session start shows no quota)"; fi
QN=1793300400; W5=$((QN + 7200)); W7=1793638800; GONE=$((QN - 60)); AT=$(((QN - 300) * 1000))
HQ="$d/qhome"; mkdir -p "$HQ/.stridal"; QF="$HQ/.stridal/quota-reading.json"
conf() { printf 'TIME_ZONE="America/New_York"\nTIME_ZONE_LABEL="ET"\nQUOTA_LANE_STOP_PCT="%s"\n' "$1" > "$P/.claude/project.conf"; }
# reading 5H 7D OPUS [OPUS_RESET]: a reading saved at 14:55 ET; OPUS_RESET defaults to past, so its
# number (any) must never show.
reading() {
  jq -cn --argjson a "$1" --argjson b "$2" --argjson c "$3" --argjson cr "${4:-$GONE}" --argjson w5 "$W5" --argjson w7 "$W7" --argjson at "$AT" \
    '{windows:[{key:"five_hour",pct:$a,resetsAt:$w5},{key:"seven_day",pct:$b,resetsAt:$w7},{key:"seven_day_opus",pct:$c,resetsAt:$cr}],at:$at}' > "$QF"
}
hl() { (cd "$d" && HOME="$HQ" QUOTA_NOW="${QNOW:-$QN}" ROOT="$P" sh "$P/.claude/health/quota.sh" 2>&1); }
gate() { (cd "$d" && HOME="$HQ" QUOTA_NOW="${QNOW:-$QN}" ROOT="$P" sh "$P/.claude/quota-gate.sh" 2>&1; echo "exit $?"); }

if [ -n "$has_health" ]; then
  conf 90; reading 41 63.0 77.5
  got=$(hl)
  want="OK quota: 5h 41% (resets 17:00 ET) · 7d 63% (resets Mon 2026-11-02 12:00 ET) · 7d Opus no reading (it reset at 14:59 ET) — saved 14:55 ET"
  [ "$got" = "$want" ] && ok "health: each window, its reset on the owner's clock (EDT today, EST on Monday), and when the reading was saved" || fail "health line: '$got' (want '$want')"
  case "$got" in *77.5*) fail "health: printed the number of a window past its reset: $got" ;; *) ok "health: ...never the number of a window past its reset" ;; esac
  got=$(QNOW=$((W7 + 1)) hl)
  case "$got" in "CANNOT CHECK — no window has a current reading: "*) ok "health: past every window's reset, CANNOT CHECK, not OK" ;; *) fail "health past every reset: $got" ;; esac
  case "$got" in *41%*|*63%*|*77.5*) fail "health: a past-reset number was printed: $got" ;; *) ok "health: ...with no number in it" ;; esac
  rm -f "$QF"; got=$(hl)
  case "$got" in "CANNOT CHECK — no quota reading at ~/.stridal/quota-reading.json"*) ok "health: no file is CANNOT CHECK, never silence or OK" ;; *) fail "health with no file: $got" ;; esac
  echo '{"windows":' > "$QF"; got=$(hl)
  case "$got" in "CANNOT CHECK — "*"cannot be read"*) ok "health: a corrupt file is CANNOT CHECK" ;; *) fail "health with a corrupt file: $got" ;; esac
  conf 60; reading 41 63 77.5; got=$(hl)
  case "$got" in "HIGH quota: "*"; 7d at or above 60% (QUOTA_LANE_STOP_PCT)"*) ok "health: a seven-day window at the threshold is HIGH, naming it (session start repeats any line not OK)" ;; *) fail "health above the threshold: $got" ;; esac
  conf 90; reading 99 10 95 "$W7"; got=$(hl)
  case "$got" in "HIGH quota: "*"; 7d Opus at or above 90%"*) ok "health: the Opus seven-day window counts too" ;; *) fail "health, Opus over: $got" ;; esac
  case "$got" in *"; 5h"*|*"; 7d at"*) fail "health: a window under the threshold, or the five-hour one, was named over it: $got" ;; *) ok "health: ...and only the windows over it are named (the five-hour one is not judged)" ;; esac
  reading 99 10 5 "$W7"; got=$(hl)
  case "$got" in "OK quota: 5h 99%"*) ok "health: the five-hour window alone at 99% is OK (the panel's quota_guard holds it)" ;; *) fail "health, 5h at 99: $got" ;; esac
  conf 0; reading 41 95 5 "$W7"; got=$(hl)
  case "$got" in "OK quota: "*"the lane guard is off (QUOTA_LANE_STOP_PCT=0)") ok "health: QUOTA_LANE_STOP_PCT=0 says the guard is off" ;; *) fail "health, guard off: $got" ;; esac
  conf abc; got=$(hl)
  case "$got" in "FAILED quota: "*"QUOTA_LANE_STOP_PCT is 'abc'"*) ok "health: a threshold that is no number is FAILED, by name" ;; *) fail "health, bad threshold: $got" ;; esac
  # The gate refuses every build on a bad threshold whatever the reading, so the line must say so
  # with no current reading, and with no file, too: never hidden behind CANNOT CHECK.
  got=$(QNOW=$((W7 + 1)) hl)
  case "$got" in "FAILED quota: QUOTA_LANE_STOP_PCT is 'abc'"*"no window has a current reading"*) ok "health: a bad threshold is FAILED with no current reading too, and still says what the reading is" ;; *) fail "health, bad threshold past every reset: $got" ;; esac
  rm -f "$QF"; got=$(hl)
  case "$got" in "FAILED quota: QUOTA_LANE_STOP_PCT is 'abc'"*"no quota reading"*) ok "health: ...and with no file" ;; *) fail "health, bad threshold and no file: $got" ;; esac
  # Session start runs it: the directory is the list (extensions.sh health).
  conf 90; reading 41 63 77.5
  got=$(cd "$d" && HOME="$HQ" QUOTA_NOW="$QN" sh "$P/.claude/extensions.sh" health 2>&1)
  case "$got" in *"- Health: quota"*"OK quota: 5h 41%"*) ok "health: session start's extensions.sh health runs it under '- Health: quota'" ;; *) fail "extensions.sh health: $got" ;; esac
fi

# The guard: exit 0 below, 1 at or above, 2 on a bad threshold; no reading fails open and says so.
conf 90; reading 41 63 77.5
got=$(gate)
case "$got" in "quota-gate: below — 7d 63% (resets Mon 2026-11-02 12:00 ET) under 90% (QUOTA_LANE_STOP_PCT)
exit 0") ok "gate: below the threshold, exit 0, naming the window it judged" ;; *) fail "gate below: $got" ;; esac
for v in 90 90.0 95; do
  reading 41 "$v" 5 "$W7"; got=$(gate)
  case "$got" in "quota-gate: STOP — 7d $(printf '%s' "$v" | sed 's/\.0$//')% "*"at or above 90%"*"exit 1") ok "gate: 7d at $v stops (exit 1)" ;; *) fail "gate at $v: $got" ;; esac
done
reading 41 89.99 5 "$W7"; got=$(gate)
case "$got" in *"quota-gate: below"*"exit 0") ok "gate: 89.99 is below 90 (compared as a number, not a string)" ;; *) fail "gate at 89.99: $got" ;; esac
reading 41 10 92 "$W7"; got=$(gate)
case "$got" in "quota-gate: STOP — 7d Opus 92% "*"exit 1") ok "gate: the Opus seven-day window stops it too" ;; *) fail "gate, Opus over: $got" ;; esac
reading 99 10 5 "$W7"; got=$(gate)
case "$got" in *"quota-gate: below"*"exit 0") ok "gate: the five-hour window is not judged" ;; *) fail "gate, 5h at 99: $got" ;; esac
# Past the reset: the old 95 says nothing, so there is no reading.
reading 41 95 95; got=$(QNOW=$((W7 + 1)) gate)
case "$got" in "quota-gate: no reading — no seven-day window has a current reading"*"exit 0") ok "gate: a 95 past its reset is no reading: exit 0, and it says the quota is NOT checked" ;; *) fail "gate past the reset: $got" ;; esac
rm -f "$QF"; got=$(gate)
case "$got" in "quota-gate: no reading — "*"NOT checked"*"exit 0") ok "gate: no file fails open, saying so" ;; *) fail "gate with no file: $got" ;; esac
reading 41 95 5 "$W7"
conf 0; got=$(gate)
case "$got" in "quota-gate: off — "*"exit 0") ok "gate: QUOTA_LANE_STOP_PCT=0 is off" ;; *) fail "gate off: $got" ;; esac
conf 095; got=$(gate)
case "$got" in *"quota-gate: STOP"*"at or above 95%"*"exit 1") ok "gate: a leading zero (095) reads as 95, not octal" ;; *) fail "gate at 095: $got" ;; esac
for bad in abc 101 -5 9.5 ""; do
  conf "$bad"; got=$(gate)
  case "$got" in "quota-gate: ERROR — QUOTA_LANE_STOP_PCT is '$bad'"*"exit 2") ok "gate: a threshold of '$bad' is an ERROR (exit 2), never a guess" ;; *) fail "gate with threshold '$bad': $got" ;; esac
done

# Who calls the guard in production (rule 3): build-change's preflight (checks/build-change.sh drives
# it) and the orchestrating session, whose rule in session-start names it. The patterns leave out the
# `sh .claude/` before the name, which the plugin's copies spell `stridal-model `.
grep -q 'quota-gate\.sh 2>&1; echo "exit \$?"' "$ROOT/.claude/workflows/build-change.js" && ok "build-change.js's preflight runs .claude/quota-gate.sh" || fail "build-change.js does not run .claude/quota-gate.sh"
awk '/^- \*\*Run as an orchestrator of lanes\*\*/ { on = 1 } on && /^- \*\*/ && !/orchestrator of lanes/ { on = 0 } on' "$ROOT/.claude/skills/session-start/SKILL.md" | grep -q 'run `[^`]*quota-gate\.sh`' \
  && ok "session-start's orchestrator rule names sh .claude/quota-gate.sh before a lane starts" || fail "session-start's 'Run as an orchestrator of lanes' rule does not name sh .claude/quota-gate.sh"
finish
