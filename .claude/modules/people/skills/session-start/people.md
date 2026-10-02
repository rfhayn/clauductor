**Who is on what, and the lanes** (the people module is on: `.claude/modules/people/README.md`).
In step 3, after the `★` PRs, repeat the context's **who** section: for each person, their `Last`,
`Now` and `Next` lines as printed, one line each, and any *Not mapped to anyone* line. Every line
is derived (people from the registry, `PEOPLE`; work from GitHub; ownership from the roadmap's
`**Owner:**` lines through the one parser), so there is nothing to record at close. A `CANNOT
CHECK` is not "nothing": say it could not be read.

To change who owns a roadmap row, edit the `**Owner:**` line over it in the roadmap; to add a
person or change a lane, edit the registry (`sh .claude/checks/run.sh people:registry` checks it).

**Stay in your lane.** The context's **lanes** section is the table of who owns which part of the
repository, read from the registry's `lanes`. Work inside your own. Crossing into another person's
lane is allowed, but say so in the PR body and prefer asking them first. Never merge another
person's PR (`merge-pr` checks the author; no hook does).
