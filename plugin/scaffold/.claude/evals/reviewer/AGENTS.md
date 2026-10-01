# Working conventions

A small multi-tenant billing service: a Go backend, a TypeScript API, Python jobs and shell
tooling. (This file is the eval suite's fixture: every case's repository gets it, as a project's
reviewer would.)

## Essentials every agent applies

- **Money is integer cents**, never floating point. Rounding is explicit and half-up.
- **Tenant data is scoped.** Every query reading tenant data filters by `tenant_id`; a handler
  checks the caller owns the resource it names.
- **Controls fail closed.** An auth check, a rate limit or quota, a signature check or a CI guard
  that cannot decide denies; it never allows.
- **No error is discarded.** A destructive step runs after the data it replaces is safe, or in a
  transaction.
- **Every behaviour a task names has a test** that would fail without the code.
- **Shell scripts are POSIX sh** and run on macOS (BSD tools) and Linux (GNU tools) alike.
