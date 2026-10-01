#!/bin/sh
# Fails the build when total coverage drops below the floor in .coverage-floor, and when it
# cannot read the total or the floor: a gate that cannot measure fails.
floor=$(cat .coverage-floor) || exit 1
case "$floor" in
  '' | *[!0-9]*) echo "coverage: the floor in .coverage-floor is not a whole number ('$floor')" >&2; exit 1 ;;
esac
pct=$(covsum --json coverage.out | jq -r '.total.percent | floor')
case "$pct" in
  '' | *[!0-9]*) echo "coverage: no total in covsum's output ('$pct')" >&2; exit 1 ;;
esac
if [ "$pct" -lt "$floor" ]; then
  echo "coverage $pct% is below the floor $floor%"
  exit 1
fi
echo "coverage $pct% (floor $floor%)"
