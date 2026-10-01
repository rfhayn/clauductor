#!/bin/sh
# .claude/owner-queue.sh prints every open item of the owner queue and nothing else, and says
# CANNOT CHECK (never "nothing queued") when the file is missing. Also: the project's queue file
# exists, since session-start and the panel card both read it.
. "$(dirname "$0")/lib.sh"

q="$ROOT/.claude/owner-queue.sh"
d=$(scratch)
cat > "$d/q.md" <<'EOF'
# Queue
- [ ] first open item
## Later section
  - [ ] indented open item
- [x] a done item
Text mentioning - [ ] mid-line
EOF
out=$(sh "$q" "$d/q.md")
case "$out" in *"• first open item"*) ok "prints an open item" ;; *) fail "missed an open item: $out" ;; esac
case "$out" in *"• indented open item"*) ok "prints an open item in any section, indented" ;; *) fail "missed an indented item" ;; esac
case "$out" in *"done item"*) fail "printed a done item" ;; *) ok "skips done items" ;; esac
case "$out" in *"mid-line"*) fail "printed a line that merely contains '- [ ]'" ;; *) ok "skips a mid-line '- [ ]'" ;; esac
case "$out" in "2 item(s)"*) ok "counts 2 items" ;; *) fail "wrong count: $(printf '%s' "$out" | head -1)" ;; esac

printf '# Queue\n- [x] done\n' > "$d/empty.md"
case "$(sh "$q" "$d/empty.md")" in "nothing queued"*) ok "an all-done queue reads 'nothing queued'" ;; *) fail "all-done queue misreported" ;; esac
case "$(sh "$q" "$d/absent.md")" in "CANNOT CHECK"*) ok "a missing file reads CANNOT CHECK, not empty" ;; *) fail "a missing file did not say CANNOT CHECK" ;; esac

if [ -f "$ROOT/$OWNER_QUEUE" ]; then ok "the queue file exists ($OWNER_QUEUE)"; else fail "OWNER_QUEUE $OWNER_QUEUE does not exist"; fi
finish
