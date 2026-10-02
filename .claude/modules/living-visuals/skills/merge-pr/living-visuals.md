**After the merge: which living pages it moved (the living-visuals module).** A merge that moves a
living page's sources does not refresh the page, and nothing else after the merge would say so. The
list comes from the tool, never from memory: after `git fetch origin`,

```bash
sh .claude/modules/living-visuals/bin/living.sh --list --ref origin/main
```

Each line that is not `OK` names the page, the source that moved, and how it is refreshed
(`/<skill>`, or editing the page). Put them in your report: they are this session's to refresh at
its close (the module's session-close step), where the artifacts module's guard rule refuses the
close's PR until each is refreshed or stamped. Refresh one now only when this session is the close.
A generated block whose source this merge moved is regenerated there too (`living.sh --regen`); the
gate's `living-visuals:pages` fails on it until then, on any branch cut from this `main`.
