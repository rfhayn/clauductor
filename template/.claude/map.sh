#!/bin/sh
# map.sh: the operating model as a graph (OPS-40). AGENTS.md's rules, the controls of its "What
# executes each rule" table, and the links between them, as one JSON object of schema v1
# (specs/map/spec.md). The panel draws it; the gate holds every reference in it.
#
#   sh .claude/map.sh [--json]          print the map on stdout
#   sh .claude/map.sh --check           exit 1 naming each fault, as FILE[:LINE]: TOKEN: why
#   sh .claude/map.sh --validate FILE   exit 1 naming each way FILE breaks schema v1, whoever made
#                                       it (the panel builds one for a repository with no map.sh)
#
# It writes nothing in the repository: its only scratch is one mktemp -d directory, removed on
# exit. POSIX sh, git and jq (ADR-0006). Exit 2: jq is missing, or a usage error.
# Test seams: MAP_NOW (unix seconds, for `generated`) and PATH (git, jq).

command -v jq >/dev/null 2>&1 || { echo "CANNOT MAP: jq is not installed" >&2; exit 2; }

usage() { echo "usage: map.sh [--json | --check | --validate FILE]" >&2; exit 2; }
mode=json
case ${1:-} in
  "") [ $# -eq 0 ] || usage ;;
  --json) [ $# -eq 1 ] || usage ;;
  --check) [ $# -eq 1 ] || usage; mode=check ;;
  --validate) [ $# -eq 2 ] || usage; mode=validate; vfile=$2 ;;
  *) usage ;;
esac

# ── Schema v1: the closed sets, shared by the map it prints and the validator ───────────────────
SCHEMA_JQ='
def layerdefs: [{n: 0, label: "Rules"}, {n: 1, label: "Controls"}, {n: 2, label: "Mechanisms"},
  {n: 3, label: "Skills and agents"}, {n: 4, label: "Records"}, {n: 5, label: "Work"}];
def kinddefs: [
  {id: "rule", label: "Rule", layer: 0},
  {id: "control", label: "Control", layer: 1},
  {id: "hook", label: "Hook", layer: 2}, {id: "check", label: "Check", layer: 2},
  {id: "script", label: "Script", layer: 2}, {id: "health", label: "Health line", layer: 2},
  {id: "workflow", label: "Workflow", layer: 2}, {id: "module", label: "Module", layer: 2},
  {id: "config", label: "Config", layer: 2}, {id: "file", label: "File", layer: 2},
  {id: "skill", label: "Skill", layer: 3}, {id: "agent", label: "Agent", layer: 3},
  {id: "adr", label: "ADR", layer: 4}, {id: "prac", label: "Practice record", layer: 4},
  {id: "spec", label: "Spec", layer: 4},
  {id: "row", label: "Roadmap row", layer: 5}, {id: "change", label: "Change", layer: 5},
  {id: "lane", label: "Lane", layer: 5}, {id: "worktree", label: "Worktree", layer: 5}];
def kindlayer: kinddefs | map({key: .id, value: .layer}) | from_entries;
def rels: ["instance", "executes", "names", "registers", "enforces", "models", "extends", "deps", "builds", "in"];
def flagsets: {control: ["unexecuted", "by-hand"], row: ["queued", "inflight", "open", "merged", "cancelled"],
  change: ["proposed", "approved", "archived"], spec: ["proposed"]};
def areadefs: [{id: "rules", label: "Rules"}, {id: "change", label: "Changes"}, {id: "merge", label: "Merging"},
  {id: "roadmap", label: "Roadmap"}, {id: "session", label: "Sessions"}, {id: "models", label: "Models"},
  {id: "safety", label: "Safety"}, {id: "records", label: "Records"}, {id: "modules", label: "Modules"},
  {id: "project", label: "Project"}];
'

# ── --validate: any map, held to schema v1 ──────────────────────────────────────────────────────
VALIDATE_JQ='
def q: tojson;
def text: type == "string" and length > 0;
def textornull: . == null or type == "string";
def list: if type == "array" then . else [] end;
def exact($p; $want): if type != "object" then "\($p): must be an object"
  else ((keys_unsorted - $want)[] | "\($p): unknown key \(q)"), (($want - keys_unsorted)[] | "\($p): missing key \(q)") end;
