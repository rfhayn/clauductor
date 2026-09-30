# The gate

| File | What it is |
|---|---|
| `steps.sh` | **Yours.** The one definition of the gate's steps: `gate_steps full\|quick`. Put your lint, typecheck and test commands here. A remote CI, if you have one, calls it too. |
| `run-local.sh` | The full gate (`GATE_RUN`). Takes the machine-wide gate lease, runs the steps, and writes the receipt after a complete, clean run. |
| `gate.sh` | The same, for an agent (`GATE`): the log goes to `<git-dir>/ci-gate.log`, stdout gets markers, failure lines and a tail. |
| `lease.sh` | The lease protocol in plain shell, vendored verbatim from clauductor's `docs/panel.md`. Used when `clauductor` is not installed. |

## The lease

Every worktree of this repository shares one gate lease, a directory at
`<git common dir>/clauductor/gate.lock`, so two lanes never run the full gate at once (two gates
binding one port or one test database starve each other into false reds). `run-local.sh` takes it
first thing: through `clauductor lock-run` when clauductor is installed, otherwise through
`lease.sh`. Both implement the same on-disk protocol, so the panel shows who holds the gate and who
waits, and can cancel a wait, either way. The protocol, its staleness rules and its conformance
suite are in the clauductor repo's `docs/panel.md`, *Queue and the gate lock protocol*.

## The receipt contract

`run-local.sh` writes `$(git rev-parse --git-dir)/ci-receipt` after a complete run, and only then:

```
<sha> TAB full TAB clean|dirty TAB all
```

- `<sha>` is the commit tested, resolved before the run started.
- `clean` only when the tested tree was exactly that commit (see `GATE_CLEAN_ROOM` in
  `run-local.sh`'s header); otherwise `dirty`.
- A failed full run deletes the receipt; a `--quick` run neither writes nor deletes one.

`.claude/hooks/pr-merge-guard.sh` rule 2 accepts a `clean`, `all` receipt naming the PR's head SHA,
found in the git dir of ANY worktree of this repository, or the named remote workflow's success on
that SHA (`GATE_REMOTE_WORKFLOW`). Nothing else counts: an unrelated green check is not evidence.
The receipt stays in the git dir, so it is per clone and never travels.

## Remote CI (optional)

The local gate is the default evidence. If you also run CI remotely, have its workflow call
`scripts/ci/steps.sh` (so the two cannot drift), name it in `GATE_REMOTE_WORKFLOW`, and trigger it
on demand (`gh workflow run <file> --ref <branch>`) or on push to `main`. Do not add a
`pull_request` trigger with a skip condition: a workflow whose jobs all skip still produces a run,
and a green run for skipped work reads as evidence.
