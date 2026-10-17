**Refresh the owner's roadmap tracker (local layer, OPS-83)** when the Context block's
*roadmap-tracker* line says STALE or NEVER RAN. The tracker is the owner's private dashboard:
update it in place, never publish a copy, share it, or write its address into the repository.

0. `sh .claude/local/roadmap-tracker.sh --url` prints the tracker's address (`<url>` below) from
   this machine's uncommitted `tracker-url`; when it fails, say *no tracker configured on this
   machine (tracker-url)* and skip the rest. Never guess the address. Then `git fetch -q origin main`
   and `git rev-parse main origin/main`: if they differ, say *roadmap tracker not refreshed: local
   main is not origin/main* and skip the rest.
1. `sh .claude/local/roadmap-tracker.sh "$TMPDIR/roadmap-tracker"`: the roadmap's rows and its
   *Session plan*, as `roadmap.json` and `sessions.json`. Put any warning it prints in the summary.
2. Upload each file: `Artifact`, `action: "publish"`, `url` `<url>`, `asset: true`, `file_path`
   the file (one call each). Each result says where it is stored, `/_blob/<id>`: keep that path.
3. `ArtifactData` (ToolSearch `select:ArtifactData`), `url` `<url>`: `get` `collection: "datasets"`,
   `doc_id` `"roadmap"`, then `"sessions"`; note each `version` and old `source.url`. Then `update`
   each with `if_version` that version and `data: {source: {kind: "file", url: "/_blob/<id>", name:
   "roadmap.json"}, updated: {at: <now, ISO 8601>, by: "session-start"}}` (`sessions.json` alike).
   `source.url` is the `/_blob/<id>` path the upload returned, as the dashboard type requires.
4. Once both updates succeed: `sh .claude/local/roadmap-tracker.sh --mark "$TMPDIR/roadmap-tracker"`.
   Then each old blob may go, unless its id is the new one: `Artifact`, `action: "delete"`,
   `url` `<url>`, `path` the old id.
   A step that fails: say *roadmap tracker not refreshed: <why>* in the summary; never mark.
