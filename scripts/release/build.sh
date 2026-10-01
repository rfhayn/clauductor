#!/bin/sh
# Build one release archive (docs/release.md). The release workflow runs it once per platform.
#
#   scripts/release/build.sh <goos> <goarch> <version> [outdir]
#
# Writes <outdir>/clauductor-<goos>-<goarch>.tar.gz (outdir defaults to dist/). The asset name
# carries no version, so https://github.com/rfhayn/clauductor/releases/latest/download/<name>
# always names the newest release: install.sh and the Homebrew formula rely on that.
#
# The archive holds the binary and template/: `clauductor init`, `install` and `update` copy
# from template/ (framework/internal/template), so a binary alone cannot adopt a repository.
#
# cgo is on because the state store is mattn/go-sqlite3. That is why each platform builds on
# its own runner rather than cross-compiling from one: only darwin/amd64 is built off-arch, on a
# macOS arm64 runner, where Apple's clang targets x86_64 with -arch.
set -eu

if [ $# -lt 3 ]; then
	echo "usage: scripts/release/build.sh <goos> <goarch> <version> [outdir]" >&2
	exit 2
fi
goos=$1 goarch=$2 version=$3
root=$(cd "$(dirname "$0")/../.." && pwd)
outdir=${4:-$root/dist}
mkdir -p "$outdir"
outdir=$(cd "$outdir" && pwd)

name="clauductor-$goos-$goarch"
stage=$(mktemp -d "${TMPDIR:-/tmp}/clauductor-release.XXXXXX")
trap 'rm -rf "$stage"' EXIT INT TERM
mkdir -p "$stage/$name"

cc=${CC:-}
if [ "$goos" = darwin ]; then
	# The oldest macOS the binary runs on; Go 1.24 itself needs 11.
	export MACOSX_DEPLOYMENT_TARGET="${MACOSX_DEPLOYMENT_TARGET:-12.0}"
	if [ -z "$cc" ]; then
		case $goarch in
		amd64) cc="clang -arch x86_64" ;;
		arm64) cc="clang -arch arm64" ;;
		esac
	fi
fi

(
	cd "$root/framework"
	env CGO_ENABLED=1 GOOS="$goos" GOARCH="$goarch" ${cc:+"CC=$cc"} \
		go build -trimpath \
		-ldflags "-s -w -X github.com/clauductor/clauductor/internal/cmd.Version=$version" \
		-o "$stage/$name/clauductor" ./cmd/clauductor
)

cp -R "$root/template" "$stage/$name/template"
cp "$root/LICENSE" "$root/README.md" "$root/CHANGELOG.md" "$stage/$name/"

# COPYFILE_DISABLE keeps macOS tar from adding ._ AppleDouble files to the archive.
COPYFILE_DISABLE=1 tar -C "$stage" -czf "$outdir/$name.tar.gz" "$name"
echo "$outdir/$name.tar.gz"
