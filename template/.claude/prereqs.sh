#!/bin/sh
# prereqs.sh: report which tools the operating model uses are present and which are missing, with
# an install hint for this OS. It WARNS and never fails (exit 0 always): a missing tool degrades
# one step, and the report says which, so the reader decides.
#
#   sh .claude/prereqs.sh
#
# Run by `clauductor install` and `clauductor init` (from the template they install), by the
# plugin's /clauductor:init (scaffold.sh), and by install.sh. Plain POSIX sh with no tool beyond
# `uname`, so it runs on a machine missing everything it reports on.
#
# PREREQS_OS overrides `uname -s` (Darwin, Linux, WSL): the checks use it to see each OS's hints.

os=${PREREQS_OS:-$(uname -s 2>/dev/null)}
if [ -z "${PREREQS_OS:-}" ] && [ "$os" = Linux ] && [ -r /proc/version ]; then
  # WSL2 reports Linux; its kernel string names Microsoft.
  read -r _v < /proc/version || _v=""
  case $_v in *[Mm]icrosoft*) os=WSL ;; esac
fi
case $os in
  Darwin) label="macOS" ;;
  WSL) label="Linux (WSL2)" ;;
  Linux) label="Linux" ;;
  *) label="$os" ;;
esac

# hint TOOL: one line saying how to install TOOL here.
hint() {
  case $os in
    Darwin)
      case $1 in
        git) echo "xcode-select --install   (or: brew install git)" ;;
        node) echo "brew install node" ;;
        claude) echo "see https://claude.ai/code" ;;
        *) echo "brew install $1" ;;
      esac ;;
    Linux | WSL)
      case $1 in
        git | jq | tmux) echo "sudo apt install $1   (or your distribution's package manager)" ;;
        gh) echo "see https://github.com/cli/cli/blob/trunk/docs/install_linux.md   (or: brew install gh)" ;;
        gitleaks) echo "a release binary from https://github.com/gitleaks/gitleaks/releases   (or: brew install gitleaks)" ;;
        node) echo "sudo apt install nodejs npm   (or: https://nodejs.org)" ;;
        claude) echo "see https://claude.ai/code" ;;
      esac ;;
    *)
      echo "see the tool's own install page (on Windows, run clauductor under WSL2 and use the Linux hints)" ;;
  esac
}

missing=0
# row LEVEL TOOL WHY: LEVEL is required, recommended or optional; WHY says what degrades without it.
row() {
  if command -v "$2" >/dev/null 2>&1; then
    printf '  ok       %-9s %s\n' "$2" "$(command -v "$2")"
  else
    missing=$((missing + 1))
    printf '  MISSING  %-9s %s: %s\n' "$2" "$1" "$3"
    printf '           %-9s install: %s\n' "" "$(hint "$2")"
  fi
}

echo "Prerequisites on $label (a report: nothing here stops the install):"
row required git "branches, worktrees and the gate all run on git"
row required jq "the hooks, checks and context scripts read JSON with jq"
row recommended gh "merge-pr, the PR flow and the health lines call the GitHub CLI"
row recommended gitleaks "the gate's secret scan SKIPS locally without it and FAILS under CI"
row recommended claude "Claude Code runs the sessions, skills and agents"
row optional tmux "the panel's lanes (the panel runs on macOS)"
row optional node "build-change's process check exercises the workflow with node; without it that check SKIPS"
if [ "$missing" -eq 0 ]; then
  echo "  All present."
else
  echo "  $missing missing. Install them and re-run: sh .claude/prereqs.sh"
fi
exit 0
