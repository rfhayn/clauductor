#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/../../.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The OpenSpec module's own check, run by checks/run.sh only while the module is on (MODULES, or the
# legacy PROPOSALS="openspec"): this project's links point where CHANGES_DIR and SPECS_DIR say, and
# its changes and specs pass `openspec validate --all --strict`. Without a 1.13+ CLI the validation
# says SKIPPED and why; OPENSPEC_REQUIRED=1 makes that a failure. The module's materials (its
# version gate, config.yaml, the example) are tested whether it is on or not, by checks/openspec.sh.
# shellcheck disable=SC1090
. "${CHECKS_LIB:?run this through checks/run.sh}"

EN="$CLAUDUCTOR_FW/modules/openspec/enable.sh"
d=$(scratch)
bin=${OPENSPEC_BIN:-openspec}
skip() {
  if [ "${OPENSPEC_REQUIRED:-}" = 1 ]; then fail "$1 (OPENSPEC_REQUIRED=1)"; else ok "SKIPPED: $1"; fi
}

out=$(OPENSPEC_BIN="$bin" sh "$EN" --check 2>&1)
printf '%s\n' "$out" | grep -E '^(ok|FAIL)' > "$d/links"
cat "$d/links"; _fails=$((_fails + $(grep -c '^FAIL' "$d/links")))
if printf '%s\n' "$out" | grep -q '^ok   openspec [0-9]'; then
  if (cd "$ROOT" && HOME="$d" XDG_CONFIG_HOME="$d/.config" DO_NOT_TRACK=1 OPENSPEC_TELEMETRY=0 NO_COLOR=1 CI=1 "$bin" validate --all --strict) > "$d/os.out" 2>&1; then
    ok "openspec validate --all --strict passes on this project"
  else
    fail "openspec validate --all --strict fails on this project:"; sed 's/^/     /' "$d/os.out" | head -20
  fi
else
  skip "the OpenSpec module is on, but openspec 1.13 or later is not installed, so this project's changes are not validated by it"
fi
finish
