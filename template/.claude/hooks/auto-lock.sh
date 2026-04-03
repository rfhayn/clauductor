#!/usr/bin/env bash
# Hook: auto-lock.sh
# Event: PreToolUse (sync, matcher: Edit|Write)
# Purpose: Auto-lock files on edit, block if locked by another worker
#
# Replaces lock-guard.sh — active locking instead of passive warnings.

set -euo pipefail

PROJECT_DIR="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

# Graceful degradation: no orchestration = no-op
test -d "$PROJECT_DIR/orchestration" || exit 0

# Read hook input JSON from stdin
INPUT="$(cat)"

# Extract file path from tool_input
FILE_PATH="$(echo "$INPUT" | jq -r '.tool_input.file_path // empty' 2>/dev/null)"
if [ -z "$FILE_PATH" ]; then
  exit 0
fi

# Make path relative to project root for consistent lock matching
FILE_PATH="$(realpath --relative-to="$PROJECT_DIR" "$FILE_PATH" 2>/dev/null || echo "$FILE_PATH")"

# Derive worker identity (same logic as session-register.sh)
BRANCH="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "")"

# Supervisor on main — skip auto-locking (supervisor doesn't edit code)
if [ "$BRANCH" = "main" ] || [ "$BRANCH" = "HEAD" ] || [ -z "$BRANCH" ]; then
  exit 0
fi

# Extract milestone from branch name
MILESTONE="$(echo "$BRANCH" | grep -oE '(feature/)?[A-Z]+-[0-9]+' | sed 's|^feature/||' || echo "")"
if [ -z "$MILESTONE" ]; then
  MILESTONE="$(echo "$BRANCH" | grep -oE '(feature/)?M[0-9]+\.[0-9]+' | sed 's|^feature/||' || echo "")"
fi

if [ -z "$MILESTONE" ]; then
  exit 0
fi

WORKER_ID="${USER:-unknown}-${MILESTONE}-build"

# Auto-lock via CLI — atomic check + lock
RESULT="$(clauductor auto-lock --worker-id "$WORKER_ID" --file "$FILE_PATH" 2>/dev/null)" || {
  EXIT_CODE=$?
  # Exit 1 = conflict (blocked)
  if [ $EXIT_CODE -eq 1 ] && [ -n "$RESULT" ]; then
    LOCK_OWNER="$(echo "$RESULT" | jq -r '.conflict.owner // "unknown"' 2>/dev/null)"
    MSG="🔒 BLOCKED: '$FILE_PATH' is locked by worker '$LOCK_OWNER'. Coordinate with them or wait for the lock to be released."
    jq -n --arg msg "$MSG" '{
      hookSpecificOutput: {
        hookEventName: "PreToolUse",
        permissionDecision: "deny",
        permissionDecisionReason: $msg
      }
    }'
    exit 0
  fi
  # Other errors — allow gracefully
  exit 0
}

# Success — edit allowed (file now locked by this worker)
exit 0
