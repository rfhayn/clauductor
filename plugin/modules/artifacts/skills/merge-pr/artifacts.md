**After step 5: publish the shared copies this merge recorded (the artifacts module, only with
`ARTIFACT_PUBLISH="claude.ai"`).** A merge that changed a page's `published` hash in the registry
has told `main` that a new copy is live. It is not live until someone publishes it, and nothing
after the merge would notice, so it happens here, in whichever session merged. The list comes from
the merge commit, never from memory:

```bash
clauductor-model modules/artifacts/bin/publish.sh --recorded-in "$(gh pr view <n> --json mergeCommit --jq .mergeCommit.oid)"
```

No lines: nothing to publish. For each `<page> <url>` line, apply the ownership test (the module's
session-start step), then `clauductor-model modules/artifacts/bin/publish.sh <page> --ref origin/main`
for the prepared copy, Artifact `action: "read"` on the URL, **Read in full** both copies, and
publish with `url`, no `icon`, no `capabilities`; then `ArtifactComments` `watch` `on: false`. Skip
a page this session's start already published, when the `now` hash it printed equals the recorded
one. **A publish that fails is Blocked**, naming the page: `main` claims a copy that is not live.
Not an owner: put the pages in the notification; the owner's session publishes them. No Artifact
tool in this session: `CANNOT CHECK — no Artifact tool`, and the pages go in the notification.
