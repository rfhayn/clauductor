# Project Naming Standards

## Naming Format

```
M[Major].[Component].[Task]

M1       = Major Feature
M1.2     = Component within that feature
M1.2.3   = Task within that component
```

## Status Indicators

| Icon | Status | Meaning |
|------|--------|---------|
| ✅ | COMPLETE | Fully implemented and validated |
| 🔄 | ACTIVE | Currently in development |
| 🚀 | READY | Ready to start |
| ⏳ | PLANNED | Future work |

## Rules

1. **Always use full identifier**: `M4.1.1` not "Phase 1" or "Step 1"
2. **Include descriptive name**: `M4.1.1: Core Settings Service`
3. **Sequential numbers**: Don't skip or reuse
4. **Consistent everywhere**: Code, commits, docs, branches

## Branch Naming

```
feature/M#.#.#-brief-kebab-case    (3-5 words max)
```

## Commit Messages

```
M#.#.#: Brief imperative description

- Detail 1
- Detail 2
```

No Co-Authored-By line.

## PR Titles

```
M#.#.#: Brief Descriptive Title    (under 70 characters)
```

## Scope Guidelines

| Level | Format | Scope | Example |
|-------|--------|-------|---------|
| Major Feature | M# | 15-60+ hours | M1: Authentication |
| Component | M#.# | 1-20 hours | M1.2: OAuth Integration |
| Task | M#.#.# | 15 min - 3 hours | M1.2.3: Google Provider |

## Forward-Only Policy

Don't rename historical work. Apply current standards to new work only.
