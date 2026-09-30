#!/bin/sh
# Rebuild plugin/ and .claude-plugin/marketplace.json from template/ (docs/plugin.md).
# Run it after any change under template/: TestCommittedPluginIsCurrent fails until you do.
set -e
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root/framework"
exec go run ./cmd/clauductor plugin build --repo "$root"
