# Principles behind the operating model

The skills, hooks and agents in `.claude/` cite these by name (*in italics*). Each one was learned
the expensive way in the project this model was extracted from; the incident is summarised in a
line so you can judge whether it applies to you. They are not rules on their own: `AGENTS.md`
holds the four rules, and its *What executes each rule* table says what, if anything, enforces
each. When your project decides something of this kind, record it as an ADR (`/clauductor:new-adr`) and cite
the ADR instead.

## Propose just in time, at most one ahead

A change is proposed when it is the next thing to be built, not when its roadmap row is written.
Until then the row is the scoping unit: it makes no design claims, so it cannot go stale the way a
proposal, a design and a task list can. At most one change sits proposed-but-unbuilt. A second
proposal written ahead of the first build is inventory that goes stale the moment the first build
teaches something.

## Every change states its slice

A change's `tasks.md` ends with `Slice: a <role> can <action> at <where>`, or `Slice: exempt —
<reason>` for pure substrate. It is a presence check, not a correctness check: it makes the
question "what can someone now do that they could not?" be answered out loud on every change. An
exemption written just to clear the check reproduces the defect the rule exists to catch.

## A diff that fits in one sentence gets no proposal

A proposal costs the owner a review, so it has to buy one. A typo, a bump or a one-line fix goes
straight to a `fix/` or `ops/` lane; a change to what a user can do gets a proposal. This is the
threshold Anthropic gives for when a plan is worth writing, and Kiro's "quick spec".

## An approval covers the design as written

The owner approves a design, not a directory. The Approved line carries a hash of `design.md` (and
the risk tier), so an edit after approval is visibly a new design awaiting a new approval, rather
than an old approval silently stretched over it.

## A scenario is proven by a test that names it

Every scenario has an ID that never changes, and the test that proves it cites the ID. A check
can then find an uncited scenario, and a deleted test that leaves one uncited, in any language
with one grep. The citation proves only that a test NAMES the scenario; a reviewer still checks
that it asserts the THEN. A scenario no test can reach says why, in the change's `tasks.md`.

## Set an appetite, and say how you will know

A change carries a cost budget (Shape Up's appetite, not an estimate) and a signal to look for
after it ships. The budget stops a build that is running away; the signal, queued as a dated
roadmap row, stops "merged" from being mistaken for "worked".

## Never write down that the system does something until a production process does it

Specs describe what the system does. A spec delta joins the living specs when its change merges
(archive), not when it is proposed. This is rule 3 in documentation form.

## A passing test is evidence of nothing until it has failed

For a test that protects a property, break the property, watch the test go red, restore it, and
record what the failure said. Name the input that makes the path SUCCEED: a feature whose every
assertion is a rejection has had its guards tested and not itself. When several unrelated tests
fail on one change, suspect the shared fixture first.

## A falsification can fail at any link in its chain

Confirm the injected defect actually landed before trusting the run. Inject each defect separately
and read the messages: two tests that fail with one message are one test. Ask what the
environment already supplies (a timezone, a locale, an empty default) and make it disagree. Ask
what the fixture cannot express. And get a second reader, because a falsification you author only
exercises the failure modes you already had in mind.

## Size a change so one review round converges

Measure two numbers per review round: all findings, and the findings inside the previous round's
fixes. Zero in the second column is convergence. If it is flat or rising, the change exceeds what
one reviewer can hold: land what is provably safe, defer the rest to named changes, and split.
Calibrate on distinct write surfaces (roughly: fewer than four, one screen or entry point), not on
lines changed, which inverts the ranking.

## A review's blind spot is set by its framing

Review per task group, not per PR: a per-group cadence catches a fix applied at N-1 of N sites,
which looks complete from inside the group and like an ordinary bug to one end-of-change reviewer.
Do not tell the reviewer what the builder intended; its blind spots should differ. Treat a claim
in prose ("this cannot happen because…") as an assertion to verify.

## Reach is set by structure, not by the prompt

An agent that must not edit gets a `tools:` line without Edit and Write; a sentence saying "do not
edit" is not a control. Research goes to a read-only agent type, never to a fork, which inherits
the parent's whole toolset. Verify what an agent did (`git status`, `git diff --stat`), never what
it reports.

## A control needs a named addressee

Every control declares who reads it when it FAILS, and whether that is where they already look. A
scheduled job, a cron or a timer has no natural reader, so its result is routed to one explicitly:
in this model, the `session-start` context block. A green result must name its SUBJECT (the commit,
the file, the input it examined): a pass over a stale subject is consumed as coverage.

## Enumerate the authority; a written list is a sample

A check over a set derives the set at run time from what answers for itself (the filesystem, `git
ls-files`, the router, the database catalogue), never from a list someone typed. Run the negative
case: these degrade to a smaller plausible number, not to an error.

## A check that reads source is a parser

It fails loudly on a shape it cannot classify, instead of skipping it. It strips comments before
matching. It matches the property, not a spelling. It walks everything and excludes by reason,
rather than listing roots. Where the system can answer directly, ask it instead of parsing.

## Nothing checks a premise

A sentence with "because", "since" or "so that" in it goes stale silently. A task written before
the code existed is thinking about the code, not a fact about it: re-check the premise against the
code as built before executing it. A fix starts from the code, not from the issue's write-up.

## The gate runs locally by default

One definition of the gate's steps, shared by the local runner and any remote CI. The local runner
is the default evidence; remote CI runs on demand. The merge guard asks for evidence BY NAME for the
exact head commit (a receipt from a complete, clean local run, or the named remote workflow's
success), never for "the checks" in general. Only a complete run writes a receipt.

## One machine, one gate at a time

Every worktree of a repository shares one gate lease (`docs/panel.md`, *Queue and the gate lock
protocol* in the clauductor repo), so two lanes never run the full gate at once and starve each
other into false reds. A false red is not harmless: the second one teaches you to distrust the
gate.
