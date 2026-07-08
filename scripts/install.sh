#!/usr/bin/env sh
# azrl installer — downloads the right prebuilt binary from the latest GitHub
# release and installs it. Usage:
#   curl -fsSL https://raw.githubusercontent.com/slamb2k/azrl/main/scripts/install.sh | sh
# Override the install dir with BINDIR, e.g.  BINDIR=/usr/local/bin sh install.sh
set -eu

REPO="slamb2k/azrl"
BIN="azrl"

err() { echo "azrl-install: $*" >&2; exit 1; }

# --- detect os/arch, mapped to GoReleaser's names ---
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux) os=linux ;;
  darwin) os=darwin ;;
  *) err "unsupported OS '$os' — grab a binary from https://github.com/$REPO/releases" ;;
esac
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) err "unsupported arch '$arch' — grab a binary from https://github.com/$REPO/releases" ;;
esac

# --- resolve latest release tag ---
tag=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
  | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)
[ -n "$tag" ] || err "could not determine the latest release — is one published yet?"
version=${tag#v}

# --- download + extract ---
archive="${BIN}_${version}_${os}_${arch}.tar.gz"
url="https://github.com/$REPO/releases/download/$tag/$archive"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
echo "azrl-install: downloading $archive ($tag)"
curl -fsSL "$url" -o "$tmp/$archive" || err "download failed: $url"
tar -xzf "$tmp/$archive" -C "$tmp" || err "extract failed"
[ -f "$tmp/$BIN" ] || err "archive did not contain '$BIN'"

# --- choose an install dir ---
if [ -n "${BINDIR:-}" ]; then
  bindir=$BINDIR
elif [ -w /usr/local/bin ] 2>/dev/null; then
  bindir=/usr/local/bin
else
  bindir="$HOME/.local/bin"
fi
mkdir -p "$bindir"
install -m 0755 "$tmp/$BIN" "$bindir/$BIN" 2>/dev/null || { cp "$tmp/$BIN" "$bindir/$BIN" && chmod 0755 "$bindir/$BIN"; }

echo "azrl-install: installed $bindir/$BIN ($tag)"

# --- bootstrap the global config if absent ---
# azrl refuses to run without ~/.azure-profiles/azrl.conf. Environment detection
# lives once in Go (internal/envdetect); `azrl setup --yes` writes the
# recommended config non-interactively. Re-run `azrl setup` to review or change it.
profiles="$HOME/.azure-profiles"
mkdir -p "$profiles"
if [ ! -f "$profiles/azrl.conf" ]; then
  "$bindir/$BIN" setup --yes || echo "azrl-install: could not seed config; run 'azrl setup' after install" >&2
  echo "azrl-install: run 'azrl setup' to review or change the detected config"
fi

# --- globally gitignore the per-directory pointer files so they are never committed ---
gi="${XDG_CONFIG_HOME:-$HOME/.config}/git/ignore"
mkdir -p "$(dirname "$gi")"
for p in .azprofile .ghprofile .awsprofile .gcpprofile; do
  grep -qxF "$p" "$gi" 2>/dev/null || echo "$p" >> "$gi"
done

case ":$PATH:" in
  *":$bindir:"*) : ;;
  *) echo "azrl-install: note — $bindir is not on your PATH; add it or move the binary." ;;
esac
"$bindir/$BIN" --version 2>/dev/null || true
