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

# Bootstrap the global config if absent. Environment detection lives once in Go
# (internal/envdetect); `azrl setup --yes` writes the recommended azrl.conf
# non-interactively. Re-run `azrl setup` any time to review or change it.
PROFILES="$HOME/.azure-profiles"
mkdir -p "$PROFILES"
if [[ ! -f "$PROFILES/azrl.conf" ]]; then
  "$BIN_DIR/azrl" setup --yes || echo "azrl: could not seed config; run 'azrl setup' after install" >&2
  echo "azrl: run 'azrl setup' to review or change the detected config"
fi

echo "azrl: done. Ensure $BIN_DIR is on your PATH."
