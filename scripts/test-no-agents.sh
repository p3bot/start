#!/bin/bash
# Test no agents detected scenario

set -e

TMPDIR_TEST=$(mktemp -d)
trap "rm -rf $TMPDIR_TEST 2>/dev/null || true" EXIT

echo "Testing no agents detected (restricted PATH)..."
echo "Temp dir: $TMPDIR_TEST"

# Create minimal PATH with no AI tools
export HOME="$TMPDIR_TEST"
export XDG_CONFIG_HOME="$TMPDIR_TEST/.config"
export PATH="/usr/bin:/bin"

mkdir -p "$XDG_CONFIG_HOME"

./bin/start 2>&1 || true
