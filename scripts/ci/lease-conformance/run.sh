#!/bin/sh
# run.sh: the lease conformance kit's runner. Any implementation of the gate lease protocol (the
# panel's `clauductor lock-run`, the template's lease.sh, a project's own gate-lock script) runs the
# suite through it, from the project, with nothing of clauductor installed.
#
#   sh scripts/ci/lease-conformance/run.sh                    the template's lease.sh (../lease.sh)
#   sh scripts/ci/lease-conformance/run.sh <impl> [args...]   your implementation, called as
#                                       <impl> [args...] <lockdir> <lane> <command> [args...]
#   sh scripts/ci/lease-conformance/run.sh --lock-env VAR <impl> [args...]
#                                       one that takes its lock from $VAR (a gate wrapper):
#                                       VAR=<lockdir> CLAUDUCTOR_LANE=<lane> <impl> [args...] <command>...
#   CASES="dead-pid cancel" sh …/run.sh …       some cases; `bash conformance.sh --list` names them all
#
# It first checks the suite is the one VERSION names (sha256 over conformance.sh and cases/): the
# suite IS the protocol, so a local edit to make a case pass would test nothing. Exits 1 on a
# failed case or a changed suite; prints TAP. README.md has the refresh procedure.
here=$(cd "$(dirname "$0")" && pwd)

# suite_sha: sha256 over "<sha256>  <path>" lines of conformance.sh and every file under cases/,
# sorted by path in the C locale. The Go test in clauductor computes the same.
sha() { if command -v sha256sum >/dev/null 2>&1; then sha256sum; else shasum -a 256; fi; }
suite_sha() {
  (cd "$here" && { echo conformance.sh; find cases -type f; } | LC_ALL=C sort | while IFS= read -r f; do
    printf '%s  %s\n' "$(sha < "$f" | cut -d' ' -f1)" "$f"
  done) | sha | cut -d' ' -f1
}
want=$(sed -n 's/^sha256 //p' "$here/VERSION")
got=$(suite_sha)
if [ "$got" != "$want" ]; then
  echo "not ok - the suite differs from the one VERSION names (sha256 $got, VERSION says ${want:-nothing}): do not edit the suite to make a case pass; README.md says how to refresh it"
  exit 1
fi
[ "${1:-}" = --sha ] && { echo "$got"; exit 0; }

if [ $# -eq 0 ]; then
  [ -f "$here/../lease.sh" ] || { echo "not ok - no ../lease.sh to test; name your implementation (see the header)"; exit 1; }
  adapter=$(mktemp "${TMPDIR:-/tmp}/lease-adapter.XXXXXX") || exit 1
  trap 'rm -f "$adapter"' EXIT
  printf '#!/usr/bin/env bash\nset -euo pipefail\n. "%s"\nlease_run "$@"\n' "$here/../lease.sh" > "$adapter"
  chmod +x "$adapter"
  bash "$here/conformance.sh" "$adapter"
  exit $?
fi
exec bash "$here/conformance.sh" "$@"
