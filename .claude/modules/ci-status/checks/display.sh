#!/bin/sh
# The ci-status module's own check, run by checks/run.sh only while the module is on:
#   - every context publish-status.sh posts is in GATE_DISPLAY_CONTEXTS, so the merge guard ignores
#     it both ways (the publisher refuses any other, so a missing one is a status never drawn);
#   - something real calls the publisher (AGENTS.md rule 3): the model's runner through
#     lib/steps.sh model_publish_status, or a project's own GATE_RUN naming it in code, not a comment.
# The publisher's payload and the guard's indifference are tested in .claude/checks/ci-status.sh.
# shellcheck disable=SC1090
. "${CHECKS_LIB:?run this through checks/run.sh}"

for c in "${CI_STATUS_LOCAL_CONTEXT:-ci/local}" "${CI_STATUS_REMOTE_CONTEXT:-ci/github}"; do
  case " $GATE_DISPLAY_CONTEXTS " in
    *" $c "*) ok "$c is in GATE_DISPLAY_CONTEXTS: the merge guard ignores it both ways" ;;
    *) fail "$c is not in GATE_DISPLAY_CONTEXTS (\"$GATE_DISPLAY_CONTEXTS\"): publish-status.sh will refuse to post it; add it there" ;;
  esac
done

# code FILE: FILE without its comments, so a mention in prose does not count as a call.
code() { sed 's/#.*$//' "$1" 2>/dev/null; }
run="$ROOT/$GATE_RUN"
if [ ! -f "$run" ]; then
  fail "GATE_RUN=$GATE_RUN does not exist, so nothing posts the status"
elif code "$run" | grep -q 'model_publish_status'; then
  lib="$(dirname "$run")/lib/steps.sh"
  if code "$lib" | grep -q 'publish-status\.sh'; then ok "GATE_RUN=$GATE_RUN posts the status after a full run (model_publish_status, $(dirname "$GATE_RUN")/lib/steps.sh)"
  else fail "GATE_RUN=$GATE_RUN calls model_publish_status, but $(dirname "$GATE_RUN")/lib/steps.sh does not run publish-status.sh"; fi
elif code "$run" | grep -q 'publish-status\.sh'; then
  ok "GATE_RUN=$GATE_RUN calls publish-status.sh itself"
else
  fail "GATE_RUN=$GATE_RUN never calls the publisher: after a full run, call model_publish_status pass|fail <sha> (scripts/ci/lib/steps.sh) or .claude/modules/ci-status/scripts/publish-status.sh local pass|fail"
fi
finish
