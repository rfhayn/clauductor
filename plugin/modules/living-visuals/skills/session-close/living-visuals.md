**Refresh the living pages, then hold them true (the living-visuals module).** This is the refresh
half of the artifacts module's close step: do it after step 3's roadmap edits and before that step
stamps, because a stamp taken before a page is refreshed records a review that did not happen. The
Context block's *living-visuals (module living-visuals)* section lists each living page's currency
and every page rule failing on this tree.

1. **Regenerate every generated block**: `clauductor-model modules/living-visuals/bin/living.sh --regen`.
   A generated block is never edited by hand; its command's output is the page's content there, so
   a roadmap edit that moved a queue row is carried by this step, not by a skill.
2. **Refresh each living page that is BEHIND.** Its line names how: `/<skill>` (run that skill, which
   ends by stamping) or "editing the page". Read the page against what moved before you write: a
   refresh changes what the page states, never only its date. A page that the moved source does not
   affect is stamped `reviewed, no change: <why>` (the artifacts module's step) instead.
3. **Hold them true**: `clauductor-model checks/run.sh living-visuals:pages` must pass. A failing claim is
   a page stating something its authority does not: fix the page (or, if the authority is wrong,
   fix the authority in its own change). A count added to a page as CURRENT STATE gets a
   `data-claim="<name>"` and a command in its registry entry's `claims`, or it is invisible to
   this check. An elapsed duration is never typed: `<span data-days-since="YYYY-MM-DD">`.
4. Then the artifacts module's step stamps each page, and its last currency check runs after
   step 6's merge of `origin/main`; if that merge moved a generated block's source, repeat 1 and 3.
