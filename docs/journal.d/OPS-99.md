# OPS-99 — wait for the sources the untrusted-config test reads

- **PR**: #316
- **Session**: S16, started 2026-10-29 08:50 ET
- **Review**: 0 rounds, peak none, cap 3 (pending: merge-pr has not run)
- **Gate**: CANNOT CHECK — pending: the gate binds 127.0.0.1, which this lane's sandbox refuses; the owner decides
- **Cost**: CANNOT CHECK — usage-report.sh sums the whole S16 session (~$3.21 so far), this lane with the main session and OPS-158's lane
- **Session start**: 3758466 tokens, 4 min

## What happened

`TestUntrustedConfigRunsNoCommandsOrTemplates` (#288) read the panel's state once in two places, each before another source goroutine's write could land. It waited for the card's "not run" error, then read the template's suggestions once, but suggest is a source of its own (`suggestSource`). After trust it waited for the card's output, then read `Trust.Trusted` once, but `markTrusted` kicks the cards before the trust source applies its own `ApplyTrust` update. That is the read-before-write shape OPS-133 fixed. The test now waits on each condition it asserts. No sibling test reads state this way: the others trust before the panel starts, or wait on the condition they then check.

The natural failure was NOT reproduced. In 50 runs beside two copies of the package's short suite, the before binary passed all 50. The after binary's load run was killed by an outside SIGKILL before it printed anything, so it counts nothing. What the evidence shows is that the fix closes both windows once each is forced open. A 1 s delay injected into the suggest source's untrusted branch failed the old test 3 of 3 times ("untrusted suggestions: {Source:{OK:false Pending:true ...}}") and the new test 0 of 3. The same delay in `markTrusted`, after the kicks, failed the old test 3 of 3 times ("still untrusted") and the new test 0 of 3. The probes were never committed.

## Decisions

The test waits, and `markTrusted` keeps its order. A card's output can show for a moment before the trust banner lifts, and nothing reads the two together but this test. Reordering production code for it would add review surface without a reader who needs it.

## Surprises

The lane's sandbox refuses a loopback bind, so neither the panel tests nor the gate could run from the lane. The owner ran the evidence script by hand. Under that load the natural flake did not show in 50 runs.

## Insights

None.
