# The lease conformance kit

The conformance suite of the gate lease protocol (clauductor's `docs/panel.md`, *Queue and the
gate lock protocol*), shipped with the template so that **any** implementation can be tested from
the project, with nothing of clauductor installed: the panel's `clauductor lock-run`, the
template's `scripts/ci/lease.sh`, or a project's own (a `gate-lock.sh` of its own, for example).

| File | What it is |
|---|---|
| `conformance.sh` | the driver: sets up each case's lock from golden records, runs the implementation, prints TAP, exits 1 on a failure |
| `cases/<case>/` | the golden `owner.json` and waiter records, `{{PLACEHOLDERS}}` filled from real processes |
| `run.sh` | the runner: checks the suite against `VERSION`, then runs it (default: against `../lease.sh`) |
| `VERSION` | the protocol version and the suite's sha256 |

## Running it

```sh
sh scripts/ci/lease-conformance/run.sh                         # the template's lease.sh
sh scripts/ci/lease-conformance/run.sh ./my-adapter.sh         # <impl> <lockdir> <lane> <cmd>...
sh scripts/ci/lease-conformance/run.sh --lock-env GATE_LOCK bash infra/ci/gate-lock.sh run --
CASES="dead-pid cancel" sh scripts/ci/lease-conformance/run.sh # some cases (bash conformance.sh --list)
```

The `--lock-env` form needs no adapter: an implementation whose lock path can be overridden by an
environment variable runs the suite as it is. Run the whole suite in your gate, or in a test, once
per change to your implementation; it takes a minute or two (each case waits for real processes).

## The suite is the protocol: do not edit it

`run.sh` refuses to run a suite whose sha256 differs from `VERSION`. If a case fails, fix the
implementation. If the case itself is wrong, fix it in clauductor, where `TestLeaseConformance`
runs it against `lock-run` and `lease.sh` and checks it can fail.

## Refreshing (the sha bump)

`clauductor update` refreshes this directory (the framework tier). In clauductor itself, after any
change to `conformance.sh` or `cases/`:

1. Run `sh template/scripts/ci/lease-conformance/run.sh --sha`. It fails and prints the new sha256.
2. Put it on `VERSION`'s `sha256` line; raise `protocol` only when the protocol itself changed.
3. `cd framework && go test ./internal/template -run LeaseConformanceKit` passes again (it holds
   `VERSION` to the files), and the lease tests run the new suite.
