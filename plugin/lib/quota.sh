# quota.sh: sourced (never run) by .claude/statusline.sh, which saves the reading, and by its two
# readers (OPS-167): .claude/health/quota.sh, session start's health line (quota_health), and
# .claude/quota-gate.sh, the guard build-change's preflight stops on (quota_stop_pct, quota_over). ONE file
# per machine, written on every status-line refresh that carries `rate_limits`, with or without the
# panel, so nothing needs the clauductor binary or the panel to read it (ADR-0006). Needs jq.
#
# THE FILE: ~/.clauductor/quota-reading.json, mode 0600, beside the panel's directory. Its fields are
# a subset of the panel's (framework/internal/panel/state/quota.go, `state.Quota`), in its units:
#   {"windows":[{"key":"five_hour","pct":12.5,"resetsAt":1790644200}, ...], "at":1790630000000}
# `key` is the status line's own (`five_hour`, `seven_day`, `seven_day_opus`, or any a plan adds),
# `pct` its used_percentage, `resetsAt` its resets_at in unix seconds (left out when the status line
# gave none), `at` when the status line last saved a reading, in unix milliseconds. Nothing else: no
# session, account or token, and no `expired` flag, so a reader applies the reset rule itself (or
# uses quota_read). As the panel's foldQuota does, a refresh that leaves a window out keeps that
# window's last value, so `at` is a lower bound on a window's age. Two refreshes that overlap can
# land in either order, so `at` can also step back by one refresh; either reading is still a floor.
# With no account in the file, a window that only another account reports (after `/login` to a
# different plan) is carried until its own reset: the panel drops its copy on an account change.
#
# THE RULE THE READER KEEPS: a window whose reset time has passed, or that has none, has NO reading.
# A window's use only grows until its reset, so a reading before the reset is a floor;
# after it, the old number says nothing. The panel's quota.json was once read 9 days stale as
# current: quota_read never prints a number it cannot vouch for.
#
# quota_file             the file's path
# quota_save JSON        save the status-line JSON's rate_limits; silent, and leaves the old file in
#                        place, on any failure or when there are none
# quota_read             one line per window, "<key>\t<pct>\t<resetsAt>", "-" for no reading or no
#                        reset time; exit 1 with no output when there is no readable file
# quota_pct KEY          the window's current percentage, or exit 1 when it has no reading; it can
#                        be a decimal (12.5, or 40.0 under jq 1.7), so compare it with awk, not -ge
# quota_at               when the reading was saved (`at`, unix ms), or exit 1
# quota_label KEY        a window's short name: 5h, 7d, 7d Opus, 7d Sonnet (any other key as it is)
# quota_when EPOCH       a time on the owner's clock (TIME_ZONE): "17:00 ET" on the reader's day,
#                        else "Mon 2026-11-02 17:00 ET"
# quota_stop_pct         QUOTA_LANE_STOP_PCT, checked: a whole number from 0 (the guard is off) to
#                        100, or exit 1 printing what is wrong with it
# quota_over PCT         reads quota_read's lines on stdin and prints those of a seven-day window (every
#                        key that starts seven_day, so a plan's per-model windows too) with a current
#                        reading at or above PCT
# quota_health           the health line: each window, its reset and when the reading was saved; HIGH
#                        when quota_over finds one at the threshold, CANNOT CHECK with no reading
# QUOTA_NOW             the clock the reader uses, in unix seconds (default: now), for a caller's test

quota_file() {
  printf '%s' "$HOME/.clauductor/quota-reading.json"
}

