#!/usr/bin/env bash
set -euo pipefail
SRC="$(dirname "$(readlink -f "$0")")"
BIN="$HOME/.local/bin"
mkdir -p "$BIN"
ln -sf "$SRC/azlogin"         "$BIN/azlogin"
ln -sf "$SRC/azlogin-capture" "$BIN/azlogin-capture"
echo "linked azlogin + azlogin-capture into $BIN"

# Ensure .azprofile is globally ignored
IGN="${XDG_CONFIG_HOME:-$HOME/.config}/git/ignore"
mkdir -p "$(dirname "$IGN")"
grep -qxF '.azprofile' "$IGN" 2>/dev/null || echo '.azprofile' >> "$IGN"
echo "ensured .azprofile in $IGN"

# Bootstrap global config if absent
[[ -f "$HOME/.azure-profiles/azlogin.conf" ]] || {
  cp "$SRC/azlogin.conf.example" "$HOME/.azure-profiles/azlogin.conf"
  echo "created ~/.azure-profiles/azlogin.conf (review values)"
}
