# Owner queue: things that need the owner at the computer

`session-start` prints every open item here at the top of its context, every session, until it is
ticked; the panel pins the same list as a card. This is where a session puts work it cannot do
unattended: an interactive login, a production deploy, a decision that needs a screen, anything
irreversible.

**How to use it.** One item per line, `- [ ]` open and `- [x]` done, with the date it was queued,
what it needs first, and the change or PR it belongs to. Tick it (or delete it) in the PR that does
the work. The printer is `${CLAUDE_PLUGIN_ROOT}/owner-queue.sh`; `${CLAUDE_PLUGIN_ROOT}/checks/owner-queue.sh` holds it to
printing every open item and nothing else.

## Open

## Done
