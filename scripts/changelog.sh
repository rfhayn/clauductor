#!/bin/sh
# Regenerate the [Unreleased] section of CHANGELOG.md from merged pull requests (docs/release.md).
#
#   scripts/changelog.sh            rewrite [Unreleased] in place
#   scripts/changelog.sh --dry-run  print the section instead
#
# Lists every pull request merged into main that no released section of CHANGELOG.md already
# cites as (#N), oldest first, as "- <title> (#N)". A PR body line starting "Slice:" (optionally
# as a list item or bold) becomes a nested bullet under its PR, so a stacked PR's slices show.
# The section is regenerated whole: sort it into Added / Changed / Fixed when you cut a release,
# which moves it under a version header, and after that this script leaves those PRs alone.
# Needs gh (signed in) and awk. GH_REPO=owner/name picks another repository.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
changelog="${CHANGELOG:-$root/CHANGELOG.md}" # CHANGELOG=<file>: another file, for trying it out
dry_run=0
case "${1:-}" in
--dry-run) dry_run=1 ;;
"") ;;
*)
	echo "usage: scripts/changelog.sh [--dry-run]" >&2
	exit 2
	;;
esac

command -v gh >/dev/null 2>&1 || {
	echo "changelog: gh is required (https://cli.github.com)" >&2
	exit 1
}
grep -q '^## \[Unreleased\]' "$changelog" || {
	echo "changelog: $changelog has no '## [Unreleased]' header" >&2
	exit 1
}

tmp=$(mktemp -d "${TMPDIR:-/tmp}/changelog.XXXXXX")
trap 'rm -rf "$tmp"' EXIT INT TERM

# PR numbers cited outside [Unreleased]: those are released already.
awk '
	/^## \[Unreleased\]/ { skip = 1; next }
	/^## \[/ { skip = 0 }
	!skip
' "$changelog" | grep -o '(#[0-9][0-9]*)' | tr -d '(#)' | sort -u >"$tmp/released" || true

# One JSON line per PR, oldest merge first; the numbers are filtered in awk, which needs no jq.
gh pr list --state merged --base main --limit 1000 \
	--json number,title,body,mergedAt \
	--jq 'sort_by(.mergedAt)[] |
		"\(.number)\t- \(.title) (#\(.number))" ,
		( (.body // "") | split("\n")[]
		  | select(test("^\\s*([-*]\\s+)?(\\*\\*)?Slice\\b"; "i"))
		  | sub("^\\s*([-*]\\s+)?"; "") | sub("\\s+$"; "")
		  | "SLICE\t  - \(.)" )' >"$tmp/prs"

awk -F '\t' -v released="$tmp/released" '
	BEGIN { while ((getline n < released) > 0) done[n] = 1 }
	$1 == "SLICE" { if (keep) print $2; next }
	{ keep = !($1 in done); if (keep) print $2 }
' "$tmp/prs" >"$tmp/section"

if [ ! -s "$tmp/section" ]; then
	echo "Nothing merged since the last release." >"$tmp/section"
fi

if [ "$dry_run" = 1 ]; then
	echo "## [Unreleased]"
	echo
	cat "$tmp/section"
	exit 0
fi

awk -v section="$tmp/section" '
	/^## \[Unreleased\]/ {
		print; print ""
		while ((getline line < section) > 0) print line
		print ""
		skip = 1; next
	}
	/^## \[/ { skip = 0 }
	!skip
' "$changelog" >"$tmp/out"
mv "$tmp/out" "$changelog"
echo "changelog: [Unreleased] lists $(grep -c '^- ' "$tmp/section" || true) pull request(s)"
