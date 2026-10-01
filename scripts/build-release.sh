#!/bin/sh
# build-release.sh [--out DIR] [TARGET...]: cross-compile clauductor for release, from any machine.
#
#   scripts/build-release.sh                       # the four release targets, into dist/
#   scripts/build-release.sh linux/amd64           # just one
#   scripts/build-release.sh --out /tmp/rel        # elsewhere
#
# Every build is CGO_ENABLED=0. Nothing in the module needs cgo since OPS-13 removed sqlite, and a
# pure-Go binary cross-compiles from one runner to every target with no C toolchain or osxcross.
# This script checks that claim for each binary (`go version -m`) instead of trusting it: a
# dependency that brings cgo back fails the build here, not on a user's machine.
#
# Each target becomes dist/clauductor-<os>-<arch>.tar.gz holding the binary, the template (tracked
# files only) beside it, where `install`, `init` and `update` find it ("shipped next to the
# binary", framework/internal/template/resolve.go), the LICENSE, the README and the CHANGELOG when
# there is one. Then dist/checksums.txt (SHA-256). The names carry no version, so a release's
# `releases/latest/download/clauductor-<os>-<arch>.tar.gz` URL never changes. The version is the
# binary's own (Version in framework/internal/cmd/root.go), which the template's must match.
#
# The panel runs on macOS only (launchctl, osascript), but the CLI (install, update, init, diff,
# plugin, lock-run) must build and run on Linux too, so the Linux targets are release targets.
#
# This builds; it never tags, uploads or publishes. Run by .github/workflows/release-build.yml.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
out="$root/dist"
targets=""
while [ $# -gt 0 ]; do
  case $1 in
    --out) out=$2; shift 2 ;;
    -h | --help) sed -n '2,6p' "$0"; exit 0 ;;
    */*) targets="$targets $1"; shift ;;
    *) echo "usage: build-release.sh [--out DIR] [os/arch ...]" >&2; exit 2 ;;
  esac
done
[ -n "$targets" ] || targets="darwin/arm64 darwin/amd64 linux/amd64 linux/arm64"

version=$(sed -n 's/^var Version = "\(.*\)"$/\1/p' "$root/framework/internal/cmd/root.go")
[ -n "$version" ] || { echo "build-release: no Version in framework/internal/cmd/root.go" >&2; exit 1; }

mkdir -p "$out"
out=$(cd "$out" && pwd)
stage=$(mktemp -d "${TMPDIR:-/tmp}/build-release.XXXXXX")
trap 'rm -rf "$stage"' EXIT INT TERM

# The template as committed: a stray local file (a worktree, an editor backup) never ships.
(cd "$root" && git ls-files template) > "$stage/template.list"
[ -s "$stage/template.list" ] || { echo "build-release: git lists no template files" >&2; exit 1; }

for t in $targets; do
  os=${t%/*} arch=${t#*/}
  name="clauductor-$os-$arch"
  dir="$stage/$name"
  mkdir -p "$dir"
  echo "build-release: $name (CGO_ENABLED=0)"
  (cd "$root/framework" && CGO_ENABLED=0 GOOS=$os GOARCH=$arch \
    go build -trimpath -ldflags "-s -w" -o "$dir/clauductor" ./cmd/clauductor)
  # The binary records its build settings: hold it to the claim above.
  info=$(go version -m "$dir/clauductor")
  for want in "CGO_ENABLED=0" "GOOS=$os" "GOARCH=$arch"; do
    printf '%s\n' "$info" | grep -q "build[[:space:]]*$want\$" || {
      echo "build-release: $name was not built with $want:" >&2
      printf '%s\n' "$info" | grep 'build' >&2
      exit 1
    }
  done
  (cd "$root" && tar -cf - -T "$stage/template.list") | (cd "$dir" && tar -xf -)
  cp "$root/LICENSE" "$root/README.md" "$dir/"
  [ ! -f "$root/CHANGELOG.md" ] || cp "$root/CHANGELOG.md" "$dir/"
  # COPYFILE_DISABLE: macOS tar would otherwise add ._ AppleDouble files to the archive.
  (cd "$stage" && COPYFILE_DISABLE=1 tar -czf "$out/$name.tar.gz" "$name")
done

# Checksums over every archive in the output directory, so a partial run's set stays verifiable.
(
  cd "$out"
  if command -v sha256sum >/dev/null 2>&1; then sha256sum clauductor-*.tar.gz
  else shasum -a 256 clauductor-*.tar.gz; fi
) > "$out/checksums.txt"
echo "build-release: $(wc -l < "$out/checksums.txt" | tr -d ' ') archive(s) in $out"
