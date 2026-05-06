#!/bin/bash
# Test single agent auto-selection

set -e

TMPDIR_TEST=$(mktemp -d)
trap "rm -rf $TMPDIR_TEST" EXIT

echo "Testing single agent auto-selection (claude only in PATH)..."
echo "Temp dir: $TMPDIR_TEST"

# Create minimal PATH with only claude
export HOME="$TMPDIR_TEST"
export XDG_CONFIG_HOME="$TMPDIR_TEST/.config"
export PATH="/Users/gcarthew/.local/bin:/usr/bin:/bin"

mkdir -p "$XDG_CONFIG_HOME"

./bin/start 2>&1 || true

echo ""
echo "Config files created:"
ls -la "$XDG_CONFIG_HOME/start/" 2>/dev/null || echo "No config created"

if [ -f "$XDG_CONFIG_HOME/start/agents.cue" ]; then
    echo ""
    echo "agents.cue contents:"
    cat "$XDG_CONFIG_HOME/start/agents.cue"
fi

if [ -f "$XDG_CONFIG_HOME/start/config.cue" ]; then
    echo ""
    echo "config.cue contents:"
    cat "$XDG_CONFIG_HOME/start/config.cue"
fi
