---
name: architecture-audit
model: opus
effort: high
description: "Check codebase for architectural violations. ⚠️ CONFIGURE FIRST: Define your project's architectural rules below. TRIGGER when the user says \"check architecture\", \"audit architecture\", \"run architecture check\", \"check for violations\", \"architecture review\", or any request to verify architectural patterns."
user_invocable: true
---

# Architecture Audit

## ⚠️ CONFIGURATION REQUIRED

Define your project's architectural rules below. Examples of what to check:

### Example Rules (replace with your own)

**1. Service layer compliance**
```bash
# Views should not call database directly
# Search for direct DB calls in view files
grep -rn 'db\.query\|db\.insert\|db\.delete' src/views/ --include='*.ts'
```
Expected: Zero matches.

**2. Import restrictions**
```bash
# Feature modules should not import from other feature modules
grep -rn "from '@features/" src/features/auth/ --include='*.ts' | grep -v "from '@features/auth"
```
Expected: Zero matches.

**3. API layer separation**
```bash
# Components should not call APIs directly
grep -rn 'fetch(\|axios\.\|httpClient\.' src/components/ --include='*.tsx'
```
Expected: Zero matches.

## How to Configure

1. Identify 3-5 architectural rules your project must follow
2. Write grep/search patterns that detect violations
3. Document the expected result (usually "Zero matches")
4. Replace the examples above with your rules

## When to Run

- Before proposing a change that creates new modules or services (`/clauductor:propose` step 0)
- During review of a task group that touches a core layer
- **Better:** once a rule is stable, turn its grep into a check (`${CLAUDE_PLUGIN_ROOT}/checks/<name>.sh`) or a
  gate step, and give it a row in AGENTS.md's *What executes each rule* table. A rule this skill
  holds is executed only when someone remembers to run it (AGENTS.md rule 4).
