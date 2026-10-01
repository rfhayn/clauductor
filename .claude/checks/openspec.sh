#!/bin/sh
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

EN="$ROOT/.claude/modules/openspec/enable.sh"
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
  cp -R "$ROOT/.claude/examples/changes/add-greeting-name" "$d/ex/changes/"
  cp -R "$ROOT/.claude/examples/specs" "$d/ex/specs"
  ln -s ../changes "$d/ex/openspec/changes"; ln -s ../specs "$d/ex/openspec/specs"
  if osv "$d/ex"; then ok "openspec $v validates the example change through the module's symlinks: $(grep -E 'Totals|passed' "$d/os.out" | tail -1)"
  else fail "openspec $v rejects the example change through the symlinks:"; sed 's/^/     /' "$d/os.out" | head -20; fi
  # The module's config.yaml must parse: OpenSpec IGNORES one that does not, with a warning, so its
  # rules silently stop reaching the artifacts it scaffolds. Read one rule back, as its README says.
  cp "$ROOT/.claude/modules/openspec/config.yaml" "$d/ex/openspec/config.yaml"
  (cd "$d/ex" && HOME="$d/home" DO_NOT_TRACK=1 OPENSPEC_TELEMETRY=0 NO_COLOR=1 "$bin" instructions specs --change add-greeting-name --json) > "$d/ins.out" 2>&1
  if grep -q 'could not parse' "$d/ins.out" || ! grep -q 'CAP-n-Sn' "$d/ins.out"; then fail "openspec $v does not read the module's config.yaml rules: $(grep -m1 -i 'parse\|warn' "$d/ins.out")"
  else ok "openspec $v reads the module's config.yaml (its scenario-ID rule reaches the specs instructions)"; fi
  rm -f "$d/ex/openspec/config.yaml"
  # Falsify the link: the same validation must fail when a MODIFIED block drops a scenario (1.13+).
  sed '/GREETING-1-S1/,/THEN/d' "$d/ex/changes/add-greeting-name/specs/greeting/spec.md" > "$d/m" && cp "$d/m" "$d/ex/changes/add-greeting-name/specs/greeting/spec.md"
  if osv "$d/ex"; then fail "openspec $v passed a MODIFIED block that drops a scenario: the validation above proves nothing"
  else ok "openspec $v fails the example once a MODIFIED block drops a scenario (it is really validating)"; fi
fi

