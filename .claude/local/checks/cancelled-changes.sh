#!/bin/sh
# A cancelled change leaves no directory, for the rows docs/roadmap-history.md holds (OPS-154).
# checks/changes.sh fails a changes/<id>/ whose roadmap row is cancelled, but it reads the queue
# through roadmap_queue, which never reads the history: the reset of 2026-10-27 (OPS-147) moved
# every cancelled row there, so a directory for one of them would read as proposed and not yet
# built with nothing failing. This reads the history's queue tables (its session plan skipped, as
# roadmap.d/history.sh does) and fails each such directory. Run first on a scratch project.
. "${CHECKS_LIB:?}"

# cancelled_dirs ROOT: "<change id>\t<row id>" for each directory under ROOT's CHANGES_DIR whose
# history row is cancelled. No history file, nothing to find.
cancelled_dirs() {
  h="$1/docs/roadmap-history.md"
  [ -f "$h" ] || return 0
  awk '
    /^## / { plan = ($0 ~ /^## Session plan/) }
    plan || !/^\| / { next }
    { n = split($0, c, "|"); id = c[2]; gsub(/^[ \t]+|[ \t]+$/, "", id)
      if (id == "" || id == "#" || id ~ /^-+$/) next
      st = c[n - 1]; sub(/^[ \t]+/, "", st)
      if (index(st, "❌ cancelled") != 1 || !match(c[3], /`[^`]*`/)) next
      print substr(c[3], RSTART + 1, RLENGTH - 2) "\t" id }
  ' "$h" | while IFS="$(printf '\t')" read -r cid rid; do
    [ -d "$1/$CHANGES_DIR/$cid" ] && printf '%s\t%s\n' "$cid" "$rid"
  done
}

F="$(scratch)/proj"
mkdir -p "$F/docs" "$F/$CHANGES_DIR/panel-9-x" "$F/$CHANGES_DIR/ops-8-y"
cat > "$F/docs/roadmap-history.md" <<'EOF'
# Roadmap history

## Phase 1 — Old

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| PANEL-9 | `panel-9-x` — cancelled | x | — | ❌ cancelled — the owner, 2026-10-27 |
| OPS-8 | `ops-8-y` — merged | x | — | ✅ merged (#27) |

## Session plan

| Session | When | Focus | Rows | The owner decides |
|---|---|---|---|---|
| S4 | (done) | `ops-8-y` ❌ cancelled | OPS-8 | — |
EOF
got=$(cancelled_dirs "$F" | tr '\t\n' ': ')
[ "$got" = "panel-9-x:PANEL-9 " ] && ok "self-test: a cancelled history row's directory is found, a merged one's and the session plan's are not" \
  || fail "self-test: found [$got], want [panel-9-x:PANEL-9 ]"
rm -r "$F/$CHANGES_DIR/panel-9-x"
got=$(cancelled_dirs "$F")
[ -z "$got" ] && ok "self-test: with the directory removed, nothing is found" || fail "self-test: found [$got] with no directory"

[ -f "$ROOT/docs/roadmap-history.md" ] || { fail "docs/roadmap-history.md is missing: nothing to check the changes directory against"; finish; }
found=$(cancelled_dirs "$ROOT")
if [ -z "$found" ]; then ok "no $CHANGES_DIR/ directory belongs to a row docs/roadmap-history.md holds cancelled"
else
  printf '%s\n' "$found" | while IFS="$(printf '\t')" read -r cid rid; do
    echo "FAIL $cid: its row $rid in docs/roadmap-history.md is cancelled. Remove the directory (git rm -r $CHANGES_DIR/$cid): the row and git history are its record ($CHANGES_DIR/README.md)"
  done
  _fails=$((_fails + $(printf '%s\n' "$found" | wc -l)))
fi
finish
