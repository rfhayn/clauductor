# quota.sh: sourced (never run) by .claude/statusline.sh, which saves the reading, and by whatever
# needs the account's current quota. Nothing reads it yet but checks/quota-reading.sh: its reader
# went with OPS-42, and OPS-167 wires one or removes it. ONE file
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
# QUOTA_NOW              the clock the reader uses, in unix seconds (default: now), for a caller's test

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

quota_read() {
  [ -f "$(quota_file)" ] || return 1
  case "${QUOTA_NOW:-}" in '' | *[!0-9]*) _now=$(date +%s) ;; *) _now=$QUOTA_NOW ;; esac
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
