#!/bin/sh
# map.sh (OPS-40): the operating model as a graph, held to schema v1 (specs/map/spec.md) in
# throwaway repositories, never this one, so each result is about map.sh's contract rather than
# what this repository holds today:
#   --validate   a panel-built map, written inline, passes [MAP-6-S1]; each way of breaking the
#                schema is refused and named [MAP-6-S2]
#   no jq        nothing on stdout, CANNOT MAP on stderr, exit 2 [MAP-1-S4]
#   the map      schema v1's keys, kinds and link ends [MAP-1-S1]; it writes nothing [MAP-1-S2];
#                two runs agree, sorted [MAP-1-S3]; the rules and controls, a control's id kept
#                across an inserted row [MAP-2-S1]; every rule and control held to rule 4
#                [MAP-3-S7]; a missing table or missing rules [MAP-5-S9]
. "$(dirname "$0")/lib.sh"
need git jq

d=$(scratch)
MAP_NOW=1791892800; export MAP_NOW   # 2026-10-13T12:00:00Z: the runs below are reproducible

# mkrepo NAME: a throwaway project at $d/NAME holding map.sh and the model's libraries.
mkrepo() {
  R="$d/$1"; new_repo "$R"; mkdir -p "$R/.claude"
  cp -R "$ROOT/.claude/lib" "$R/.claude/"
  cp "$ROOT/.claude/map.sh" "$R/.claude/"
}
# agents_md RULES ROWS: an AGENTS.md with rules 1..RULES and a table of the rows given, one per
# line, as their first cells; each executes nothing ("Nothing. You."), so it names no reference.
agents_md() {
  { printf '# Working conventions\n\n## Four rules\n\n'
    i=1; while [ "$i" -le "$1" ]; do printf '**%s. Rule number %s holds.** It says why.\n\n' "$i" "$i"; i=$((i + 1)); done
    if [ -n "$2" ]; then
      printf '### What executes each rule\n\n| Rule / convention | What executes it |\n|---|---|\n'
      printf '%s\n' "$2" | while IFS= read -r c; do printf '| %s | **Nothing. You.** |\n' "$c"; done
    fi
    printf '\nRun every check.\n'; } > "$R/AGENTS.md"
}
commit_all() { git -C "$R" add -A && git -C "$R" commit -qm "${1:-seed}"; }
# mrun ARGS…: run the throwaway's map.sh; stdout in $d/map.json, stderr in $d/map.err, exit in $rc.
mrun() { rc=0; sh "$R/.claude/map.sh" "$@" > "$d/map.json" 2> "$d/map.err" || rc=$?; }
# q JQ: the jq expression's value over the last map, compact.
q() { jq -c "$1" "$d/map.json"; }

