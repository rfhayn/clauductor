# Project Naming Standards

## Naming Format

```
PREFIX-[Feature].[Task]

AUTH        = Epic (domain area, defined in prefix registry)
AUTH-1      = Feature within that epic
AUTH-1.3    = Task within that feature
```

### Prefix Registry

Define your project's epic prefixes here. Each prefix should be 2-5 uppercase characters that clearly identify a domain area.

| Prefix | Domain | Status |
|--------|--------|--------|
| _TODO_ | _Define your first epic prefix_ | PLANNED |

> **Examples from other projects:**
> `AUTH` = Authentication, `DASH` = Dashboard, `API` = Public API, `DATA` = Data layer, `UI` = User interface

## Status Indicators

| Status | Meaning |
|--------|---------|
| COMPLETE | Fully implemented and validated |
| ACTIVE | Currently in development |
| READY | Ready to start |
| PLANNED | Future work |

## Rules

1. **Always use full identifier**: `AUTH-1.3` not "Phase 1" or "Step 1"
2. **Include descriptive name**: `AUTH-1.3: OAuth callback handler`
3. **Sequential numbers**: Don't skip or reuse within a prefix
4. **Consistent everywhere**: Code, commits, docs, branches
5. **Register prefixes**: Add every new epic prefix to the registry above

## Depth Flexibility

Choose the granularity that fits the work:
- `AUTH-1` — for features that don't need sub-task tracking
- `AUTH-1.3` — for tasks within a feature
- Sub-tasks below that are checklist items in the task file, not separate IDs

## Branch Naming

```
feature/AUTH-1.3-brief-kebab-case    (3-5 words max)
```

## Commit Messages

```
AUTH-1.3: Brief imperative description

- Detail 1
- Detail 2
```

No Co-Authored-By line.

## PR Titles

```
AUTH-1.3: Brief Descriptive Title    (under 70 characters)
```

## Scope Guidelines

| Level | Format | Scope | Example |
|-------|--------|-------|---------|
| Epic | PREFIX | 15-60+ hours | AUTH: Authentication |
| Feature | PREFIX-# | 1-20 hours | AUTH-1: OAuth Integration |
| Task | PREFIX-#.# | 15 min - 3 hours | AUTH-1.3: Google Provider |

## Legacy Support

Projects using the M#.#.# format (e.g., M1.2.3) continue to work. The framework supports both conventions. New projects should use PREFIX-#.#.

## Forward-Only Policy

Don't rename historical work. Apply current standards to new work only.