# The explore skill the module ships (skills/explore/SKILL.md): a whole skill, installed by enable.sh
# into .claude/skills/explore, refreshed by it, and reported by --check once the copy drifts.
SK="$ROOT/.claude/modules/openspec/skills/explore/SKILL.md"
if [ ! -f "$SK" ]; then fail "the module's explore skill is missing ($SK)"
else
  fmv() { awk -v k="$1" 'NR==1 && $0!="---"{exit} NR>1 && $0=="---"{exit} NR>1 { i=index($0, ":"); if (i && substr($0,1,i-1)==k) { v=substr($0,i+1); sub(/^[ \t]+/,"",v); gsub(/^"|"$/,"",v); print v; exit } }' "$SK"; }
  [ "$(fmv name)" = explore ] && ok "the explore skill's frontmatter names it explore" || fail "explore SKILL.md: name is '$(fmv name)'"
  # The shipped frontmatter is the TEMPLATE's thinker role. A project that retunes thinker sets its
  # installed copy to match (checks/model-roles.sh), which enable.sh keeps, so the shipped file is held
  # only to the template's own model-roles.json, here in clauductor's repository.
  roles=""
  if [ -d "$ROOT/../framework/internal/template" ]; then roles="$ROOT/.claude/model-roles.json"
  elif [ -d "$ROOT/framework/internal/template" ]; then roles="$ROOT/template/.claude/model-roles.json"; fi
  if [ -z "$roles" ]; then
    ok "not clauductor's template: the installed explore skill is held to this project's thinker role by checks/model-roles.sh, not here"
  elif command -v jq >/dev/null 2>&1 && [ -f "$roles" ]; then
    wm=$(jq -r '.roles.thinker.model // empty' "$roles"); we=$(jq -r '.roles.thinker.effort // empty' "$roles")
    [ "$(fmv model)/$(fmv effort)" = "$wm/$we" ] && ok "its model and effort are the template's thinker role ($wm/$we), the role enable.sh tells the project to map it to" \
      || fail "explore SKILL.md says $(fmv model)/$(fmv effort); the template's thinker role is $wm/$we"
  else
    fail "cannot compare the explore skill with the template's thinker role: jq or $roles is missing"
  fi
  # The template's rules, not the vendored skill's: scenario IDs, the CLI floor, /propose for a new
  # change, an approved design is the owner's; and no path that assumes the records live in openspec/.
  for want in '[CAP-n-Sn]' '1.13 or later' '/propose' 'voids that approval' 'CHANGES_DIR/<id>/'; do
    grep -qF -- "$want" "$SK" && ok "the explore skill states '$want'" || fail "the explore skill does not state '$want'"
  done
  grep -qE 'openspec/changes/|openspec new change|/opsx:' "$SK" && fail "the explore skill still names OpenSpec's own paths or commands: $(grep -nE 'openspec/changes/|openspec new change|/opsx:' "$SK" | head -2)" \
    || ok "the explore skill names no openspec/changes path and no opsx command"
  X="$d/explore"; mkdir -p "$X/.claude/lib"
  cp "$ROOT/.claude/lib/conf.sh" "$ROOT/.claude/lib/modules.sh" "$X/.claude/lib/"
  cp -R "$ROOT/.claude/modules" "$X/.claude/modules"
  xen() { (cd "$X" && PATH="$d/stub:$PATH" OPENSPEC_BIN="$d/stub/os-1.13.2" sh .claude/modules/openspec/enable.sh "$@") 2>&1; }
  # --check reports every finding: an old CLI, missing links and a missing skill, all in one run.
  out=$(cd "$X" && PATH="$d/stub:$PATH" OPENSPEC_BIN="$d/stub/os-1.2.0" sh .claude/modules/openspec/enable.sh --check 2>&1); rc=$?
  n=$(printf '%s\n' "$out" | grep -c '^FAIL')
  if [ "$rc" -ne 0 ] && [ "$n" -eq 4 ]; then ok "--check reports all four findings (old CLI, two links, the skill), not just the first (exit $rc)"
  else fail "--check with everything wrong: exit $rc, $n FAIL lines (want 4): $out"; fi
  mkdir -p "$X/openspec"; ln -s ../changes "$X/openspec/changes"; ln -s ../specs "$X/openspec/specs"
  out=$(xen --check); case $out in *"FAIL .claude/skills/explore is not installed"*) ok "--check reports the explore skill not installed" ;; *) fail "--check before enable: $out" ;; esac
  out=$(xen); cmp -s "$SK" "$X/.claude/skills/explore/SKILL.md" && ok "enable.sh installs .claude/skills/explore from the module" || fail "enable.sh did not install the skill: $out"
  out=$(xen --check); case $out in *"ok   .claude/skills/explore is the module's"*) ok "--check then reports it current" ;; *) fail "--check after enable: $out" ;; esac
  echo "drift" >> "$X/.claude/skills/explore/SKILL.md"
  out=$(xen --check); rc=$?
  case $out in *"FAIL .claude/skills/explore differs from"*) ok "--check fails a copy that drifted from the module's (exit $rc)" ;; *) fail "--check on a drifted copy: $out" ;; esac
  xen >/dev/null; cmp -s "$SK" "$X/.claude/skills/explore/SKILL.md" && ok "enable.sh refreshes a drifted copy" || fail "enable.sh left the drifted copy"
  # A project that retuned its thinker role sets the copy's model:/effort: to match (model-roles.sh
  # holds it there): --check accepts that, and a refresh keeps it.
  IN="$X/.claude/skills/explore/SKILL.md"
  sed 's/^model: .*/model: sonnet/; s/^effort: .*/effort: medium/' "$SK" > "$IN"
  out=$(xen --check); rc=$?
  case $out in *"ok   .claude/skills/explore is the module's"*) [ "$rc" -eq 0 ] && ok "--check accepts a copy whose only change is the project's model: and effort:" || fail "--check on a retuned copy exited $rc: $out" ;; *) fail "--check on a retuned copy: $out" ;; esac
  echo "drift" >> "$IN"; xen >/dev/null
  if grep -qx 'model: sonnet' "$IN" && grep -qx 'effort: medium' "$IN" && ! grep -qx drift "$IN"; then ok "a refresh replaces the body and keeps the project's model: and effort:"
  else fail "after a refresh of a retuned copy: $(head -5 "$IN" | tr '\n' ' ') / drift $(grep -c '^drift$' "$IN")"; fi
  f=$(cd "$X" && cp "$ROOT/.claude/extensions.sh" .claude/ && printf 'MODULES="openspec"\n' > .claude/project.conf && sh .claude/extensions.sh fragments explore 2>&1)
  case $f in None.*) ok "the explore SKILL.md is not offered as a fragment of itself" ;; *) fail "fragments explore: $f" ;; esac
  # A project that retuned its thinker role: this very check, run there, must not fail on the shipped
  # skill's frontmatter (it is the template's, and the project's copy is model-roles.sh's to hold).
  if [ -z "${OPENSPEC_INNER:-}" ]; then
    mkdir -p "$X/.claude/checks"; cp "$ROOT/.claude/checks/lib.sh" "$ROOT/.claude/checks/openspec.sh" "$X/.claude/checks/"
    printf '{"roles":{"thinker":{"model":"sonnet","effort":"medium"}},"skills":{}}\n' > "$X/.claude/model-roles.json"
    out=$(cd "$X" && OPENSPEC_INNER=1 ROOT="$X" OPENSPEC_BIN="$d/no-such-openspec" OPENSPEC_REQUIRED= sh .claude/checks/openspec.sh 2>&1); rc=$?
    [ "$rc" -eq 0 ] && ok "in a project whose thinker role is sonnet/medium, this check passes (the shipped frontmatter is held to the template's role only)" \
      || fail "this check fails a project that retuned thinker: $(printf '%s\n' "$out" | grep FAIL | head -3)"
  fi
fi

# With the module on, this project's own links and records are the module's check
# (modules/openspec/checks/project.sh, run by checks/run.sh as openspec:project), not this one's.
finish
