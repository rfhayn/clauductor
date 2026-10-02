#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
FRAMEWORK_DIR="$SCRIPT_DIR"

echo "==================================="
echo "  Clauductor Framework Installer"
echo "==================================="
echo ""

# --- Prerequisites ---
#
# A missing tool is OFFERED, never installed unasked, and never with sudo: on macOS the offer is
# `brew install` (Homebrew installs into its own prefix, as the user); anywhere else the
# prerequisites report at the end prints the command to run yourself. With no terminal to ask
# (a CI job, a script), or CLAUDUCTOR_INSTALL_ASSUME=no, every offer is answered "no". Re-running
# is safe: a tool already present is not offered again.

OS="$(uname -s)"

# ask PROMPT: true only on an explicit y or Y typed at a terminal. /dev/tty, not stdin, when stdin
# is not one: under `curl … | bash` stdin is this script, and reading it would eat the script.
ask() {
    local response=""
    [[ "${CLAUDUCTOR_INSTALL_ASSUME:-}" == no ]] && return 1
    if [[ -t 0 ]]; then
        read -r -p "$1" response || return 1
    elif { : < /dev/tty; } 2>/dev/null; then
        read -r -p "$1" response < /dev/tty || return 1
    else
        return 1
    fi
    [[ "$response" == y || "$response" == Y ]]
}

# offer TOOL WHY: report TOOL, and offer to install it when it is missing. True when TOOL is present
# afterwards.
offer() {
    local tool=$1 why=$2
    if command -v "$tool" &> /dev/null; then
        echo "  $tool found"
        return 0
    fi
    echo "  $tool is missing: $why"
    if [[ "$OS" != Darwin ]]; then
        echo "    Install it with your package manager (the report at the end has the command)."
        return 1
    fi
    if ! command -v brew &> /dev/null; then
        echo "    Install Homebrew (https://brew.sh), then: brew install $tool"
        return 1
    fi
    if ask "    Install $tool now with 'brew install $tool'? [y/N] "; then
        if brew install "$tool"; then
            echo "  $tool installed"
            return 0
        fi
        echo "    brew install $tool failed. Install it yourself, then re-run this script."
    else
        echo "    Not installed. Later: brew install $tool"
    fi
    return 1
}

echo "Checking prerequisites..."
if ! offer go "builds clauductor from this checkout"; then
    echo ""
    echo "Go is required to build clauductor from source: https://go.dev/dl/"
    exit 1
fi
offer jq "the operating model's hooks, checks and context scripts read JSON with it" || true
offer gh "merge-pr and the PR flow call the GitHub CLI" || true
offer gitleaks "the gate's secret scan (skipped locally without it, a failure under CI)" || true
if [[ "$OS" == Darwin ]]; then
    offer tmux "the panel's lanes" || true
fi
if ! command -v claude &> /dev/null; then
    echo "  Claude Code is missing: install it from https://claude.ai/code"
fi
echo ""

# --- Build ---

echo "Building clauductor..."
cd "$FRAMEWORK_DIR/framework"
go build -o clauductor ./cmd/clauductor
echo "  Build successful."
echo ""

# --- Install ---

echo "Installing to $INSTALL_DIR..."
mkdir -p "$INSTALL_DIR"
cp clauductor "$INSTALL_DIR/clauductor"
chmod +x "$INSTALL_DIR/clauductor"

# Clean up build artifact
rm -f clauductor

# Check if INSTALL_DIR is in PATH
if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
    echo ""
    echo "  Warning: $INSTALL_DIR is not in your PATH."
    echo "  Add this to your shell profile (~/.zshrc or ~/.bashrc):"
    echo ""
    echo "    export PATH=\"$INSTALL_DIR:\$PATH\""
    echo ""
fi

# Set framework location for template lookups
echo ""
echo "Setting CLAUDUCTOR_FRAMEWORK environment variable..."
SHELL_RC="$HOME/.zshrc"
if [[ -f "$HOME/.bashrc" ]] && [[ ! -f "$HOME/.zshrc" ]]; then
    SHELL_RC="$HOME/.bashrc"
fi

if ! grep -q "CLAUDUCTOR_FRAMEWORK" "$SHELL_RC" 2>/dev/null; then
    echo "" >> "$SHELL_RC"
    echo "# Clauductor framework location" >> "$SHELL_RC"
    echo "export CLAUDUCTOR_FRAMEWORK=\"$FRAMEWORK_DIR\"" >> "$SHELL_RC"
    echo "  Added to $SHELL_RC"
else
    echo "  Already set in $SHELL_RC"
fi

# Export for current session
export CLAUDUCTOR_FRAMEWORK="$FRAMEWORK_DIR"

echo ""
# The same report `clauductor install` prints in a project: every tool, with this OS's install hint.
sh "$FRAMEWORK_DIR/template/.claude/prereqs.sh" || true

echo ""
echo "==================================="
echo "  Installation complete!"
echo "==================================="
echo ""
echo "  Binary:    $INSTALL_DIR/clauductor"
echo "  Framework: $FRAMEWORK_DIR"
echo "  Version:   $(\"$INSTALL_DIR/clauductor\" version 2>/dev/null || echo 'unknown')"
echo ""
echo "Usage:"
echo "  clauductor init ~/Development/my-app    # New project"
echo "  cd existing-project && clauductor install  # Existing repo"
echo "  clauductor panel                         # The local panel (docs/panel.md)"
echo ""
echo "Note: restart your shell or run 'source $SHELL_RC' to update PATH."
