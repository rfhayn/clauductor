#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# The OpenSpec module (D2 of the change process), where the CLI can run:
#   - the module's version gate refuses a CLI older than 1.13 and accepts 1.13 and later;
#   - the module's config.yaml parses, and its rules reach `openspec instructions`;
#   - the example change (.claude/examples) passes `openspec validate --all --strict` through the
#     module's symlinks (openspec/changes -> ../changes, openspec/specs -> ../specs), so what the
#     skills write by hand is what OpenSpec reads;
#   - with the module on, this project's own links and records are checked by the module itself
#     (modules/openspec/checks/project.sh, which checks/run.sh runs as openspec:project).
#
# The CLI is optional, so without it (or with one older than 1.13) the two validations say SKIPPED
# and why, on an `ok` line that names the reason. OPENSPEC_REQUIRED=1 turns that into a failure: the
# clauductor repository's CI sets it after installing the CLI, so the example is validated there on
# every push. OPENSPEC_BIN names the binary (default: openspec on PATH). Telemetry is off
# (DO_NOT_TRACK=1, OPENSPEC_TELEMETRY=0) and HOME is a scratch directory while it runs.
. "$(dirname "$0")/lib.sh"

EN="$CLAUDUCTOR_FW/modules/openspec/enable.sh"
d=$(scratch)
bin=${OPENSPEC_BIN:-openspec}

# The version gate, against stub CLIs.
mkdir -p "$d/stub"
for v in 1.2.0 1.13.2 2.0.1; do
  printf '#!/bin/sh\necho %s\n' "$v" > "$d/stub/os-$v"; chmod +x "$d/stub/os-$v"
  out=$(ROOT="$d" OPENSPEC_BIN="$d/stub/os-$v" sh "$EN" --check 2>&1 | head -1)
  case "$v:$out" in
    1.2.0:FAIL*"needs 1.13 or later"*) ok "the module refuses openspec 1.2.0, saying it needs 1.13" ;;
    1.13.2:ok*|2.0.1:ok*) ok "the module accepts openspec $v" ;;
    *) fail "openspec $v: $out" ;;
  esac
done

osv() { (cd "$1" && HOME="$d/home" XDG_CONFIG_HOME="$d/home/.config" DO_NOT_TRACK=1 OPENSPEC_TELEMETRY=0 NO_COLOR=1 CI=1 "$bin" validate --all --strict) > "$d/os.out" 2>&1; }
skip() {
  if [ "${OPENSPEC_REQUIRED:-}" = 1 ]; then fail "$1 (OPENSPEC_REQUIRED=1)"; else ok "SKIPPED: $1"; fi
}
usable=""
if ! command -v "$bin" >/dev/null 2>&1; then
  skip "openspec validate on the example: the openspec CLI is not installed"
else
  v=$(DO_NOT_TRACK=1 OPENSPEC_TELEMETRY=0 HOME="$d/home" "$bin" --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+(\.[0-9]+)?' | head -1)
  if [ -n "$v" ] && (ROOT="$d" OPENSPEC_BIN="$bin" sh "$EN" --check 2>/dev/null | grep -q '^ok   openspec'); then usable=1
  else skip "openspec validate on the example: openspec ${v:-of unknown version} is older than 1.13"; fi
fi

if [ -n "$usable" ]; then
  mkdir -p "$d/home" "$d/ex/openspec" "$d/ex/changes"
  cp -R "$CLAUDUCTOR_FW/examples/changes/add-greeting-name" "$d/ex/changes/"
  cp -R "$CLAUDUCTOR_FW/examples/specs" "$d/ex/specs"
  ln -s ../changes "$d/ex/openspec/changes"; ln -s ../specs "$d/ex/openspec/specs"
  if osv "$d/ex"; then ok "openspec $v validates the example change through the module's symlinks: $(grep -E 'Totals|passed' "$d/os.out" | tail -1)"
  else fail "openspec $v rejects the example change through the symlinks:"; sed 's/^/     /' "$d/os.out" | head -20; fi
  # The module's config.yaml must parse: OpenSpec IGNORES one that does not, with a warning, so its
  # rules silently stop reaching the artifacts it scaffolds. Read one rule back, as its README says.
  cp "$CLAUDUCTOR_FW/modules/openspec/config.yaml" "$d/ex/openspec/config.yaml"
  (cd "$d/ex" && HOME="$d/home" DO_NOT_TRACK=1 OPENSPEC_TELEMETRY=0 NO_COLOR=1 "$bin" instructions specs --change add-greeting-name --json) > "$d/ins.out" 2>&1
  if grep -q 'could not parse' "$d/ins.out" || ! grep -q 'CAP-n-Sn' "$d/ins.out"; then fail "openspec $v does not read the module's config.yaml rules: $(grep -m1 -i 'parse\|warn' "$d/ins.out")"
  else ok "openspec $v reads the module's config.yaml (its scenario-ID rule reaches the specs instructions)"; fi
  rm -f "$d/ex/openspec/config.yaml"
  # Falsify the link: the same validation must fail when a MODIFIED block drops a scenario (1.13+).
  sed '/GREETING-1-S1/,/THEN/d' "$d/ex/changes/add-greeting-name/specs/greeting/spec.md" > "$d/m" && cp "$d/m" "$d/ex/changes/add-greeting-name/specs/greeting/spec.md"
  if osv "$d/ex"; then fail "openspec $v passed a MODIFIED block that drops a scenario: the validation above proves nothing"
  else ok "openspec $v fails the example once a MODIFIED block drops a scenario (it is really validating)"; fi
fi

# With the module on, this project's own links and records are the module's check
# (modules/openspec/checks/project.sh, run by checks/run.sh as openspec:project), not this one's.
finish