def twice($what): map(select(type == "string")) | group_by(.) | map(select(length > 1) | .[0])[] | "\($what): \(q) is used more than once";
def validate:
  if type != "object" then "the map: must be a JSON object" else
  . as $m
  | [$m.nodes | list[] | objects] as $nodes
  | [$m.areas | list[] | objects | .id] as $areaids
  | (reduce ($nodes[] | .id | strings) as $i ({}; .[$i] = true)) as $idset
  | exact("the map"; ["version", "source", "generated", "commit", "notes", "layers", "kinds", "areas", "nodes", "links"]),
    (if $m.version != 1 then "version: \($m.version | q), not 1" else empty end),
    (if ($m.source | text) then empty else "source: must be a non-empty string" end),
    (if ($m.generated | type) == "string" and ($m.generated | test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$")) then empty
     else "generated: \($m.generated | q) is not an ISO 8601 UTC time (YYYY-MM-DDTHH:MM:SSZ)" end),
    (if $m.commit == null or (($m.commit | type) == "string" and ($m.commit | test("^[0-9a-f]{40}([0-9a-f]{24})?$"))) then empty
     else "commit: \($m.commit | q) is neither a full commit sha nor null" end),
    (if ($m.notes | type) == "array" and all($m.notes[]; type == "string") then empty else "notes: must be a list of strings" end),
    (if ($m.layers | type) != "array" then "layers: must be a list"
     elif ($m.layers | length) != 6 then "layers: must be the six layers 0 to 5, not \($m.layers | length)"
     else $m.layers | to_entries[] | .key as $k | .value as $l | "layers[\($k)]" as $p
       | ($l | exact($p; ["n", "label", "note"])),
         (if ($l | type) == "object" then
            (if $l.n != $k then "\($p): n is \($l.n | q), not \($k)" else empty end),
            (if ($l.label | text) then empty else "\($p): label must be a non-empty string" end),
            (if ($l.note | textornull) then empty else "\($p): note must be a string or null" end),
            (if any($nodes[]; .layer == $k) or ($l.note | text) then empty
             else "\($p): layer \($k) has no nodes, so its note must say why" end)
          else empty end)
     end),
    (if ($m.kinds | type) != "array" then "kinds: must be a list" else
       ($m.kinds | to_entries[] | .key as $k | .value as $v | "kinds[\($k)]" as $p
         | ($v | exact($p; ["id", "label", "layer"])),
           (if ($v | type) == "object" then
              (if kindlayer[$v.id | tostring] == null then "\($p): \($v.id | q) is not a schema v1 kind"
               elif kindlayer[$v.id] != $v.layer then "\($p): kind \($v.id | q) is at layer \(kindlayer[$v.id]), not \($v.layer | q)"
               else empty end),
              (if ($v.label | text) then empty else "\($p): label must be a non-empty string" end)
            else empty end)),
       ([$m.kinds[] | objects | .id] as $have | (kinddefs | map(.id)) - $have | .[] | "kinds: schema v1 kind \(q) is missing"),
       ([$m.kinds[] | objects | .id] | twice("kinds"))
     end),
    (if ($m.areas | type) != "array" then "areas: must be a list" else
       ($m.areas | to_entries[] | .key as $k | .value as $v | "areas[\($k)]" as $p
         | ($v | exact($p; ["id", "label"])),
           (if ($v | type) == "object" then
              (if ($v.id | text) then empty else "\($p): id must be a non-empty string" end),
              (if ($v.label | text) then empty else "\($p): label must be a non-empty string" end)
            else empty end)),
       ($areaids | twice("areas"))
     end),
    (if ($m.nodes | type) != "array" then "nodes: must be a list" else
       ($m.nodes | to_entries[] | .key as $k | .value as $v
         | "nodes[\($k)]\(if ($v | type) == "object" and ($v.id | type) == "string" then " (\($v.id))" else "" end)" as $p
         | ($v | exact($p; ["id", "kind", "label", "area", "layer", "path", "note", "changed", "flags"])),
           (if ($v | type) == "object" then
              (if ($v.id | text) then empty else "\($p): id must be a non-empty string" end),
              (if kindlayer[$v.kind | tostring] == null then "\($p): kind \($v.kind | q) is not a schema v1 kind"
               elif kindlayer[$v.kind] != $v.layer then "\($p): kind \($v.kind) is at layer \(kindlayer[$v.kind]), not \($v.layer | q)"
               else empty end),
              (if ($v.label | type) == "string" then empty else "\($p): label must be a string" end),
              (if any($areaids[]; . == $v.area) then empty else "\($p): area \($v.area | q) is not in areas" end),
              (if ($v.path | textornull) then empty else "\($p): path must be a string or null" end),
              (if ($v.note | textornull) then empty else "\($p): note must be a string or null" end),
              (if $v.changed == null or (($v.changed | type) == "string" and ($v.changed | test("^[0-9]{4}-[0-9]{2}-[0-9]{2}$"))) then empty
               else "\($p): changed \($v.changed | q) is neither YYYY-MM-DD nor null" end),
              (if ($v.flags | type) != "array" then "\($p): flags must be a list"
               else (flagsets[$v.kind | tostring] // []) as $ok
                 | $v.flags[] | select(. as $f | any($ok[]; . == $f) | not) | "\($p): flag \(q) is not one a \($v.kind) takes" end)
            else empty end)),
       ([$nodes[] | .id] | twice("nodes: the id"))
     end),
    (if ($m.links | type) != "array" then "links: must be a list" else
       $m.links | to_entries[] | .key as $k | .value as $v | "links[\($k)]" as $p
       | ($v | exact($p; ["source", "target", "rel", "note"])),
         (if ($v | type) == "object" then
            (if ($v.source | type) == "string" and $idset[$v.source] then empty else "\($p): source \($v.source | q) is not a node id" end),
            (if ($v.target | type) == "string" and $idset[$v.target] then empty else "\($p): target \($v.target | q) is not a node id" end),
            (if any(rels[]; . == $v.rel) then empty else "\($p): rel \($v.rel | q) is not a schema v1 rel" end),
            (if ($v.note | textornull) then empty else "\($p): note must be a string or null" end)
          else empty end)
     end)
  end;
if length != 1 then "the file holds \(length) JSON values, not one map" else .[0] | validate end
'

if [ "$mode" = validate ]; then
  [ -f "$vfile" ] && [ -r "$vfile" ] || { echo "map.sh --validate: cannot read $vfile" >&2; exit 2; }
  faults=$(jq -rs "$SCHEMA_JQ$VALIDATE_JQ" "$vfile" 2>/dev/null) || { echo "$vfile: is not JSON"; exit 1; }
  [ -z "$faults" ] && exit 0
  printf '%s\n' "$faults" | while IFS= read -r l; do printf '%s: %s\n' "$vfile" "$l"; done
  exit 1
fi

# ── The map ─────────────────────────────────────────────────────────────────────────────────────
case ${MAP_NOW:-} in *[!0-9]*) echo "map.sh: MAP_NOW must be unix seconds, not '$MAP_NOW'" >&2; exit 2 ;; esac
# ROOT from this file's own location, before conf.sh: conf.sh would otherwise ask git for the
# top level, a git call of its own, and the wrong root for a project in a subdirectory.
ROOT=$(cd "$(dirname "$0")/.." 2>/dev/null && pwd)
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/conf.sh"
# shellcheck disable=SC1091
. "$ROOT/.claude/lib/refs.sh"

tmp=$(mktemp -d "${TMPDIR:-/tmp}/map.XXXXXX") || { echo "map.sh: cannot make a scratch directory" >&2; exit 2; }
trap 'rm -rf "$tmp"' EXIT
trap 'rm -rf "$tmp"; exit 1' INT TERM
: > "$tmp/faults"; : > "$tmp/rules"; : > "$tmp/controls"; : > "$tmp/lnotes"
fault() { printf '%s\n' "$*" >> "$tmp/faults"; }
lnote() { printf '%s\t%s\n' "$1" "$2" >> "$tmp/lnotes"; }

A="$ROOT/AGENTS.md"
if [ ! -f "$A" ]; then
  lnote 0 "no AGENTS.md at the project root, so no rules to read"
  lnote 1 "no AGENTS.md at the project root, so no 'What executes each rule' table to read"
  fault "AGENTS.md: AGENTS.md: no such file at the project root"
else
  # The rules: N TAB LINE TAB the bold sentence, its number dropped.
  awk '/^\*\*[0-9]+\. / { s = substr($0, 3); n = s; sub(/\..*/, "", n); s = substr(s, length(n) + 3)
         e = index(s, "**"); if (e) s = substr(s, 1, e - 1); gsub(/\t/, " ", s); printf "%s\t%d\t%s\n", n, NR, s }' "$A" > "$tmp/rules"
  if [ ! -s "$tmp/rules" ]; then
    lnote 0 "AGENTS.md has no numbered rules (looked for lines that start **N. …**)"
    fault "AGENTS.md: **N. …**: no numbered rules, so the map's layer 0 is empty"
  fi
  awk -F'\t' '{ l[$1] = l[$1] (l[$1] == "" ? "" : " and ") $2; c[$1]++; if (!($1 in f)) { f[$1] = $2; o[++k] = $1 } }
    END { for (i = 1; i <= k; i++) if (c[o[i]] > 1) printf "AGENTS.md:%s: rule:%s: lines %s each number rule %s\n", f[o[i]], o[i], l[o[i]], o[i] }' "$tmp/rules" >> "$tmp/faults"

  # The controls, one per table row. A control's id is control: plus the POSIX cksum of its
  # trimmed first cell (the key `clauductor update` reads a row by), so inserting or reordering
  # rows keeps every id (D2). One file per cell and ONE cksum call, however long the table.
  refs_table_rows "$A" > "$tmp/rows"
  if [ ! -s "$tmp/rows" ]; then
    lnote 1 "AGENTS.md has no 'What executes each rule' table (looked for the rows under '### What executes each rule')"
    fault "AGENTS.md: ### What executes each rule: no table under this heading, so the map's layer 1 is empty"
  else
    mkdir "$tmp/cells"
    MAP_CELLS="$tmp/cells" awk '{ t = index($0, "\t"); line = substr($0, 1, t - 1); r = substr($0, t + 1)
        sub(/^\|+/, "", r); e = index(r, "|"); c = e ? substr(r, 1, e - 1) : r
        sub(/^[ \t]+/, "", c); sub(/[ \t]+$/, "", c)
        if (c == "") next   # no key, so no control: agentsRows (projectfiles.go) skips it too
        f = sprintf("%s/%06d", ENVIRON["MAP_CELLS"], NR); printf "%s", c > f; close(f)
        label = c; gsub(/\*\*/, "", label); gsub(/\t/, " ", label); printf "%s\t%s\n", line, label }' "$tmp/rows" > "$tmp/ctl"
    if [ -s "$tmp/ctl" ]; then
      # shellcheck disable=SC2046
      printf 'control:%08x\n' $(cksum "$tmp"/cells/* | awk '{ print $1 }') > "$tmp/ids"
      paste "$tmp/ids" "$tmp/ctl" > "$tmp/controls"
    fi
    awk -F'\t' '{ l[$1] = l[$1] (l[$1] == "" ? "" : " and ") $2; c[$1]++; if (!($1 in f)) { f[$1] = $2; o[++k] = $1 } }
      END { for (i = 1; i <= k; i++) if (c[o[i]] > 1) printf "AGENTS.md:%s: %s: lines %s give this one id: same first cell, or a checksum collision\n", f[o[i]], o[i], l[o[i]] }' "$tmp/controls" >> "$tmp/faults"
  fi
fi

commit=$(git -C "$ROOT" rev-parse -q --verify HEAD 2>/dev/null) || commit=""

jq -nc --rawfile rules "$tmp/rules" --rawfile controls "$tmp/controls" --rawfile lnotes "$tmp/lnotes" \
  --arg commit "$commit" --arg now "${MAP_NOW:-}" "$SCHEMA_JQ"'
def tsv: split("\n") | map(select(length > 0) | split("\t"));
def node($id; $kind; $label; $area):
  {id: $id, kind: $kind, label: $label, area: $area, layer: kindlayer[$kind], path: null, note: null, changed: null, flags: []};
($rules | tsv | map(node("rule:\(.[0])"; "rule"; .[2] // ""; "rules"))) as $rn
| ($controls | tsv | map(node(.[0]; "control"; .[2] // ""; "rules"))) as $cn
| ($lnotes | tsv | map({key: .[0], value: .[1]}) | from_entries) as $ln
| ($rn + $cn | sort_by(.id)) as $nodes
# Rules 1 to 3 are instances of rule 4, and the table is rule 4 applied (D4).
| (if any($rn[]; .id == "rule:4")
   then [($rn[] | select(.id != "rule:4")), $cn[] | {source: .id, target: "rule:4", rel: "instance", note: null}]
   else [] end | sort_by([.source, .rel, .target])) as $links
| {version: 1, source: "map.sh",
   generated: ((if $now == "" then now else ($now | tonumber) end) | floor | todate),
   commit: (if $commit == "" then null else $commit end),
   notes: [],
   layers: [layerdefs[] | .n as $n | {n, label,
     note: ($ln[$n | tostring] // (if any($nodes[]; .layer == $n) then null else "no nodes: nothing map.sh read reaches this layer" end))}],
   kinds: kinddefs, areas: areadefs, nodes: $nodes, links: $links}' > "$tmp/map.json" \
  || { echo "map.sh: could not assemble the map" >&2; exit 2; }

if [ "$mode" = json ]; then cat "$tmp/map.json"; exit 0; fi
if [ -s "$tmp/faults" ]; then cat "$tmp/faults"; exit 1; fi
jq -r '"map.sh --check: \(.nodes | length) nodes and \(.links | length) links, and no fault in what it read (AGENTS.md)"' "$tmp/map.json"
