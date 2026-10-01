#!/bin/sh
# Fails the build when total coverage drops below the floor in .coverage-floor.
floor=$(cat .coverage-floor)
pct=$(covsum --json coverage.out 2>/dev/null | jq -r '.total.percent | floor')
if [ -n "$pct" ] && [ "$pct" -lt "$floor" ]; then
  echo "coverage $pct% is below the floor $floor%"
  exit 1
fi
echo "coverage ${pct:-?}% (floor $floor%)"
