#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
FRAMEWORK_DIR="$SCRIPT_DIR"

echo "==================================="
echo "  Clauductor Framework Installer"
echo "==================================="
echo ""

# --- Check / Install Prerequisites ---

check_brew() {
    if ! command -v brew &> /dev/null; then
        echo "Homebrew not found. Installing..."
        /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
        # Source brew for current session
        if [[ -f /opt/homebrew/bin/brew ]]; then
            eval "$(/opt/homebrew/bin/brew shellenv)"
        elif [[ -f /usr/local/bin/brew ]]; then
            eval "$(/usr/local/bin/brew shellenv)"
        fi
    fi
}

check_go() {
    if ! command -v go &> /dev/null; then
        echo "Go not found. Installing via Homebrew..."
        check_brew
        brew install go
        # Refresh PATH
        export PATH="$(go env GOPATH)/bin:$PATH"
        echo "  Go installed: $(go version)"
    else
        echo "  Go found: $(go version)"
    fi
}

check_tmux() {
    if ! command -v tmux &> /dev/null; then
        echo "tmux not found. Installing via Homebrew..."
        check_brew
        brew install tmux
        echo "  tmux installed: $(tmux -V)"
    else
        echo "  tmux found: $(tmux -V)"
    fi
}

check_claude() {
    if command -v claude &> /dev/null; then
        echo "  Claude Code found: $(claude --version 2>/dev/null || echo 'version unknown')"
    else
        echo "  Warning: Claude Code not found. Install from https://claude.ai/code"
    fi
}

echo "Checking prerequisites..."
check_go
check_tmux
check_claude
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
echo "  clauductor start                         # Launch orchestrator"
echo ""
echo "Note: restart your shell or run 'source $SHELL_RC' to update PATH."
