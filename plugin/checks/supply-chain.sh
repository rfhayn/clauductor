#!/bin/sh
CLAUDUCTOR_FW=$(cd "$(dirname "$0")/.." && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)
# Dependency hygiene (item 11 of the change process):
#   - a dependency-update config holds each update back until the release has aged: every
#     Dependabot entry has `cooldown: default-days: N`, or renovate.json has "minimumReleaseAge";
#     N at least SUPPLY_MIN_AGE_DAYS (default 3);
#   - the scheduled dependency audit exists (.github/workflows/dependency-audit.yml, on a schedule and
#     on demand), and its health line reports it: ADVISORIES on a failed run, CANNOT CHECK offline.
# The secret scan is a gate step (scripts/ci/run-local.sh), falsified by checks/gate.sh.
. "$(dirname "$0")/lib.sh"
need jq

min=${SUPPLY_MIN_AGE_DAYS:-3}
d=$(scratch)

# ages DIR: prints ok/FAIL lines for the update config in DIR.
ages() {
  if [ -f "$1/.github/dependabot.yml" ]; then
    awk -v min="$min" '
      /^[[:space:]]*-[[:space:]]*package-ecosystem:/ { if (eco != "") report(); eco = $0; sub(/.*package-ecosystem:[[:space:]]*/, "", eco); gsub(/"/, "", eco); days = ""; next }
      /^[[:space:]]*default-days:/ { days = $2 }
      function report() { if (days == "") printf "FAIL dependabot %s has no cooldown default-days (a minimum release age)\n", eco
                          else if (days + 0 < min) printf "FAIL dependabot %s cooldown is %s day(s), under %s\n", eco, days, min
                          else printf "ok   dependabot %s waits %s days before proposing a release\n", eco, days }
      END { if (eco != "") report(); else print "FAIL .github/dependabot.yml has no update entry" }' "$1/.github/dependabot.yml"
  elif [ -f "$1/renovate.json" ]; then
    age=$(jq -r '.minimumReleaseAge // empty' "$1/renovate.json" 2>/dev/null)
    n=$(printf '%s' "$age" | sed -n 's/^\([0-9][0-9]*\) days\{0,1\}$/\1/p')
    if [ -z "$age" ]; then echo "FAIL renovate.json has no minimumReleaseAge"
    elif [ -z "$n" ] || [ "$n" -lt "$min" ]; then echo "FAIL renovate.json minimumReleaseAge is '$age', want at least '$min days'"
    else echo "ok   renovate waits $age before proposing a release"; fi
  else
    echo "FAIL no dependency-update config (.github/dependabot.yml or renovate.json)"
  fi
}

# Self-test.
mkdir -p "$d/a/.github" "$d/b/.github" "$d/c" "$d/e"
printf 'version: 2\nupdates:\n  - package-ecosystem: "npm"\n    directory: "/"\n    cooldown:\n      default-days: 7\n  - package-ecosystem: "gomod"\n    directory: "/"\n' > "$d/a/.github/dependabot.yml"
ages "$d/a" | grep -q '^FAIL dependabot gomod has no cooldown' && ok "self-test: a Dependabot entry with no cooldown fails" || fail "self-test: a Dependabot entry with no cooldown passed"
printf 'version: 2\nupdates:\n  - package-ecosystem: "npm"\n    cooldown:\n      default-days: 1\n' > "$d/b/.github/dependabot.yml"
ages "$d/b" | grep -q '^FAIL' && ok "self-test: a one-day cooldown fails" || fail "self-test: a one-day cooldown passed"
printf '{"extends":["config:recommended"]}\n' > "$d/c/renovate.json"
ages "$d/c" | grep -q '^FAIL' && ok "self-test: renovate.json with no minimumReleaseAge fails" || fail "self-test: renovate.json with no minimumReleaseAge passed"
printf '{"minimumReleaseAge":"7 days"}\n' > "$d/c/renovate.json"
ages "$d/c" | grep -q '^FAIL' && fail "self-test: renovate.json with 7 days failed" || ok "self-test: renovate.json with minimumReleaseAge 7 days passes"
ages "$d/e" | grep -q '^FAIL no dependency-update config' && ok "self-test: no update config at all fails" || fail "self-test: no update config passed"

ages "$ROOT" > "$d/real"; cat "$d/real"; _fails=$((_fails + $(grep -c '^FAIL' "$d/real")))

# The scheduled audit and its reader.
wf="$ROOT/.github/workflows/dependency-audit.yml"
if [ -f "$wf" ]; then
  grep -qE '^[[:space:]]*schedule:' "$wf" && grep -qE '^[[:space:]]*workflow_dispatch:' "$wf" \
    && ok "dependency-audit.yml runs on a schedule and on demand" || fail "dependency-audit.yml needs both schedule: and workflow_dispatch: triggers"
  # It audits every tracked manifest at any depth, not only the root's (OPS-8 rehearsal: a Go
  # module in framework/ was never audited). The block between its markers runs, dry, in a repo
  # whose manifests are nested.
  sed -n '/# audit: begin/,/# audit: end/p' "$wf" | sed 's/^          //' > "$d/audit.sh"
  if [ -s "$d/audit.sh" ]; then
    new_repo "$d/m"
    mkdir -p "$d/m/framework" "$d/m/web" "$d/m/svc/api" "$d/m/framework/x/testdata" "$d/m/.claude/evals/r/before"
    : > "$d/m/framework/go.mod"; : > "$d/m/web/package-lock.json"; : > "$d/m/svc/api/requirements.txt"; : > "$d/m/framework/x/testdata/go.mod"; : > "$d/m/.claude/evals/r/before/go.mod"
    git -C "$d/m" add -A && git -C "$d/m" commit -qm m
    out=$(cd "$d/m" && AUDIT_DRY_RUN=1 sh "$d/audit.sh" 2>&1)
    case "$out" in *"govulncheck framework "*"npm web "*|*"npm web "*"govulncheck framework "*) ok "the audit finds manifests below the root (framework/go.mod, web/package-lock.json)" ;; *) fail "the audit missed nested manifests: $out" ;; esac
    case "$out" in *"pip-audit svc/api "*) ok "the audit finds a nested requirements.txt" ;; *) fail "the audit missed svc/api/requirements.txt: $out" ;; esac
    case "$out" in *testdata* | *evals*) fail "the audit ran on a test fixture's manifest: $out" ;; *) ok "the audit skips test fixtures (testdata/, the evals' cases)" ;; esac
    new_repo "$d/empty"; out=$(cd "$d/empty" && AUDIT_DRY_RUN=1 sh "$d/audit.sh" 2>&1)
    case "$out" in "no dependency manifest"*) ok "a repo with no manifest says so" ;; *) fail "a repo with no manifest printed: $out" ;; esac
  else
    fail "dependency-audit.yml has no '# audit: begin' … '# audit: end' block (the audit this check runs)"
  fi
