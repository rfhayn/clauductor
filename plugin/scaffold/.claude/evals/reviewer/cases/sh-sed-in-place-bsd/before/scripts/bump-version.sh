#!/bin/sh
# Bumps the patch version in VERSION.
set -eu
cd "$(dirname "$0")/.."
v=$(cat VERSION)
next=$(echo "$v" | awk -F. '{ printf "%d.%d.%d", $1, $2, $3 + 1 }')
echo "$next" > VERSION
echo "bumped $v -> $next"
