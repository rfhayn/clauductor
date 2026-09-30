# Specs: what the system does, per capability

One file per capability, `specs/<capability>/spec.md`, stating what the system does **now**. A
requirement joins a spec when the change that built it merges and is archived, never when it is
proposed: never write down that the system does something until a production process does it
(`docs/principles.md`).

```markdown
# <Capability>

## Purpose
<one paragraph: what this capability is for>

## Requirements

### Requirement: <name>
The system SHALL <behaviour>.

#### Scenario: <name>
- **WHEN** <situation>
- **THEN** <observable result>
```

- Every requirement has at least one scenario, and every scenario is something a test can check.
  A reviewer treats a scenario with no test as a finding.
- `/propose` reads the specs a change touches **before** drafting it, and says in the proposal what
  they already guarantee.
- The format is OpenSpec's, so switching to the optional OpenSpec module later needs no rewrite.