else
  fail "no .github/workflows/dependency-audit.yml (the scheduled dependency audit)"
fi
h="$ROOT/.claude/health/dependency-audit.sh"
if [ -f "$h" ]; then
  out=$(CONTEXT_OFFLINE=1 sh "$h"); case "$out" in "CANNOT CHECK — offline"*|"OK no "*) ok "the advisories health line makes no network call offline" ;; *) fail "health line offline said: $out" ;; esac
  mkdir -p "$d/bin" "$d/h/.claude/health" "$d/h/.github/workflows"; cp "$h" "$d/h/.claude/health/"; : > "$d/h/.github/workflows/dependency-audit.yml"
  printf '#!/bin/sh\necho "$GH_RUNS"\n' > "$d/bin/gh"; chmod +x "$d/bin/gh"
  runs='[{"conclusion":"failure","status":"completed","createdAt":"2099-01-05T06:17:00Z","headSha":"abcdef0123456","url":"https://x/1"}]'
  out=$(GH_RUNS=$runs CONTEXT_OFFLINE= PATH="$d/bin:$PATH" sh "$d/h/.claude/health/dependency-audit.sh")
  case "$out" in ADVISORIES*"high severity"*"abcdef012"*) ok "a failed audit reads ADVISORIES, naming the run's commit" ;; *) fail "failed audit read as: $out" ;; esac
  out=$(GH_RUNS='[{"conclusion":"success","status":"completed","createdAt":"2020-01-05T06:17:00Z","headSha":"abcdef0123456","url":"u"}]' CONTEXT_OFFLINE= PATH="$d/bin:$PATH" sh "$d/h/.claude/health/dependency-audit.sh")
  case "$out" in STALE*) ok "an audit more than two weeks old reads STALE" ;; *) fail "old audit read as: $out" ;; esac
  out=$(GH_RUNS='[]' CONTEXT_OFFLINE= PATH="$d/bin:$PATH" sh "$d/h/.claude/health/dependency-audit.sh")
  case "$out" in "NEVER RAN"*) ok "no audit on record reads NEVER RAN" ;; *) fail "no runs read as: $out" ;; esac
else
  fail "no .claude/health/dependency-audit.sh (the advisories health line)"
fi
finish