# The windows are kept as the panel's signals.RateLimits keeps them: a key of at most 64 word
# characters with a numeric used_percentage, and a resets_at that is absent, null or a whole number
# within int64 (the panel's *int64: any other value fails that window's decode); any other entry is
# skipped, never fatal. One difference is jq's, not ours: jq 1.6 reads 1700000000.0 as the integer
# it equals and keeps it (saved as 1700000000), where jq 1.7 and the panel drop it.
quota_save() (
  f=$(quota_file)
  dir=$(dirname "$f")
  old=$(jq -c . "$f" 2>/dev/null) || old=""
  new=$(printf '%s' "$1" | jq -ec --arg old "$old" --argjson at "$(date +%s)000" '
    (.rate_limits | if type == "object" then . else {} end | to_entries
      | map(select((.key | test("^[A-Za-z0-9_]{1,64}\\z")) and (.value | type) == "object"
                   and (.value.used_percentage | type) == "number"
                   and (.value.resets_at | type == "null" or (type == "number" and . > -9e18 and . < 9e18
                                                               and (tostring | test("^-?[0-9]+\\z")))))
        | {key, pct: .value.used_percentage}
          + (if .value.resets_at == null then {} else {resetsAt: .value.resets_at} end))) as $new
    | if ($new | length) == 0 then empty else
        (($old | fromjson? // {}) | (.windows? // []) | if type == "array" then . else [] end
          | map(select(type == "object" and (.key | type) == "string"))) as $prev
        | {windows: ([$prev[] | select(.key as $k | all($new[]; .key != $k))] + $new | sort_by(.key)), at: $at}
      end' 2>/dev/null) || return 0
  umask 077
  mkdir -p "$dir" 2>/dev/null || return 0
  # A refresh killed between mktemp and mv leaves its temp file: one older than a few minutes is
  # no refresh's any more.
  find "$dir" -maxdepth 1 -name '.quota-reading.*' -type f -mmin +5 -exec rm -f {} + 2>/dev/null
  # Same directory, so the mv is a rename: a reader sees the old file or the new, never half of one.
  tmp=$(mktemp "$dir/.quota-reading.XXXXXX" 2>/dev/null) || return 0  # mktemp-in-tmpdir: beside the reading, so the mv stays on one file system
  if printf '%s\n' "$new" > "$tmp" 2>/dev/null && chmod 600 "$tmp" 2>/dev/null && mv -f "$tmp" "$f" 2>/dev/null; then
    return 0
  fi
  rm -f "$tmp" 2>/dev/null
  return 0
)

_quota_now() {
  case "${QUOTA_NOW:-}" in '' | *[!0-9]*) date +%s ;; *) printf '%s\n' "$QUOTA_NOW" ;; esac
}

quota_read() {
  [ -f "$(quota_file)" ] || return 1
  _now=$(_quota_now)
  jq -er --argjson now "$_now" '
    .windows | if type == "array" then . else error end | .[]
    | select(type == "object" and (.key | type) == "string")
    | [.key,
       (if (.pct | type) == "number" and (.resetsAt | type) == "number" and .resetsAt > $now
        then (.pct | tostring) else "-" end),
       (if (.resetsAt | type) == "number" then (.resetsAt | tostring) else "-" end)]
    | join("\t")' "$(quota_file)" 2>/dev/null || return 1
}

quota_pct() {
  _q=$(quota_read) || return 1
  _q=$(printf '%s\n' "$_q" | awk -F '\t' -v k="$1" '!got && $1 == k && $2 != "-" { print $2; got = 1 }')
  [ -n "$_q" ] || return 1
  printf '%s\n' "$_q"
}

quota_at() {
  jq -er '.at | numbers' "$(quota_file)" 2>/dev/null || return 1
}

quota_label() {
  case $1 in
    five_hour) echo 5h ;;
    seven_day) echo 7d ;;
    seven_day_opus) echo "7d Opus" ;;
    seven_day_sonnet) echo "7d Sonnet" ;;
    *) printf '%s\n' "$1" ;;
  esac
}

# _quota_date EPOCH FORMAT: EPOCH on the owner's clock, by GNU date or BSD date. LC_ALL=C keeps %a
# in English.
_quota_date() {
  LC_ALL=C TZ="${TIME_ZONE:-America/New_York}" date -d "@$1" "$2" 2>/dev/null \
    || LC_ALL=C TZ="${TIME_ZONE:-America/New_York}" date -r "$1" "$2" 2>/dev/null
}

quota_when() {
  _qw_day=$(_quota_date "$1" +%Y-%m-%d) || { printf 'an unreadable time (%s)\n' "$1"; return 1; }
  if [ "$_qw_day" = "$(_quota_date "$(_quota_now)" +%Y-%m-%d)" ]; then
    printf '%s %s\n' "$(_quota_date "$1" +%H:%M)" "${TIME_ZONE_LABEL:-ET}"
  else
    printf '%s %s\n' "$(_quota_date "$1" '+%a %Y-%m-%d %H:%M')" "${TIME_ZONE_LABEL:-ET}"
  fi
}

quota_stop_pct() {
  _qs=${QUOTA_LANE_STOP_PCT-90}
  case $_qs in
    '' | *[!0-9]* | ????*) echo "QUOTA_LANE_STOP_PCT is '$_qs', not a whole number from 0 (off) to 100"; return 1 ;;
  esac
  # awk drops a leading zero, so 095 is said and compared as 95, never printed "095%".
  _qs=$(printf '%s\n' "$_qs" | awk '{ printf "%d", $1 + 0 }')
  [ "$_qs" -le 100 ] || { echo "QUOTA_LANE_STOP_PCT is '$QUOTA_LANE_STOP_PCT', not a whole number from 0 (off) to 100"; return 1; }
  printf '%s\n' "$_qs"
}

