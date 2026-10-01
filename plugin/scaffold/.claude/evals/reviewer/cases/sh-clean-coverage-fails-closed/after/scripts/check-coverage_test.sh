#!/bin/sh
# Tests check-coverage.sh against a stub covsum on PATH: sh scripts/check-coverage_test.sh
here=$(cd "$(dirname "$0")" && pwd)
t=$(mktemp -d) || exit 1
trap 'rm -rf "$t"' EXIT
mkdir "$t/bin"
run() {  # run PERCENT|fail [FLOOR]: the script's exit code with covsum reporting PERCENT, or failing
  echo "${2-80}" > "$t/.coverage-floor"
  if [ "$1" = fail ]; then
    printf '#!/bin/sh\nexit 1\n' > "$t/bin/covsum"
  else
    printf '#!/bin/sh\necho "{\\"total\\":{\\"percent\\":%s}}"\n' "$1" > "$t/bin/covsum"
  fi
  chmod +x "$t/bin/covsum"
  (cd "$t" && PATH="$t/bin:$PATH" sh "$here/check-coverage.sh" >/dev/null 2>&1)
  echo $?
}
fails=0
[ "$(run 85.2)" = 0 ] || { echo "FAIL: coverage above the floor should pass"; fails=1; }
[ "$(run 12)" = 1 ] || { echo "FAIL: coverage below the floor should fail"; fails=1; }
[ "$(run fail)" = 1 ] || { echo "FAIL: a failing covsum should fail the gate"; fails=1; }
[ "$(run 85.2 eighty)" = 1 ] || { echo "FAIL: a floor that is not a number should fail the gate"; fails=1; }
[ "$(run 85.2 '')" = 1 ] || { echo "FAIL: an empty floor should fail the gate"; fails=1; }
exit "$fails"
