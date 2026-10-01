---
name: release-prep
model: opus
effort: high
description: "Deploy what is on main to an environment. CONFIGURE FIRST: put your deployment steps in step 3. A production deploy is the owner's decision (AGENTS.md, Who decides), so this skill prepares and verifies, and runs the deploy only on the owner's explicit go. TRIGGER when the user says 'release this', 'deploy', 'push to production', 'go live', or asks to take merged work to an environment."
---

# Release prep: from main to an environment

**Merging is not deploying.** Changes reach `main` through `merge-pr`; this skill takes what is
on `main` to an environment. A production deploy, and anything irreversible on the way (a data
migration, a destructive config change), is the owner's decision: prepare everything, show what
will happen, and run it only when the owner says go in this conversation. If they are not here,
queue it in the owner queue (`OWNER_QUEUE` in `.claude/project.conf`) and stop.

## Context
- Branch: !`git branch --show-current`
- Last deployed / tagged: !`git describe --tags --abbrev=0 2>/dev/null || echo "no tags"`
- On main since then: !`git log --oneline "$(git describe --tags --abbrev=0 2>/dev/null || git rev-list --max-parents=0 HEAD | tail -1)"..origin/main 2>/dev/null | head -30`

If those lines show as literal text, run the commands yourself.

## Step 1: what ships

List the commits and PRs since the last release, and anything in them that needs a step beyond
the deploy itself (a migration, a new secret, a config key). Name each one's owner.

## Step 2: evidence

The release commit must have gate evidence: a receipt from a complete clean `GATE_RUN` for that
SHA, or your remote CI's success for it. Name the SHA the evidence covers.

## Step 3: deploy (CONFIGURE)

```bash
# Replace with your deployment steps, e.g.:
#   git tag vX.Y.Z && git push origin vX.Y.Z
#   ./deploy.sh production <sha>
echo "DEPLOYMENT NOT CONFIGURED: edit .claude/skills/release-prep/SKILL.md step 3"
```

## Step 4: verify by existence

Check the environment itself (a health endpoint, the running version, the migration table), never
the deploy script's exit code (AGENTS.md rule 2). Report the subject: which SHA is now running
where, and how you know.
