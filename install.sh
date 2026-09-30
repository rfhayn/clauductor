#!/bin/bash
# Install the clauductor binary (docs/install.md).
#
#   ./install.sh                  in a clone: build from source (installs Go and tmux with
#                                 Homebrew if missing); without Go, a release binary first
#   ./install.sh --release        download the latest release binary instead of building
#   ./install.sh --version v0.1.0 a given release (implies --release)
#   ./install.sh --source         always build from this clone
#   curl -fsSL https://raw.githubusercontent.com/rfhayn/clauductor/main/install.sh | bash
#                                 no clone: download the latest release
#
# A release archive holds the binary and template/ (what `clauductor init`, `install` and
# `update` copy from). It is unpacked to ~/.local/share/clauductor/<version>, with `current`
# pointing at it, and CLAUDUCTOR_FRAMEWORK names `current`. A source build names the clone.
#
# Environment: INSTALL_DIR (default ~/.local/bin), CLAUDUCTOR_SHARE (default
# ~/.local/share/clauductor), CLAUDUCTOR_REPO (default rfhayn/clauductor), CLAUDUCTOR_RELEASE_BASE
# (a URL or file:// directory holding the archives and checksums.txt, instead of GitHub).
set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
SHARE_DIR="${CLAUDUCTOR_SHARE:-$HOME/.local/share/clauductor}"
REPO="${CLAUDUCTOR_REPO:-rfhayn/clauductor}"
MODE=auto
VERSION=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --source) MODE=source ;;
        --release) MODE=release ;;
        --version)
            [[ $# -ge 2 ]] || { echo "--version needs a tag, such as v0.1.0" >&2; exit 2; }
            VERSION="$2"; MODE=release; shift ;;
        --version=*) VERSION="${1#--version=}"; MODE=release ;;
        -h|--help) awk 'NR > 1 && /^#/ { sub(/^# ?/, ""); print; next } NR > 1 { exit }' "${BASH_SOURCE[0]:-$0}"; exit 0 ;;
        *) echo "unknown option: $1 (try --help)" >&2; exit 2 ;;
    esac
    shift
done
if [[ -n "$VERSION" && ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.+-]+)?$ ]]; then
    echo "--version takes a release tag such as v0.1.0, not '$VERSION'" >&2
    exit 2
fi

# Piped through bash there is no script file, so there is no clone to build from.
SCRIPT_DIR=""
if [[ -n "${BASH_SOURCE[0]:-}" && -f "${BASH_SOURCE[0]}" ]]; then
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
fi
IN_CLONE=0
[[ -n "$SCRIPT_DIR" && -f "$SCRIPT_DIR/framework/go.mod" && -d "$SCRIPT_DIR/template" ]] && IN_CLONE=1

OS="$(uname -s)"
echo "==================================="
echo "  Clauductor Installer"
echo "==================================="
echo ""

# --- Prerequisites ---

have() { command -v "$1" &> /dev/null; }

check_brew() {
    if ! have brew; then
        echo "Homebrew is required but not found."
        read -r -p "Install Homebrew? [y/N] " response
        if [[ "$response" != "y" && "$response" != "Y" ]]; then
            echo "Please install Homebrew manually: https://brew.sh"
            exit 1
        fi
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
    if ! have go; then
        echo "Go not found. Installing via Homebrew..."
        check_brew
        brew install go
        export PATH="$(go env GOPATH)/bin:$PATH"
        echo "  Go installed: $(go version)"
    else
        echo "  Go found: $(go version)"
    fi
}

# tmux runs the panel's lanes. On macOS it is installed with Homebrew, as before; elsewhere, or
# when this script has no terminal to ask on (piped through bash), it only says how.
check_tmux() {
    if have tmux; then
        echo "  tmux found: $(tmux -V)"
    elif [[ "$OS" == Darwin ]] && { have brew || [[ -t 0 ]]; }; then
        echo "tmux not found. Installing via Homebrew..."
        check_brew
        brew install tmux
        echo "  tmux installed: $(tmux -V)"
    else
        echo "  Warning: tmux not found; the panel's lanes need it (apt install tmux, dnf install tmux, brew install tmux)."
    fi
}

check_optional() {
    local tool="$1" why="$2"
    if have "$tool"; then
        echo "  $tool found"
    else
        echo "  Warning: $tool not found; $why"
    fi
}

check_claude() {
    if have claude; then
        echo "  Claude Code found: $(claude --version 2>/dev/null || echo 'version unknown')"
    else
        echo "  Warning: Claude Code not found. Install from https://claude.ai/code"
    fi
}

# --- From a release ---

sha256_of() {
    if have sha256sum; then sha256sum "$1" | awk '{print $1}'; else shasum -a 256 "$1" | awk '{print $1}'; fi
}

