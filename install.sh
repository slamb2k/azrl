#!/usr/bin/env bash
set -euo pipefail
SRC="$(dirname "$(readlink -f "$0")")"
BIN="$HOME/.local/bin"
mkdir -p "$BIN"
ln -sf "$SRC/azrl"         "$BIN/azrl"
ln -sf "$SRC/azrl-capture" "$BIN/azrl-capture"
echo "linked azrl + azrl-capture into $BIN"

# Ensure .azprofile is globally ignored
IGN="${XDG_CONFIG_HOME:-$HOME/.config}/git/ignore"
mkdir -p "$(dirname "$IGN")"
grep -qxF '.azprofile' "$IGN" 2>/dev/null || echo '.azprofile' >> "$IGN"
echo "ensured .azprofile in $IGN"

# Bootstrap global config if absent
[[ -f "$HOME/.azure-profiles/azrl.conf" ]] || {
  cp "$SRC/azrl.conf.example" "$HOME/.azure-profiles/azrl.conf"
  echo "created ~/.azure-profiles/azrl.conf (review values)"
}
