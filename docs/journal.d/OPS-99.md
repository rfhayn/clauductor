# OPS-99 — wait for the sources the untrusted-config test reads

- **PR**: #316
- **Session**: S16, started 2026-10-29 08:50 ET
- **Review**: 0 rounds, peak none, cap 3 (pending: merge-pr has not run)
- **Gate**: CANNOT CHECK — pending: the gate binds 127.0.0.1, which this lane's sandbox refuses; the owner decides
- **Cost**: CANNOT CHECK — usage-report.sh sums the whole S16 session (~$3.21 so far), this lane with the main session and OPS-158's lane
- **Session start**: 3758466 tokens, 4 min

## What happened

`TestUntrustedConfigRunsNoCommandsOrTemplates` (#288) read the panel's state once in two places, each before another source goroutine's write could land. It waited for the card's "not run" error, then read the template's suggestions once, but suggest is a source of its own (`suggestSource`). After trust it waited for the card's output, then read `Trust.Trusted` once, but `markTrusted` kicks the cards before the trust source applies its own `ApplyTrust` update. That is the read-before-write shape OPS-133 fixed. The test now waits on each condition it asserts. No sibling test reads state this way: the others trust before the panel starts, or wait on the condition they then check.

The natural failure was NOT reproduced. Beside one copy of the package's short suite, run from a binary path of its own, the old test passed 50 of 50 and the fixed test passed 50 of 50. That run shows the fix causes no regression; it does not show the flake is gone. Two earlier runs of the fixed binary were killed partway, so they are not counts. The load suite's own leakcheck killed them because they ran the same binary path, and the fix is not implicated (see Surprises). What the evidence shows is that the fix closes both windows once each is forced open. A 1 s delay injected into the suggest source's untrusted branch failed the old test 3 of 3 times ("untrusted suggestions: {Source:{OK:false Pending:true ...}}") and the new test 0 of 3. The same delay in `markTrusted`, after the kicks, failed the old test 3 of 3 times ("still untrusted") and the new test 0 of 3. The probes were never committed.

## Decisions

The test waits, and `markTrusted` keeps its order. A card's output can show for a moment before the trust banner lifts, and nothing reads the two together but this test. Reordering production code for it would add review surface without a reader who needs it.

## Surprises

The lane's sandbox refused a loopback bind, and once that was allowed it still could not resolve `clauductor.localhost`, so neither the panel tests nor the gate could run from the lane. The owner ran the evidence scripts by hand. Under that load the natural flake did not show in 50 runs.

The measured runs of the fixed binary were SIGKILLed, and the cause was not the fix. Both evidence scripts ran the load suite and the measured run from one prebuilt binary. After its tests, leakcheck lists every process on the machine and SIGKILLs any running its own binary's path, whoever started it (`internal/leakcheck/leakcheck.go:172-177`). That killed each fixed run, and the earlier load copies killed one another. A run from a different path survived its load suite. `go test` gives each run a binary path of its own, so a gate does not hit this. But leakcheck kills, by pattern, processes it never started, the class ADR-0005 forbids. The lead is opening a row for it.

## Insights

None.