# Returns non-zero, having said why, when there is no release to install; the caller decides
# whether to build instead.
install_release() {
    local goos goarch asset base tmp want got dir ver
    case "$OS" in
        Darwin) goos=darwin ;;
        Linux) goos=linux ;;
        *) echo "  No release binary for $OS. On Windows, run this inside WSL." >&2; return 1 ;;
    esac
    case "$(uname -m)" in
        arm64|aarch64) goarch=arm64 ;;
        x86_64|amd64) goarch=amd64 ;;
        *) echo "  No release binary for $(uname -m)." >&2; return 1 ;;
    esac
    have curl || { echo "  curl is needed to download a release." >&2; return 1; }
    asset="clauductor-$goos-$goarch.tar.gz"
    if [[ -n "${CLAUDUCTOR_RELEASE_BASE:-}" ]]; then
        base="$CLAUDUCTOR_RELEASE_BASE" # a mirror, or a file:// directory of built archives
    elif [[ -n "$VERSION" ]]; then
        base="https://github.com/$REPO/releases/download/$VERSION"
    else
        base="https://github.com/$REPO/releases/latest/download"
    fi

    tmp="$(mktemp -d "${TMPDIR:-/tmp}/clauductor-install.XXXXXX")"
    # shellcheck disable=SC2064 # expand now: tmp is local
    trap "rm -rf '$tmp'" RETURN
    echo "Downloading $asset from $base ..."
    if ! curl -fsSL -o "$tmp/$asset" "$base/$asset" || ! curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt"; then
        echo "  No release download found at $base." >&2
        return 1
    fi
    want="$(awk -v f="$asset" '$2 == f || $2 == "*" f {print $1}' "$tmp/checksums.txt")"
    got="$(sha256_of "$tmp/$asset")"
    if [[ -z "$want" || "$want" != "$got" ]]; then
        echo "  Checksum mismatch for $asset (want ${want:-none}, got $got). Not installing." >&2
        exit 1
    fi
    echo "  Checksum verified."

    tar -C "$tmp" -xzf "$tmp/$asset"
    dir="$tmp/clauductor-$goos-$goarch"
    [[ -x "$dir/clauductor" && -d "$dir/template" ]] || { echo "  $asset is not a clauductor release archive." >&2; exit 1; }
    ver="$("$dir/clauductor" version | awk '{print $2}')"
    [[ "$ver" =~ ^v[0-9A-Za-z.+-]+$ ]] || { echo "  Unexpected version output: $ver" >&2; exit 1; }

    mkdir -p "$SHARE_DIR"
    rm -rf "${SHARE_DIR:?}/$ver"
    mv "$dir" "$SHARE_DIR/$ver"
    ln -sfn "$ver" "$SHARE_DIR/current"
    mkdir -p "$INSTALL_DIR"
    cp "$SHARE_DIR/$ver/clauductor" "$INSTALL_DIR/clauductor.new"
    chmod +x "$INSTALL_DIR/clauductor.new"
    mv "$INSTALL_DIR/clauductor.new" "$INSTALL_DIR/clauductor"
    FRAMEWORK_DIR="$SHARE_DIR/current"
    echo "  Installed clauductor $ver (template in $SHARE_DIR/$ver/template)."
}

# --- From source ---

install_source() {
    check_go
    echo ""
    echo "Building clauductor..."
    (cd "$SCRIPT_DIR/framework" && go build -o clauductor ./cmd/clauductor)
    echo "  Build successful."
    echo ""
    echo "Installing to $INSTALL_DIR..."
    mkdir -p "$INSTALL_DIR"
    cp "$SCRIPT_DIR/framework/clauductor" "$INSTALL_DIR/clauductor"
    chmod +x "$INSTALL_DIR/clauductor"
    rm -f "$SCRIPT_DIR/framework/clauductor"
    FRAMEWORK_DIR="$SCRIPT_DIR"
}

echo "Checking prerequisites..."
check_tmux
check_optional git "clauductor works in git repositories."
check_optional gh "pull requests, merge readiness and the Metrics view use it (https://cli.github.com)."
check_optional jq "the operating model's hooks and checks read JSON with it."
check_claude
echo ""

FRAMEWORK_DIR=""
case "$MODE" in
    source)
        [[ "$IN_CLONE" == 1 ]] || { echo "--source needs a clone: git clone https://github.com/$REPO.git" >&2; exit 1; }
        install_source ;;
    release)
        install_release || exit 1 ;;
    auto)
        if [[ "$IN_CLONE" == 1 ]] && have go; then
            install_source
        elif install_release; then
            :
        elif [[ "$IN_CLONE" == 1 ]]; then
            echo "Building from source instead."
            install_source
        else
            echo "" >&2
            echo "Build from source instead:" >&2
            echo "  git clone https://github.com/$REPO.git ~/clauductor && ~/clauductor/install.sh" >&2
            exit 1
        fi ;;
esac

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
    {
        echo ""
        echo "# Clauductor framework location"
        echo "export CLAUDUCTOR_FRAMEWORK=\"$FRAMEWORK_DIR\""
    } >> "$SHELL_RC"
    echo "  Added to $SHELL_RC"
elif grep -qF "export CLAUDUCTOR_FRAMEWORK=\"$FRAMEWORK_DIR\"" "$SHELL_RC"; then
    echo "  Already set in $SHELL_RC"
else
    # Never rewrite a line the user may have edited: say what it should be instead.
    echo "  Warning: $SHELL_RC sets CLAUDUCTOR_FRAMEWORK to something else. For this install it should be:"
    echo "    export CLAUDUCTOR_FRAMEWORK=\"$FRAMEWORK_DIR\""
fi

export CLAUDUCTOR_FRAMEWORK="$FRAMEWORK_DIR"

echo ""
echo "==================================="
echo "  Installation complete!"
echo "==================================="
echo ""
echo "  Binary:    $INSTALL_DIR/clauductor"
echo "  Framework: $FRAMEWORK_DIR"
echo "  Version:   $("$INSTALL_DIR/clauductor" version 2>/dev/null || echo 'unknown')"
echo ""
echo "Usage:"
echo "  clauductor panel init && clauductor panel trust   # the panel, in any repository"
echo "  clauductor panel                                 # open it"
echo "  clauductor init ~/Development/my-app             # a new project with the operating model"
echo "  cd existing-project && clauductor install        # add the operating model to a repository"
echo ""
echo "Note: restart your shell or run 'source $SHELL_RC' to update PATH."
