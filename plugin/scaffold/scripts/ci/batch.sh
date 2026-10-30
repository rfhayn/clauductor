#!/bin/sh
# batch.sh: a batch of reviewed lanes, run the way PRAC-0013 describes it (OPS-176). Run it on the
# integration branch, in its own worktree, cut from origin/<main>:
#
#   scripts/ci/batch.sh merge <lane branch>…    merge each --no-ff in the order given (a row that
#                                               changes a rule the others are judged by goes first),
#                                               then run BATCH_REGENERATE and commit what it changed
#   scripts/ci/batch.sh sync                    fetch, merge origin/<main>: run it right before EVERY
#                                               gate, so the receipt covers a head GitHub will merge
#   scripts/ci/batch.sh record <PR> <row id>…   once the batch PR is open: write #PR into each row's
#                                               fragment, and name a row with no fragment under the
#                                               first fragment's **Also closes**; commits
#
# WHY. The steps were by hand (S16), and two of them failed silently: a lane PR's receipt for a head
# GitHub then refused for a conflict once main moved (OPS-155), and the fold read each batched row's
# merge from the CLOSED lane PR its fragment named (S16's close fixed twelve Status cells by hand).
# `sync` makes the conflict a stop here; `record` makes the fold read the PR that merged.
#
# A step that cannot finish leaves no merge half done: a conflict is aborted and named, and the
# merges before it stay (fix the lane, or drop it from the batch, and run `merge` with the rest).
# Exit 0 done, 1 stopped, 2 usage.
ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || { echo "batch: not inside a git checkout" >&2; exit 2; }
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/fragments.sh"

