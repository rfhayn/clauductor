#!/bin/sh
# Fails the build when total coverage drops below the floor in .coverage-floor.
floor=$(cat .coverage-floor)
pct=$(go tool cover -func=coverage.out | awk '/^total:/ { sub("%", "", $3); print int($3) }')
if [ "$pct" -lt "$floor" ]; then
  echo "coverage $pct% is below the floor $floor%"
  exit 1
fi
echo "coverage $pct% (floor $floor%)"