quota_over() {
  awk -F '\t' -v p="$1" '$1 ~ /^seven_day/ && $2 != "-" && $2 + 0 >= p + 0'
}

# _quota_pct_text PCT: 40.0 (jq 1.7) as 40%, 12.5 as 12.5%.
_quota_pct_text() {
  awk -v v="$1" 'BEGIN { printf "%.10g%%", v + 0 }'
}

# The health line (.claude/health/README.md's contract): one line, its verdict first, its subject
# named (each window, and when the reading was saved). A window past its reset is named with NO
# number, the rule quota_read keeps; no reading at all is CANNOT CHECK, never OK.
quota_health() {
  # A bad threshold comes first: quota-gate.sh exits 2 on it whatever the reading, so no reading
  # (CANNOT CHECK) may hide it. The reading is still said, from a pass that skips the threshold.
  if [ -z "${_QH_NOGUARD:-}" ] && ! _qh_pct=$(quota_stop_pct); then
    _qh_r=$(_QH_NOGUARD=1; quota_health)
    echo "FAILED quota: $_qh_pct, so .claude/quota-gate.sh exits 2 and build-change stops on every run until it is one; the reading: ${_qh_r#OK quota: }"
    return 0
  fi
  command -v jq >/dev/null 2>&1 || { echo "CANNOT CHECK — jq is not installed, so the quota reading cannot be read; the quota is UNKNOWN"; return 0; }
  [ -f "$(quota_file)" ] || { echo "CANNOT CHECK — no quota reading at ~/.clauductor/quota-reading.json (the status line saves one on each refresh that carries rate_limits); the quota is UNKNOWN, not low"; return 0; }
  _qh=$(quota_read) || { echo "CANNOT CHECK — ~/.clauductor/quota-reading.json cannot be read (not the shape lib/quota.sh writes); the quota is UNKNOWN"; return 0; }
  _qh_tab=$(printf '\t') _qh_parts="" _qh_cur=""
  while IFS="$_qh_tab" read -r _qh_k _qh_p _qh_r; do
    [ -n "$_qh_k" ] || continue
    _qh_l=$(quota_label "$_qh_k")
    if [ "$_qh_p" != "-" ]; then
      _qh_cur=1
      _qh_w="$_qh_l $(_quota_pct_text "$_qh_p") (resets $(quota_when "$_qh_r"))"
    elif [ "$_qh_r" = "-" ]; then
      _qh_w="$_qh_l no reading (no reset time)"
    else
      _qh_w="$_qh_l no reading (it reset at $(quota_when "$_qh_r"))"
    fi
    _qh_parts="${_qh_parts:+$_qh_parts · }$_qh_w"
  done <<EOF
$_qh
EOF
  _qh_at=$(quota_at) && case $_qh_at in '' | *[!0-9]*) _qh_at="" ;; esac
  if [ -n "$_qh_at" ]; then _qh_saved="saved $(quota_when "$((_qh_at / 1000))")"; else _qh_saved="saved at an unknown time"; fi
  if [ -z "$_qh_cur" ]; then
    echo "CANNOT CHECK — no window has a current reading: ${_qh_parts:-none saved} — $_qh_saved; the quota is UNKNOWN until a status line refreshes"
    return 0
  fi
  [ -z "${_QH_NOGUARD:-}" ] || { echo "OK quota: $_qh_parts — $_qh_saved"; return 0; }
  _qh_over=""
  [ "$_qh_pct" -gt 0 ] && _qh_over=$(printf '%s\n' "$_qh" | quota_over "$_qh_pct" | while IFS="$_qh_tab" read -r _qh_k _qh_p _qh_r; do
    printf '%s, ' "$(quota_label "$_qh_k")"; done)
  if [ -n "$_qh_over" ]; then
    echo "HIGH quota: $_qh_parts — $_qh_saved; ${_qh_over%, } at or above ${_qh_pct}% (QUOTA_LANE_STOP_PCT): .claude/quota-gate.sh stops a new build-change run until it resets"
  elif [ "$_qh_pct" -eq 0 ]; then
    echo "OK quota: $_qh_parts — $_qh_saved; the lane guard is off (QUOTA_LANE_STOP_PCT=0)"
  else
    echo "OK quota: $_qh_parts — $_qh_saved"
  fi
}