usage() { echo "batch: usage: batch.sh merge <branch>… | sync | record <PR number> <row id>…" >&2; exit 2; }
stop() { echo "batch: STOPPED — $*" >&2; exit 1; }
clean() { [ -z "$(git -C "$ROOT" status --porcelain)" ] || stop "the tree is not clean: commit or stash first"; }
# Every arm commits, and catch-up-guard reads git commands in the Bash line, not inside this script:
# so it refuses <main> and a detached HEAD itself (a subagent's Bash starts in the main checkout).
branch=$(git -C "$ROOT" symbolic-ref -q --short HEAD) || branch=""
case $branch in
  ""|"$MAIN_BRANCH") [ $# -eq 0 ] || stop "run it on the batch's integration branch, not ${branch:-a detached HEAD}" ;;
esac
merge_one() {  # merge_one REF: --no-ff; a conflict is aborted and stops the run, naming REF
  git -C "$ROOT" merge --no-ff --no-edit -q "$1" >/dev/null 2>&1 && return 0
  conflicts=$(git -C "$ROOT" diff --name-only --diff-filter=U | paste -sd ' ' -)
  git -C "$ROOT" merge --abort 2>/dev/null
  stop "$1 does not merge cleanly (${conflicts:-no conflict listed}); the merge is aborted, the earlier ones kept"
}

cmd=${1:-}; [ $# -gt 0 ] && shift
case $cmd in
  merge)
    [ $# -gt 0 ] || usage
    clean
    for b in "$@"; do
      git -C "$ROOT" rev-parse -q --verify "$b^{commit}" >/dev/null || stop "$b is not a branch or commit here (fetch it first)"
    done
    for b in "$@"; do merge_one "$b"; echo "batch: merged $b"; done
    # Every lane built its generated files from its own sources; build them once from the merge.
    if [ -n "${BATCH_REGENERATE:-}" ]; then
      (cd "$ROOT" && sh -c "$BATCH_REGENERATE") >/dev/null || stop "BATCH_REGENERATE ($BATCH_REGENERATE) failed"
      if [ -n "$(git -C "$ROOT" status --porcelain)" ]; then
        git -C "$ROOT" add -A && git -C "$ROOT" commit -qm "Regenerate after merging $(printf '%s ' "$@" | sed 's/ $//')" \
          || stop "could not commit what BATCH_REGENERATE changed"
        echo "batch: regenerated ($BATCH_REGENERATE) and committed"
      else
        echo "batch: regenerated ($BATCH_REGENERATE); nothing changed"
      fi
    fi
    ;;
  sync)
    [ $# -eq 0 ] || usage
    clean
    git -C "$ROOT" fetch -q origin "$MAIN_BRANCH" || stop "cannot fetch origin/$MAIN_BRANCH"
    merge_one "origin/$MAIN_BRANCH"
    echo "batch: origin/$MAIN_BRANCH merged; gate this head ($(git -C "$ROOT" rev-parse --short HEAD))"
    ;;
  record)
    [ $# -ge 2 ] || usage
    pr=$1; shift
    printf '%s' "$pr" | grep -qE '^[0-9]+$' || usage
    for id in "$@"; do printf '%s' "$id" | grep -qE '^[A-Za-z0-9][A-Za-z0-9._-]*$' || usage; done
    clean
    d="$ROOT/$JOURNAL_FRAGMENTS"; first="" closed=""
    tmp=$(mktemp "${TMPDIR:-/tmp}/batch.XXXXXX") || stop "no temp file"
    trap 'rm -f "$tmp"' EXIT
    # A stop part way restores the fragments, so it leaves the tree as it found it.
    undo() { git -C "$ROOT" checkout -q -- "$d"; stop "$@"; }
    # The rows an earlier record ADDED (its commits' Batch-added: line) come out of the listed
    # fragments' Also closes first, so a run with the rows that remain is the whole recovery when a row leaves
    # the batch, however many record commits came before. Rows a fragment's author wrote stay.
    prior=$(git -C "$ROOT" log --format=%B "origin/$MAIN_BRANCH..HEAD" 2>/dev/null \
      | sed -n 's/^Batch-added: //p' | tr ',' ' ' | xargs)
    # A fragment outside the batch that closes a listed row would be a second close the fold
    # refuses: stop here, naming it, and never touch that fragment.
    for f in "$d"/*.md; do
      [ -f "$f" ] || continue
      case " $* " in *" $(basename "$f" .md) "*) continue ;; esac
      for id in $(fragment_rows "$f"); do
        case " $* " in *" $id "*) stop "$id is already closed by $(basename "$f") outside this batch: leave it out, or fix that fragment" ;; esac
      done
    done
    if [ -n "$prior" ]; then
      for id in "$@"; do
        f="$d/$id.md"; [ -f "$f" ] || continue
        awk -v drop=" $prior " '
          /^(```|~~~)/ { fence = !fence }
          !fence && !done && /^## / { done = 1 }
          !fence && !done && /^- \*\*Also closes\*\*:/ {
            v = $0; sub(/^- \*\*Also closes\*\*:[ \t]*/, "", v); n = split(v, a, /[ \t]*,[ \t]*/); out = ""
            for (i = 1; i <= n; i++) { gsub(/^[ \t]+|[ \t]+$/, "", a[i]); if (a[i] != "" && index(drop, " " a[i] " ") == 0) out = out (out == "" ? "" : ", ") a[i] }
            if (out == "") next
            $0 = "- **Also closes**: " out
          }
          { print }' "$f" > "$tmp" && cat "$tmp" > "$f" || undo "could not write $f"
      done
    fi
    # A row a listed fragment already closes (its own, or under Also closes) is not added again,
    # so a rerun changes nothing and the fold never sees one row closed twice.
    for id in "$@"; do
      [ -f "$d/$id.md" ] || continue
      [ -n "$first" ] || first=$id
      closed="$closed $(fragment_rows "$d/$id.md" | tr '\n' ' ')"
    done
    [ -n "$first" ] || stop "no fragment for any of $* in $JOURNAL_FRAGMENTS: a batch needs one to record its PR"
    missing=""
    for id in "$@"; do
      case " $closed " in *" $id "*) ;; *) missing="$missing${missing:+, }$id"; closed="$closed $id" ;; esac
    done
    for id in "$@"; do
      f="$d/$id.md"; [ -f "$f" ] || continue
      add=""; [ "$id" = "$first" ] && add=$missing
      # The PR line is set; Also closes gains the rows with no fragment, after any it already names.
      awk -v pr="#$pr" -v add="$add" '
        /^(```|~~~)/ { fence = !fence }
        !fence && !done && /^## / { if (add != "" && !had) print "- **Also closes**: " add; done = 1 }
        !fence && !done && /^- \*\*PR\*\*:/ { print "- **PR**: " pr; next }
        !fence && !done && /^- \*\*Also closes\*\*:/ { had = 1; if (add != "") $0 = $0 ", " add }
        !fence && !done && add != "" && !had && /^$/ && seen { print "- **Also closes**: " add; had = 1 }
        /^- \*\*/ { seen = 1 }
        { print }' "$f" > "$tmp" && cat "$tmp" > "$f" || undo "could not write $f"
      p=$(fragment_problems "$f") || undo "$f lost its shape: $p"
    done
    [ -n "$(git -C "$ROOT" status --porcelain -- "$d")" ] || { echo "batch: #$pr already recorded for $*; nothing changed"; exit 0; }
    { printf '%s: Record batch PR #%s in the rows%s fragments\n' "$first" "$pr" "'"
      [ -z "$missing" ] || printf '\nBatch-added: %s\n' "$missing"; } > "$tmp"
    git -C "$ROOT" add -- "$d" && git -C "$ROOT" commit -q -F "$tmp" \
      || stop "could not commit the fragments"
    note=""; [ -z "$missing" ] || note=" ($missing under the Also closes of $first)"
    echo "batch: #$pr recorded in $JOURNAL_FRAGMENTS for $*$note"
    ;;
  *) usage ;;
esac
