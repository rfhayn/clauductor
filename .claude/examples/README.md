# The example change

A complete, well-formed change in the shape `changes/README.md` gives, with the living spec it
modifies and the test that cites its scenarios. The checks run against it, so it stays correct:

- `checks/changes.sh` holds it to every rule an open change must meet (except approval: it stays
  `awaiting approval`, and the check approves a copy of it with `.claude/change-approval.sh`);
- `checks/scenarios.sh` traces its scenario IDs to `test/greeting.sh`, and to the `(manual: …)`
  line in its `tasks.md`;
- `checks/openspec.sh` runs `openspec validate --all --strict` on it through the OpenSpec module's
  symlinks, when the OpenSpec CLI (1.13 or later) is installed.

Copy its shape; do not copy it into `changes/`.