# ── --validate: a map that map.sh did not make ──────────────────────────────────────────────────
# A map the panel builds for an attached repository with no model: layers 0, 1 and 4 are empty,
# each with a note saying why.
cat > "$d/panel.json" <<'JSON'
{"version": 1, "source": "panel", "generated": "2026-10-13T12:00:00Z", "commit": null,
 "notes": ["built by the panel: this repository has no Clauductor model"],
 "layers": [{"n": 0, "label": "Rules", "note": "this repository has no Clauductor model: no AGENTS.md rules to read"},
            {"n": 1, "label": "Controls", "note": "this repository has no Clauductor model: no AGENTS.md table to read"},
            {"n": 2, "label": "Mechanisms", "note": null},
            {"n": 3, "label": "Skills and agents", "note": null},
            {"n": 4, "label": "Records", "note": "this repository has no Clauductor model: no ADRs or specs to read"},
            {"n": 5, "label": "Work", "note": null}],
 "kinds": [{"id": "rule", "label": "Rule", "layer": 0}, {"id": "control", "label": "Control", "layer": 1},
           {"id": "hook", "label": "Hook", "layer": 2}, {"id": "check", "label": "Check", "layer": 2},
           {"id": "script", "label": "Script", "layer": 2}, {"id": "health", "label": "Health line", "layer": 2},
           {"id": "workflow", "label": "Workflow", "layer": 2}, {"id": "module", "label": "Module", "layer": 2},
           {"id": "config", "label": "Config", "layer": 2}, {"id": "file", "label": "File", "layer": 2},
           {"id": "skill", "label": "Skill", "layer": 3}, {"id": "agent", "label": "Agent", "layer": 3},
           {"id": "adr", "label": "ADR", "layer": 4}, {"id": "prac", "label": "Practice record", "layer": 4},
           {"id": "spec", "label": "Spec", "layer": 4}, {"id": "row", "label": "Roadmap row", "layer": 5},
           {"id": "change", "label": "Change", "layer": 5}, {"id": "lane", "label": "Lane", "layer": 5},
           {"id": "worktree", "label": "Worktree", "layer": 5}],
 "areas": [{"id": "project", "label": "Project"}],
 "nodes": [
  {"id": "agent:helper", "kind": "agent", "label": "helper", "area": "project", "layer": 3, "path": "agents/helper.md", "note": null, "changed": "2026-10-01", "flags": []},
  {"id": "config:settings.json", "kind": "config", "label": "settings.json", "area": "project", "layer": 2, "path": "settings.json", "note": null, "changed": null, "flags": []},
  {"id": "hook:hooks/guard.sh", "kind": "hook", "label": "guard.sh", "area": "project", "layer": 2, "path": "hooks/guard.sh", "note": null, "changed": "2026-10-09", "flags": []},
  {"id": "lane:build-1", "kind": "lane", "label": "build-1", "area": "project", "layer": 5, "path": null, "note": "a panel lane", "changed": null, "flags": []},
  {"id": "skill:deploy", "kind": "skill", "label": "deploy", "area": "project", "layer": 3, "path": "skills/deploy/SKILL.md", "note": null, "changed": null, "flags": []},
  {"id": "worktree:worktrees/build-1", "kind": "worktree", "label": "build-1", "area": "project", "layer": 5, "path": "worktrees/build-1", "note": null, "changed": null, "flags": []}],
 "links": [{"source": "config:settings.json", "target": "hook:hooks/guard.sh", "rel": "registers", "note": null},
           {"source": "lane:build-1", "target": "worktree:worktrees/build-1", "rel": "in", "note": null}]}
JSON
mkrepo validate
vrun() { rc=0; sh "$R/.claude/map.sh" --validate "$1" > "$d/v.out" 2>&1 || rc=$?; }
vrun "$d/panel.json"
if [ "$rc" -eq 0 ]; then ok "[MAP-6-S1] a panel-built map (lane, worktree, registers, in; layers 0, 1, 4 empty with notes) validates"
else fail "[MAP-6-S1] a panel-built map is refused (exit $rc):"; sed 's/^/       /' "$d/v.out"; fi
# refused JQ MSG TEXT: the panel map, changed by JQ, is refused, and the fault names TEXT.
refused() {
  jq "$1" "$d/panel.json" > "$d/bad.json"
  vrun "$d/bad.json"
  if [ "$rc" -eq 1 ] && grep -qF -- "$3" "$d/v.out"; then ok "$2"
  else fail "$2 (exit $rc, want 1 naming '$3'):"; sed 's/^/       /' "$d/v.out" | head -5; fi
}
refused '.layers[0].note = null' "[MAP-6-S2] an empty layer with a null note is refused" "layers[0]"
refused '.nodes[0].kind = "gizmo"' "[MAP-6-S2] a node of an unknown kind is refused" '"gizmo"'
refused '.links[0].target = "hook:hooks/gone.sh"' "[MAP-6-S2] a link to a missing node id is refused" "hook:hooks/gone.sh"
refused '. + {colours: {}}' "[MAP-6-S2] an unknown top-level key is refused" '"colours"'
refused '.nodes += [.nodes[0]]' "[MAP-6-S2] two nodes of one id are refused" "agent:helper"
# The rest of schema v1, each refused by name.
refused '.version = 2' "--validate refuses another version" "version"
refused '.source = ""' "--validate refuses an empty source" "source"
refused '.generated = "2026-10-13 12:00"' "--validate refuses a generated time that is not ISO 8601 UTC" "generated"
refused '.links[0].rel = "calls"' "--validate refuses an unknown rel" '"calls"'
refused '.nodes[3].flags = ["merged"]' "--validate refuses a flag the node's kind does not take" '"merged"'
refused '.nodes[0].layer = 2' "--validate refuses a node off its kind's layer" "agent:helper"
refused 'del(.nodes[1].changed)' "--validate refuses a node missing a key" '"changed"'
refused '.nodes[1].changed = "13 Oct"' "--validate refuses a changed that is not YYYY-MM-DD" "config:settings.json"
refused '.nodes[0].area = "elsewhere"' "--validate refuses an area that is not in areas" '"elsewhere"'
refused '.kinds |= map(select(.id != "prac"))' "--validate refuses kinds that are not schema v1's" '"prac"'
refused '.layers |= .[0:5]' "--validate refuses layers that are not 0 to 5" "layers"
refused '.links[1] += {weight: 1}' "--validate refuses a link with an unknown key" '"weight"'
printf 'not json\n' > "$d/bad.json"; vrun "$d/bad.json"
[ "$rc" -eq 1 ] && grep -qi 'not JSON' "$d/v.out" && ok "--validate refuses a file that is not JSON" || fail "--validate on a file that is not JSON: exit $rc: $(head -2 "$d/v.out")"
# A directory is not a map it can read: no answer (exit 2), not an invalid map (exit 1).
mkdir -p "$d/adir"; vrun "$d/adir"
[ "$rc" -eq 2 ] && grep -qF 'cannot read' "$d/v.out" && ok "--validate on a directory exits 2: it cannot read it" || fail "--validate on a directory: exit $rc, want 2: $(head -2 "$d/v.out")"
# MAP_NOW is read only where the map is made: --validate never reads it, and the map refuses one that is not seconds.
rc=0; MAP_NOW=soon sh "$R/.claude/map.sh" --validate "$d/panel.json" > "$d/v.out" 2>&1 || rc=$?
[ "$rc" -eq 0 ] && ok "--validate ignores MAP_NOW, which it never reads" || fail "--validate with MAP_NOW=soon: exit $rc: $(head -2 "$d/v.out")"
rc=0; MAP_NOW=soon sh "$R/.claude/map.sh" > "$d/v.out" 2>&1 || rc=$?
[ "$rc" -eq 2 ] && grep -qF 'MAP_NOW' "$d/v.out" && ok "the map refuses a MAP_NOW that is not unix seconds (exit 2)" || fail "the map with MAP_NOW=soon: exit $rc: $(head -2 "$d/v.out")"

