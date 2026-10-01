# Specs: what the system does, per capability

One file per capability, `specs/<capability>/spec.md`, stating what the system does **now**. A
requirement joins a spec when the change that built it merges and is archived, never when it is
proposed: never write down that the system does something until a production process does it
(`docs/principles.md`).

```markdown
# <Capability>

## Purpose
<one paragraph, at least 50 characters: what this capability is for>

## Requirements

### Requirement: <name>
The system SHALL <behaviour>.

#### Scenario: [CAP-1-S1] <title>
- **GIVEN** <the state before>
- **WHEN** <the situation or action>
- **THEN** <the observable result>
- **AND** <another result, or condition>
```

## Scenario IDs

Every scenario header is `#### Scenario: [CAP-n-Sn] <title>`.

- The ID is `<CAPABILITY>-<requirement n>-S<scenario n>`: `AUTH-2-S1` is the first scenario of the
  second requirement of `auth`. The capability part is upper case and may contain hyphens
  (`MEMBER-INVITES-1-S1`); every scenario of one requirement shares its `CAP-n`.
- **An ID never changes and is never reused.** A removed requirement's scenario IDs are retired.
  A living scenario's whole header stays as it is: a MODIFIED block copies it word for word
  (OpenSpec matches scenarios by header, and deletes one left out), so change a scenario by editing
  its bullets, or add a new scenario with a new ID. `${CLAUDE_PLUGIN_ROOT}/checks/changes.sh` refuses a missing,
  malformed or duplicate ID, and one already held by a living spec or an archived change.
- EARS wording is welcome on the requirement line: "WHEN <trigger> THE SYSTEM SHALL <response>".
- `SCENARIO_IDS="new-only"` in `.claude/project.conf` lets living specs keep scenarios written
  before IDs existed; a change's deltas still need them.

## Every scenario has a test

- Every requirement has a SHALL or MUST line and at least one scenario, and every scenario is
  something a test can check.
- **The test names the scenario's ID** (in its name, a comment or a tag) and asserts its THEN.
  `${CLAUDE_PLUGIN_ROOT}/scenario-trace.sh` lists each ID and the test files that cite it; the gate fails a
  living scenario no test cites, so deleting the last test of a scenario is a red gate. The
  reviewer checks that each citing test really asserts the THEN.
- A scenario no test can reach was escaped, with a reason, in the `tasks.md` of the change that
  added it (`(manual: …)` or `(untestable: …)`); that line stays in the archive and still counts.
- `/clauductor:propose` reads the specs a change touches **before** drafting it, and says in the proposal what
  they already guarantee.
- The format is OpenSpec's, so its CLI can read these files through the optional module's
  symlinks (`${CLAUDE_PLUGIN_ROOT}/modules/openspec`) without a rewrite.
