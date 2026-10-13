# quota.sh: sourced (never run) by .claude/statusline.sh, which saves the reading, and by whatever
# needs the account's current quota (routines, OPS-42 D4/D17; the orchestrating session). ONE file
# per machine, written on every status-line refresh that carries `rate_limits`, with or without the
# panel, so nothing needs the clauductor binary or the panel to read it (ADR-0006). Needs jq.
#
# THE FILE: ~/.clauductor/quota-reading.json, mode 0600, beside the panel's directory. Its shape is
# the panel's (framework/internal/panel/state/quota.go, `state.Quota`), so one reader serves both:
#   {"windows":[{"key":"five_hour","pct":12.5,"resetsAt":1790644200}, ...], "at":1790630000000}
# `key` is the status line's own (`five_hour`, `seven_day`, `seven_day_opus`, or any a plan adds),
# `pct` its used_percentage, `resetsAt` its resets_at in unix seconds (left out when the status line
# gave none), `at` when the status line last saved a reading, in unix milliseconds. Nothing else: no
# session, account or token. As the panel's foldQuota does, a refresh that leaves a window out keeps
# that window's last value, so `at` is a lower bound on a window's age.
#
# THE RULE THE READER KEEPS: a window whose reset time has passed, or that has none, has NO reading.
# A window's use only grows until its reset, so a reading before the reset is a floor (OPS-42 D4);
# after it, the old number says nothing. The panel's quota.json was once read 9 days stale as
# current: quota_read never prints a number it cannot vouch for.
#
# quota_file             the file's path
# quota_save JSON        save the status-line JSON's rate_limits; silent, and leaves the old file in
#                        place, on any failure or when there are none
# quota_read             one line per window, "<key>\t<pct>\t<resetsAt>", "-" for no reading or no
#                        reset time; exit 1 with no output when there is no readable file
# quota_pct KEY          the window's current percentage, or exit 1 when it has no reading

quota_file() {
  printf '%s' "$HOME/.clauductor/quota-reading.json"
}

# The windows are kept as the panel's signals.RateLimits keeps them: a key of at most 64 word
# characters with a numeric used_percentage; any other entry is skipped, never fatal.
quota_save() (
  f=$(quota_file)
  dir=$(dirname "$f")
  old=$(jq -c . "$f" 2>/dev/null) || old=""
  new=$(printf '%s' "$1" | jq -ec --arg old "$old" --argjson at "$(date +%s)000" '
    (.rate_limits | if type == "object" then . else {} end | to_entries
      | map(select((.key | test("^[A-Za-z0-9_]{1,64}$")) and (.value | type) == "object"
                   and (.value.used_percentage | type) == "number")
        | {key, pct: .value.used_percentage}
          + (if (.value.resets_at | type) == "number" then {resetsAt: (.value.resets_at | floor)} else {} end))) as $new
    | if ($new | length) == 0 then empty else
        (($old | fromjson? // {}) | (.windows? // []) | if type == "array" then . else [] end
          | map(select(type == "object" and (.key | type) == "string"))) as $prev
        | {windows: ([$prev[] | select(.key as $k | all($new[]; .key != $k))] + $new | sort_by(.key)), at: $at}
      end' 2>/dev/null) || return 0
  umask 077
  mkdir -p "$dir" 2>/dev/null || return 0
  # Same directory, so the mv is a rename: a reader sees the old file or the new, never half of one.
  tmp=$(mktemp "$dir/.quota-reading.XXXXXX" 2>/dev/null) || return 0
  if printf '%s\n' "$new" > "$tmp" 2>/dev/null && chmod 600 "$tmp" 2>/dev/null && mv -f "$tmp" "$f" 2>/dev/null; then
    return 0
  fi
  rm -f "$tmp" 2>/dev/null
  return 0
)

quota_read() {
  [ -f "$(quota_file)" ] || return 1
  jq -er --argjson now "$(date +%s)" '
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
  _q=$(printf '%s\n' "$_q" | awk -F '\t' -v k="$1" '$1 == k && $2 != "-" { print $2; exit }')
  [ -n "$_q" ] || return 1
  printf '%s\n' "$_q"
}