# ── Without jq there is no map ──────────────────────────────────────────────────────────────────
mkdir -p "$d/nojq"
for t in sh cat git awk sed grep tr sort cut mktemp dirname basename wc head tail pwd printf cksum rm mkdir; do
  p=$(command -v "$t" 2>/dev/null) && ln -sf "$p" "$d/nojq/$t"
done
mkrepo nojq; agents_md 4 "Row A"
rc=0; PATH="$d/nojq" "$d/nojq/sh" "$R/.claude/map.sh" > "$d/nojq.out" 2> "$d/nojq.err" || rc=$?
[ "$rc" -eq 2 ] && [ ! -s "$d/nojq.out" ] && grep -qxF 'CANNOT MAP: jq is not installed' "$d/nojq.err" \
  && ok "[MAP-1-S4] without jq: exit 2, nothing on stdout, 'CANNOT MAP: jq is not installed' on stderr" \
  || fail "[MAP-1-S4] without jq: exit $rc, stdout $(wc -c < "$d/nojq.out" | tr -d ' ') bytes, stderr: $(head -2 "$d/nojq.err")"

# ── The map of a repository with the model installed ────────────────────────────────────────────
# install NAME: a throwaway project at $d/NAME with the operating model installed as this project
# has it: its .claude (no worktrees, no per-machine settings), its instructions, its records and
# its gate. In a plugin project the model's own files are the plugin's (CLAUDUCTOR_FW): they are
# laid in first, where an install puts them, and the project's own files over them.
install() {
  R="$d/$1"; new_repo "$R"; mkdir -p "$R/.claude"
  if [ -n "${CLAUDUCTOR_FW:-}" ] && [ "$CLAUDUCTOR_FW" != "$ROOT/.claude" ]; then
    for e in "$CLAUDUCTOR_FW"/*; do
      case $(basename "$e") in scaffold) continue ;; esac
      cp -R "$e" "$R/.claude/"
    done
  fi
  for e in "$ROOT"/.claude/* "$ROOT"/.claude/.[!.]*; do
    [ -e "$e" ] || continue
    case $(basename "$e") in worktrees|settings.local.json) continue ;; esac
    cp -R "$e" "$R/.claude/"
  done
  for e in AGENTS.md CLAUDE.md .gitattributes .gitignore .clauductor .github docs changes specs scripts; do
    [ -e "$ROOT/$e" ] && cp -R "$ROOT/$e" "$R/"
  done
  cp -R "$ROOT/.claude/lib" "$R/.claude/"
  cp "$ROOT/.claude/map.sh" "$R/.claude/"
}
install installed
commit_all
# The scenario's GIVEN, held first: the model's skills, agents, hooks and checks, its config and
# its records are there for the map to read, not only AGENTS.md.
# The paths are spelled through $D, so no line here is itself a path the model's files are read by.
miss=""; D=.claude
for p in AGENTS.md CLAUDE.md $D/settings.json $D/project.conf $D/model-roles.json $D/checks/run.sh $D/hooks $D/skills $D/agents docs/roadmap.md; do
  [ -e "$R/$p" ] || miss="$miss $p"
done
[ -z "$miss" ] && ok "[MAP-1-S1] the throwaway has the model installed: $(ls "$R/.claude/skills" | wc -l | tr -d ' ') skills, $(ls "$R/.claude/checks"/*.sh | wc -l | tr -d ' ') checks, the config and the roadmap" \
  || fail "[MAP-1-S1] the throwaway lacks what an install lays down:$miss"
# s1_holds WHAT: MAP-1-S1's THEN on the map of the throwaway at $R, whose AGENTS.md is WHAT.
s1_holds() {
mrun --json
if [ "$rc" -ne 0 ] || ! jq -e . "$d/map.json" >/dev/null 2>&1; then
  fail "[MAP-1-S1] map.sh --json on $1: exit $rc, or not JSON: $(head -3 "$d/map.err")"
else
  [ "$(q 'keys_unsorted')" = '["version","source","generated","commit","notes","layers","kinds","areas","nodes","links"]' ] \
    && [ "$(q '[.version, .source]')" = '[1,"map.sh"]' ] \
    && ok "[MAP-1-S1] the map has exactly schema v1's top-level keys, version 1 and source map.sh" \
    || fail "[MAP-1-S1] top level: $(q 'keys_unsorted') $(q '[.version, .source]')"
  [ "$(q '[.nodes[] | keys_unsorted == ["id","kind","label","area","layer","path","note","changed","flags"]] | all')" = true ] \
    && [ "$(q '[.nodes[].id] | length == (unique | length)')" = true ] \
    && [ "$(q '(.kinds | map({key: .id, value: .layer}) | from_entries) as $k | [.nodes[] | $k[.kind] != null and $k[.kind] == .layer] | all')" = true ] \
    && ok "[MAP-1-S1] every node has exactly the node keys, a unique id, a kind in kinds and that kind's layer" \
    || fail "[MAP-1-S1] a node breaks the node shape: $(q '[.nodes[] | select(keys_unsorted != ["id","kind","label","area","layer","path","note","changed","flags"])][0]')"
  [ "$(q '[.nodes[].id] as $ids | [.links[] | (.source as $s | $ids | index([$s])) != null and (.target as $t | $ids | index([$t])) != null and (.rel | IN("instance","executes","names","registers","enforces","models","extends","deps","builds","in"))] | all')" = true ] \
    && ok "[MAP-1-S1] every link's source and target is a node, and its rel is schema v1's ($(q '.links | length') links)" \
    || fail "[MAP-1-S1] a link has a dangling end or an unknown rel: $(q '.links[0:3]')"
  # What the AGENTS.md holds, counted here without map.sh's readers: its rules and its rows, and
  # so its instance links (each other rule and each control to rule 4, when there is a rule 4).
  rules=$(grep -c '^\*\*[0-9][0-9]*\. ' "$R/AGENTS.md")
  rows=$(awk '/^### What executes each rule/{p=1; next} p && /^#/{p=0} p && /^\|/' "$R/AGENTS.md" | sed '1,2d' | grep -c .)
  inst=0; grep -q '^\*\*4\. ' "$R/AGENTS.md" && inst=$((rules - 1 + rows))
  [ "$(q '[.nodes[] | select(.kind == "rule")] | length')" = "$rules" ] && [ "$(q '[.nodes[] | select(.kind == "control")] | length')" = "$rows" ] \
    && [ "$(q '[.links[] | select(.rel == "instance")] | length')" = "$inst" ] \
    && ok "[MAP-1-S1] $1 gives $rules rules, $rows controls and $inst instance links" \
    || fail "[MAP-1-S1] $1 gives $(q '[.nodes[] | select(.kind == "rule")] | length') rules, $(q '[.nodes[] | select(.kind == "control")] | length') controls and $(q '[.links[] | select(.rel == "instance")] | length') instance links, want $rules, $rows and $inst"
  [ "$(q '.commit')" = "\"$(git -C "$R" rev-parse HEAD)\"" ] && ok "[MAP-1-S1] commit is HEAD's full sha ($1)" || fail "[MAP-1-S1] commit is $(q '.commit'), not HEAD's $(git -C "$R" rev-parse HEAD) ($1)"
  cp "$d/map.json" "$d/installed.json"; vrun "$d/installed.json"
  [ "$rc" -eq 0 ] && ok "[MAP-1-S1] map.sh's own map passes --validate ($1)" || { fail "[MAP-1-S1] map.sh's own map fails --validate ($1):"; sed 's/^/       /' "$d/v.out" | head -5; }
fi
}
s1_holds "the installed AGENTS.md"

# ── The map writes nothing ──────────────────────────────────────────────────────────────────────
snap() { (cd "$R" && find . -type f | LC_ALL=C sort | while IFS= read -r f; do cksum "$f"; done); }
snap > "$d/before"
mkdir -p "$d/tmp-s2"
rc=0; TMPDIR="$d/tmp-s2" sh "$R/.claude/map.sh" > "$d/map.json" 2>&1 || rc=$?
snap > "$d/after"
[ "$rc" -eq 0 ] && jq -e '.version == 1' "$d/map.json" >/dev/null 2>&1 && cmp -s "$d/before" "$d/after" && [ -z "$(git -C "$R" status --porcelain)" ] \
  && [ -z "$(ls -A "$d/tmp-s2")" ] \
  && ok "[MAP-1-S2] printing the map changes no file, leaves git status empty, and removes its scratch" \
  || fail "[MAP-1-S2] the map wrote something: exit $rc; changed: $(diff "$d/before" "$d/after" | head -2 | tr '\n' ' '); status: $(git -C "$R" status --porcelain | head -2 | tr '\n' ' '); scratch left: $(ls -A "$d/tmp-s2" | head -2 | tr '\n' ' ')"

# ── Two runs at one commit agree ────────────────────────────────────────────────────────────────
mrun; cp "$d/map.json" "$d/run1.json"
mrun; cp "$d/map.json" "$d/run2.json"
cmp -s "$d/run1.json" "$d/run2.json" && [ -s "$d/run1.json" ] \
  && ok "[MAP-1-S3] two runs at one commit with MAP_NOW set are byte for byte the same" \
  || fail "[MAP-1-S3] two runs differ: $(diff "$d/run1.json" "$d/run2.json" | head -3 | tr '\n' ' ')"
[ "$(q '.generated')" = '"2026-10-13T12:00:00Z"' ] && ok "[MAP-1-S3] generated is MAP_NOW in ISO 8601 UTC" || fail "[MAP-1-S3] generated is $(q '.generated') for MAP_NOW=$MAP_NOW"
[ "$(q '[.nodes[].id] == ([.nodes[].id] | sort)')" = true ] && [ "$(q '[.links[] | [.source, .rel, .target]] == ([.links[] | [.source, .rel, .target]] | sort)')" = true ] \
  && ok "[MAP-1-S3] nodes are sorted by id, and links by source, rel, then target" \
  || fail "[MAP-1-S3] the map is not sorted: $(q '[.nodes[].id][0:4]') $(q '[.links[] | .source][0:4]')"

# A project's AGENTS.md is its own: one that adds a fifth rule is still the model installed.
install fiverules
printf '\n**5. A rule of the project'"'"'s own.** It says why.\n' >> "$R/AGENTS.md"
commit_all
s1_holds "an AGENTS.md with a fifth rule"

# ── Rules and controls come from AGENTS.md ──────────────────────────────────────────────────────
mkrepo rules; agents_md 4 "$(printf 'Row A\nRow B\nRow C')"
mrun
[ "$(q '[.nodes[] | select(.kind == "rule") | [.id, .layer]]')" = '[["rule:1",0],["rule:2",0],["rule:3",0],["rule:4",0]]' ] \
  && [ "$(q '[.nodes[] | select(.kind == "control") | .layer] | [length, unique]')" = '[3,[1]]' ] \
  && ok "[MAP-2-S1] four rules make rule:1 to rule:4 at layer 0, and three rows three controls at layer 1" \
  || fail "[MAP-2-S1] rules $(q '[.nodes[] | select(.kind == "rule") | [.id, .layer]]'), controls $(q '[.nodes[] | select(.kind == "control") | .layer]')"
# The id is control: plus the POSIX cksum of the trimmed first cell, as 8 hex digits (D2).
want=$(printf 'control:%08x' "$(printf '%s' 'Row B' | cksum | cut -d' ' -f1)")
idb=$(q '[.nodes[] | select(.kind == "control" and .label == "Row B") | .id] | .[0]' | tr -d '"')
[ "$idb" = "$want" ] && ok "[MAP-2-S1] a control's id is the cksum of its first cell ($idb)" || fail "[MAP-2-S1] Row B's id is '$idb', want $want"
agents_md 4 "$(printf 'Row A\nRow Z, inserted\nRow B\nRow C')"
mrun
[ "$(q '[.nodes[] | select(.kind == "control" and .label == "Row B") | .id] | .[0]' | tr -d '"')" = "$idb" ] && [ "$(q '[.nodes[] | select(.kind == "control")] | length')" = 4 ] \
  && ok "[MAP-2-S1] after a row is inserted above it, Row B's control keeps its id" \
  || fail "[MAP-2-S1] after an insert, Row B is $(q '[.nodes[] | select(.label == "Row B") | .id]') (was $idb)"

# ── Everything holds to rule 4 ──────────────────────────────────────────────────────────────────
held=$(q '[.links[] | select(.rel == "instance")] as $i | [.nodes[] | select(.kind == "control" or (.kind == "rule" and .id != "rule:4")) | .id as $n | [$i[] | select(.source == $n)] | length == 1 and .[0].target == "rule:4"] | [length, all]')
[ "$held" = '[7,true]' ] && [ "$(q '[.links[] | select(.rel == "instance" and .source == "rule:4")] | length')" = 0 ] \
  && ok "[MAP-3-S7] rules 1 to 3 and every control have one instance link each, to rule:4, and rule:4 has none" \
  || fail "[MAP-3-S7] instance links: $held; $(q '[.links[] | select(.rel == "instance")]')"

# ── A missing table or missing rules fail ───────────────────────────────────────────────────────
mkrepo notable; agents_md 4 ""
mrun
[ "$rc" -eq 0 ] && [ "$(q '[.nodes[] | select(.kind == "control")] | length')" = 0 ] && q '.layers[1].note' | grep -q 'What executes each rule' \
  && ok "[MAP-5-S9] with no table, layer 1 is empty and its note names the table looked for: $(q '.layers[1].note')" \
  || fail "[MAP-5-S9] with no table: exit $rc, layer 1 note $(q '.layers[1].note')"
cp "$d/map.json" "$d/notable.json"; vrun "$d/notable.json"
[ "$rc" -eq 0 ] && ok "[MAP-5-S9] the map with no table is still valid schema v1" || fail "[MAP-5-S9] the map with no table fails --validate: $(head -3 "$d/v.out")"
mrun --check
[ "$rc" -eq 1 ] && grep -qF 'What executes each rule' "$d/map.json" && ok "[MAP-5-S9] with no table, --check exits 1 naming it: $(head -1 "$d/map.json")" \
  || fail "[MAP-5-S9] with no table, --check exits $rc: $(head -2 "$d/map.json")"
mkrepo norules; agents_md 0 "Row A"
mrun
[ "$rc" -eq 0 ] && [ "$(q '[.nodes[] | select(.kind == "rule")] | length')" = 0 ] && q '.layers[0].note' | grep -qF '**N.' \
  && [ "$(q '[.links[] | select(.rel == "instance")] | length')" = 0 ] \
  && ok "[MAP-5-S9] with no rules, layer 0 is empty, its note names the rules looked for, and nothing links to a missing rule:4" \
  || fail "[MAP-5-S9] with no rules: exit $rc, layer 0 note $(q '.layers[0].note'), links $(q '.links')"
mrun --check
[ "$rc" -eq 1 ] && grep -qF '**N.' "$d/map.json" && ok "[MAP-5-S9] with no rules, --check exits 1 naming them: $(head -1 "$d/map.json")" \
  || fail "[MAP-5-S9] with no rules, --check exits $rc: $(head -2 "$d/map.json")"
# …and the same check passes the whole fixture, so the two exits above are not vacuous.
mkrepo whole; agents_md 4 "$(printf 'Row A\nRow B')"
mrun --check
[ "$rc" -eq 0 ] && ok "--check passes four rules and a table whose cells name nothing" || fail "--check on the whole fixture exits $rc: $(head -3 "$d/map.json")"

# With no AGENTS.md at all, both layers say so, and --check fails naming the file.
mkrepo noagents
mrun
[ "$rc" -eq 0 ] && q '.layers[0].note' | grep -qF 'no AGENTS.md' && q '.layers[1].note' | grep -qF 'no AGENTS.md' \
  && ok "with no AGENTS.md, layers 0 and 1 are empty and their notes say there is no AGENTS.md" \
  || fail "with no AGENTS.md: exit $rc, notes $(q '[.layers[0].note, .layers[1].note]')"
mrun --check
[ "$rc" -eq 1 ] && grep -qE '^AGENTS\.md: AGENTS\.md: no such file' "$d/map.json" && ok "with no AGENTS.md, --check exits 1 naming it: $(head -1 "$d/map.json")" \
  || fail "with no AGENTS.md, --check exits $rc: $(head -2 "$d/map.json")"

# ── Two rules with one number fail ──────────────────────────────────────────────────────────────
mkrepo duprule; agents_md 4 "Row A"
printf '**2. Rule number 2 again.** A second rule 2.\n' >> "$R/AGENTS.md"
l1=$(grep -n '^\*\*2\. ' "$R/AGENTS.md" | head -1 | cut -d: -f1); l2=$(grep -n '^\*\*2\. ' "$R/AGENTS.md" | tail -1 | cut -d: -f1)
mrun --check
[ "$rc" -eq 1 ] && grep -qxF "AGENTS.md:$l1: rule:2: lines $l1 and $l2 each number rule 2" "$d/map.json" \
  && ok "--check names both lines when two rules share a number: $(head -1 "$d/map.json")" \
  || fail "--check with two rule 2s (lines $l1, $l2) exits $rc: $(head -2 "$d/map.json")"

# ── A row with an empty first cell is no control, as `clauductor update` reads the table ────────
mkrepo emptycell; agents_md 4 "$(printf 'Row A\n\n\nRow B')"
mrun
[ "$(q '[.nodes[] | select(.kind == "control") | .label] | sort')" = '["Row A","Row B"]' ] \
  && ok "two rows with an empty first cell make no control: the controls are Row A and Row B" \
  || fail "rows with an empty first cell: the controls are $(q '[.nodes[] | select(.kind == "control") | .label]')"
mrun --check
[ "$rc" -eq 0 ] && ok "...and --check finds no collision between them" || fail "rows with an empty first cell: --check exits $rc: $(head -2 "$d/map.json")"

# ── Two controls with one id fail closed (D2) ───────────────────────────────────────────────────
mkrepo dup; agents_md 4 "$(printf 'Row A\nRow B\nRow A')"
mrun --check
[ "$rc" -eq 1 ] && grep -qE '^AGENTS\.md:[0-9]+: control:[0-9a-f]{8}: .*lines [0-9]+ and [0-9]+.*same first cell, or a checksum collision' "$d/map.json" \
  && ok "--check names both rows' lines when two controls share an id: $(head -1 "$d/map.json")" \
  || fail "--check with two 'Row A' rows exits $rc: $(head -2 "$d/map.json")"
mrun; cp "$d/map.json" "$d/dup.json"; vrun "$d/dup.json"
dupid=$(jq -r '[.nodes[] | select(.label == "Row A") | .id][0]' "$d/dup.json")
[ "$rc" -eq 1 ] && grep -qF "$dupid" "$d/v.out" && ok "[MAP-6-S2] the map two rows with one first cell make is refused by --validate, naming $dupid" \
  || fail "[MAP-6-S2] the duplicate-row map: --validate exit $rc: $(head -2 "$d/v.out")"
finish
