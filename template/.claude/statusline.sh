#!/bin/sh
# Clauductor status line for Claude Code
# Reads current session state and displays milestone, session type, and context usage.
# Updated dynamically by /session-start, /claim, and other framework skills.

input=$(cat)

# Extract context usage percentage
used=$(echo "$input" | jq -r '.context_window.used_percentage // empty')

# Get current working directory and branch
cwd=$(echo "$input" | jq -r '.workspace.current_dir // .cwd // empty')
branch=""
if [ -n "$cwd" ]; then
  branch=$(git -C "$cwd" --no-optional-locks branch --show-current 2>/dev/null)
fi

# --- Read Clauductor session state ---

label=""
session_type=""

# Option 1: Read from orchestration status file (lightweight, no SQLite dependency)
status_file=""
if [ -n "$cwd" ]; then
  status_file="$cwd/orchestration/.session-status"
fi

if [ -n "$status_file" ] && [ -f "$status_file" ]; then
  # Format: MILESTONE|SESSION_TYPE|WORKER_NAME|DESCRIPTION
  milestone=$(cut -d'|' -f1 "$status_file")
  session_type=$(cut -d'|' -f2 "$status_file")
  worker=$(cut -d'|' -f3 "$status_file")
  desc=$(cut -d'|' -f4 "$status_file")

  if [ -n "$milestone" ]; then
    type_badge=""
    case "$session_type" in
      build)    type_badge="BUILD" ;;
      test)     type_badge="TEST" ;;
      research) type_badge="RESEARCH" ;;
      spike)    type_badge="SPIKE" ;;
    esac

    if [ -n "$type_badge" ] && [ -n "$desc" ]; then
      label="[${milestone}] ${type_badge} ${desc}"
    elif [ -n "$type_badge" ]; then
      label="[${milestone}] ${type_badge}"
    elif [ -n "$desc" ]; then
      label="[${milestone}] ${desc}"
    else
      label="[${milestone}]"
    fi
  fi
fi

# Option 2: Fallback — parse milestone from git branch name
if [ -z "$label" ] && [ -n "$branch" ]; then
  milestone=$(echo "$branch" | grep -oE 'M[0-9]+\.[0-9]+(\.[0-9]+)?' | head -1)
  if [ -n "$milestone" ]; then
    desc=$(echo "$branch" | sed "s|.*${milestone}-||" | tr '-' ' ')
    [ "$desc" = "$branch" ] && desc=""
    if [ -n "$desc" ]; then
      label="[${milestone}] ${desc}"
    else
      label="[${milestone}]"
    fi
  else
    # Not on a milestone branch — show branch name
    label="$branch"
  fi
fi

# Build context part
ctx=""
if [ -n "$used" ]; then
  ctx="$(printf '%.0f' "$used")% ctx"
fi

# Output
if [ -n "$label" ] && [ -n "$ctx" ]; then
  echo "${label} | ${ctx}"
elif [ -n "$label" ]; then
  echo "$label"
elif [ -n "$ctx" ]; then
  echo "$ctx"
fi
