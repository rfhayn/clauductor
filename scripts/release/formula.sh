#!/bin/sh
# Render the Homebrew formula for a release (docs/release.md, Homebrew).
#
#   scripts/release/formula.sh <version> <checksums.txt> > clauductor.rb
#
# <checksums.txt> is the release's own file (sha256sum format). Every platform must be in it:
# a formula with an empty sha256 would install nothing on that platform, so a missing one fails.
set -eu

if [ $# -ne 2 ]; then
	echo "usage: scripts/release/formula.sh <version> <checksums.txt>" >&2
	exit 2
fi
version=${1#v}
sums=$2
root=$(cd "$(dirname "$0")/../.." && pwd)

sha() {
	s=$(awk -v f="clauductor-$1.tar.gz" '$2 == f || $2 == "*" f { print $1 }' "$sums")
	if ! printf '%s' "$s" | grep -Eq '^[0-9a-f]{64}$'; then
		echo "formula: no sha256 for clauductor-$1.tar.gz in $sums" >&2
		exit 1
	fi
	printf '%s' "$s"
}

da=$(sha darwin-arm64)
di=$(sha darwin-amd64)
la=$(sha linux-arm64)
li=$(sha linux-amd64)

sed -e "s/@VERSION@/$version/g" \
	-e "s/@SHA_DARWIN_ARM64@/$da/" -e "s/@SHA_DARWIN_AMD64@/$di/" \
	-e "s/@SHA_LINUX_ARM64@/$la/" -e "s/@SHA_LINUX_AMD64@/$li/" \
	"$root/packaging/homebrew/clauductor.rb.tmpl"
