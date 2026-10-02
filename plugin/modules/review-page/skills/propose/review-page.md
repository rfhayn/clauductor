The review-page module is on (`REVIEW_PAGE="artifact"`). Step 3 builds and publishes the owner's
review page as `${CLAUDE_PLUGIN_ROOT}/modules/review-page/README.md` describes, and `proposal.md` line 1 is
`**Review page:** https://claude.ai/artifact/<id>`. A session without the Artifact tool falls back
to the PR form and says so: CANNOT CHECK — no Artifact tool.
