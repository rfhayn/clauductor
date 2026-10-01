#!/bin/sh
# Check that the repository is ready to release (docs/release.md).
#
#   scripts/release-check.sh            the CHANGELOG has a section for the current Version
#   scripts/release-check.sh v0.1.0     also: the tag names that Version, and its section is dated
#
# The release workflow runs the second form on every tag, before it builds anything, so a tag
# that disagrees with framework/internal/cmd/root.go or the CHANGELOG never becomes a release.
# Prints the Version on success.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
rootgo="$root/framework/internal/cmd/root.go"
changelog="$root/CHANGELOG.md"

version=$(sed -n 's/^var Version = "\([^"]*\)"$/\1/p' "$rootgo")
if [ -z "$version" ]; then
	echo "release-check: no 'var Version = \"...\"' line in $rootgo" >&2
	exit 1
fi

if [ ! -f "$changelog" ]; then
	echo "release-check: $changelog is missing" >&2
	exit 1
fi

# grep -F on the literal header prefix: a Version holds dots, which a regex would treat as any byte.
header=$(grep -F -- "## [$version]" "$changelog" | head -n 1 || true)
if [ -z "$header" ]; then
	echo "release-check: CHANGELOG.md has no '## [$version]' section; add one (scripts/changelog.sh fills [Unreleased])" >&2
	exit 1
fi

if [ $# -ge 1 ]; then
	tag=$1
	if [ "$tag" != "v$version" ]; then
		echo "release-check: tag $tag does not match Version $version in framework/internal/cmd/root.go (expected v$version)" >&2
		exit 1
	fi
	# At release time the section carries its date: "## [0.1.0] - 2026-10-01".
	if ! printf '%s\n' "$header" | grep -Eq -- '^## \[[^]]+\] - [0-9]{4}-[0-9]{2}-[0-9]{2}$'; then
		echo "release-check: '$header' has no release date; write it as '## [$version] - YYYY-MM-DD'" >&2
		exit 1
	fi
fi

echo "$version"
