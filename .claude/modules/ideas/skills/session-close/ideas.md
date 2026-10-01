**Render the Ideas queue into the repository (the ideas module), with step 3's records.** The
queue lives in the Ideas page's database; `IDEAS_FILE` (default `docs/ideas.md`) is generated from
it here and never edited (the module's check, `ideas:queue`, fails the gate on a hand edit).
`ArtifactData` is deferred: load it with ToolSearch `select:ArtifactData`. The page's URL, for every
call below: `sh .claude/modules/ideas/bin/render.sh --url`. No `ArtifactData` tool:
`CANNOT CHECK — no Artifact tool`; leave the file as it is and say so in the journal.

1. **Set the status of each idea this session talked through**, and only those: `ArtifactData`
   `update`, `collection: "ideas"`, the idea's `doc_id`, `if_version` from the read, `data`
   `{"status": "discussed" | "parked" | "roadmap", "updatedAt": <now, ISO UTC>}`, plus
   `"roadmapRow": "<row id>"` for `roadmap` (a row that exists in the roadmap: the owner scopes
   promotion, so `roadmap` means they made the row). Never delete an idea.
2. **Dump the queue**: `rm -rf <scratchpad>/ideas-dump`, then `ArtifactData` `list`,
   `collection: "ideas"`, `query: {"limit": 1000}`, `out_dir: "<scratchpad>/ideas-dump"` (repeat
   with `query.cursor` while there is a `next_cursor`). A list that errors: leave the file as it is
   and say so in the journal; never render from a partial dump. After a successful list,
   `mkdir -p <scratchpad>/ideas-dump/ideas`: an empty collection writes nothing, and the renderer
   refuses a missing directory.
3. **Names**: the distinct `authorId`s in the dump, one `ArtifactData` `profiles` call (skip it when
   there are none), then ALWAYS write `<scratchpad>/ideas-names.json`: `{ "<authorId>": "<name>" }`
   for each resolved name, or `{}` when there are none. An unnamed id reads "someone".
4. **Render**: `sh .claude/modules/ideas/bin/render.sh <scratchpad>/ideas-dump --names <scratchpad>/ideas-names.json --out <IDEAS_FILE>`.
   It refuses, naming the file, a document it cannot represent: fix that document (set its
   `status` or `text`) and re-run, never skip it. Commit the file with the close.
