#!/bin/sh
# A cancelled change leaves no directory, for the rows docs/roadmap-history.md holds (OPS-154).
# checks/changes.sh fails a changes/<id>/ whose roadmap row is cancelled, but it reads the queue
# through roadmap_queue, which never reads the history: the reset of 2026-10-27 (OPS-147) moved
# every cancelled row there, so a directory for one of them would read as proposed and not yet
# built with nothing failing. This reads the history's queue tables (its session plan skipped, as
# roadmap.d/history.sh does) and fails each such directory. Run first on a scratch project.
. "${CHECKS_LIB:?}"

# cancelled_dirs ROOT LIVE: "<change id>\t<row id>" for each directory under ROOT's CHANGES_DIR
# whose history row is cancelled, unless file LIVE lists its change id (a row of the roadmap names it
# live: revived). No history file, nothing to find; one that cannot be read returns 1.
cancelled_dirs() {
  h="$1/docs/roadmap-history.md"
  [ -f "$h" ] || return 0
  _rows="$(scratch)/rows"
  awk '
    /^## / { plan = ($0 ~ /^## Session plan/) }
    plan || !/^\| / { next }
    { n = split($0, c, "|"); id = c[2]; gsub(/^[ \t]+|[ \t]+$/, "", id)
      if (id == "" || id == "#" || id ~ /^-+$/) next
      st = c[n - 1]; sub(/^[ \t]+/, "", st)
      if (index(st, "❌ cancelled") != 1 || !match(c[3], /`[^`]*`/)) next
      print substr(c[3], RSTART + 1, RLENGTH - 2) "\t" id }
  ' "$h" > "$_rows" || return 1
  while IFS="$(printf '\t')" read -r cid rid; do
    [ -d "$1/$CHANGES_DIR/$cid" ] || continue
    grep -qxF -- "$cid" "$2" && continue
    printf '%s\t%s\n' "$cid" "$rid"
  done < "$_rows"
}
# live_ids [FILE]: the change ids a row of the roadmap (or of FILE, for the self-test) names and has
# not cancelled, read through roadmap_queue (the one parser): a change revived under a new row is
# not the history's. A roadmap that cannot be read returns 1, naming why: nothing then tells revived
# from cancelled.
live_ids() {
  _tsv=$(roadmap_queue --tsv ${1:+"$1"} 2>&1) || { printf '%s\n' "$_tsv" | head -1; return 1; }
  printf '%s\n' "$_tsv" | awk -F'\t' '$7 != "cancelled" { print $5 }'
}

F="$(scratch)/proj"
mkdir -p "$F/docs" "$F/$CHANGES_DIR/panel-9-x" "$F/$CHANGES_DIR/ops-8-y" "$F/$CHANGES_DIR/panel-7-z"
# The session plan's row has the queue's shape, a change id and a cancelled last cell, so only the
# plan's skip keeps ops-8-y out. panel-7-z is cancelled here but live on the roadmap (revived).
cat > "$F/docs/roadmap-history.md" <<'EOF'
# Roadmap history

## Phase 1 — Old

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| PANEL-9 | `panel-9-x` — cancelled | x | — | ❌ cancelled — the owner, 2026-10-27 |
| OPS-8 | `ops-8-y` — merged | x | — | ✅ merged (#27) |
| PANEL-7 | `panel-7-z` — cancelled | x | — | ❌ cancelled — the owner, 2026-10-27 |

## Session plan

| Session | When | Focus | Rows | The owner decides |
|---|---|---|---|---|
| S4 | `ops-8-y` | x | OPS-8 | ❌ cancelled — moved |
EOF
printf '## Phase 1 — x\n\n| # | Change | Scope | Deps | Status |\n|---|---|---|---|---|\n| X-1 | `panel-7-z` — revived | x | — | ⬜ queued |\n| X-2 | `panel-9-x` — cancelled | x | — | ❌ cancelled — no |\n' > "$F/docs/roadmap.md"
if live_ids "$F/docs/roadmap.md" > "$F/live"; then
  [ "$(cat "$F/live")" = panel-7-z ] && ok "self-test: the live list holds the roadmap's queued change id, not its cancelled one" \
    || fail "self-test: the live list is [$(tr '\n' ' ' < "$F/live")], want [panel-7-z]"
else fail "self-test: the fixture roadmap could not be read: $(cat "$F/live")"; fi
printf '## Phase 1 — x\n\n| # | Change | Scope | Deps | Status |\n|---|---|---|---|---|\n| X-1 | no id here | x | — | ⬜ queued |\n' > "$F/docs/bad.md"
if live_ids "$F/docs/bad.md" >/dev/null; then fail "self-test: an unparsable roadmap gave a live list"
else ok "self-test: an unparsable roadmap fails, never reads as no live change"; fi
got=$(cancelled_dirs "$F" "$F/live" | tr '\t\n' ': ')
[ "$got" = "panel-9-x:PANEL-9 " ] && ok "self-test: a cancelled history row's directory is found; a merged one's, a revived one's and the session plan's are not" \
  || fail "self-test: found [$got], want [panel-9-x:PANEL-9 ]"
rm -r "$F/$CHANGES_DIR/panel-9-x"
got=$(cancelled_dirs "$F" "$F/live")
[ -z "$got" ] && ok "self-test: with the directory removed, nothing is found" || fail "self-test: found [$got] with no directory"
chmod 000 "$F/docs/roadmap-history.md"
if cancelled_dirs "$F" "$F/live" >/dev/null 2>&1; then fail "self-test: an unreadable history passed"
else ok "self-test: an unreadable history fails, never reads as no cancelled row"; fi
chmod 644 "$F/docs/roadmap-history.md"

[ -f "$ROOT/docs/roadmap-history.md" ] || { fail "docs/roadmap-history.md is missing: nothing to check the changes directory against"; finish; }
live="$(scratch)/live"
live_ids > "$live" || { fail "the roadmap could not be read ($(cat "$live")): nothing to tell a revived change from a cancelled one"; finish; }
found=$(cancelled_dirs "$ROOT" "$live") || { fail "docs/roadmap-history.md could not be read"; finish; }
if [ -z "$found" ]; then ok "no $CHANGES_DIR/ directory belongs to a row docs/roadmap-history.md holds cancelled"
else
  printf '%s\n' "$found" | while IFS="$(printf '\t')" read -r cid rid; do
    echo "FAIL $cid: its row $rid in docs/roadmap-history.md is cancelled. Remove the directory (git rm -r $CHANGES_DIR/$cid): the row and git history are its record ($CHANGES_DIR/README.md)"
  done
  _fails=$((_fails + $(printf '%s\n' "$found" | wc -l)))
fi
finish
