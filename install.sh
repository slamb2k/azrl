#!/usr/bin/env bash
set -euo pipefail

# Build and install the azrl binary, gitignore .azprofile, and bootstrap config.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"
mkdir -p "$BIN_DIR"

echo "azrl: building..."
( cd "$ROOT" && go build -o "$BIN_DIR/azrl" . )
echo "azrl: installed $BIN_DIR/azrl"

# Globally gitignore .azprofile so it is never committed.
GI="${XDG_CONFIG_HOME:-$HOME/.config}/git/ignore"
mkdir -p "$(dirname "$GI")"
grep -qxF '.azprofile' "$GI" 2>/dev/null || echo '.azprofile' >> "$GI"

# Bootstrap the global config from the example if absent.
PROFILES="$HOME/.azure-profiles"
mkdir -p "$PROFILES"
if [[ ! -f "$PROFILES/azrl.conf" && -f "$ROOT/azrl.conf.example" ]]; then
  cp "$ROOT/azrl.conf.example" "$PROFILES/azrl.conf"
  echo "azrl: wrote $PROFILES/azrl.conf (edit LOCAL_HOST/LOCAL_BROWSER_CMD/VM_HOST)"
fi

echo "azrl: done. Ensure $BIN_DIR is on your PATH."
