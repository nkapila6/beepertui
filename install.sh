#!/bin/sh
# curl -fsSL https://raw.githubusercontent.com/nkapila6/beepertui/main/install.sh | sh
set -eu

repo="nkapila6/beepertui"
bin_dir="${BIN_DIR:-$HOME/.local/bin}"

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) echo "unsupported OS: $(uname -s). On Windows grab the zip from https://github.com/$repo/releases" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) echo "unsupported arch: $(uname -m)" >&2; exit 1 ;;
esac

if [ -n "${VERSION:-}" ]; then
  base="https://github.com/$repo/releases/download/v${VERSION#v}"
else
  base="https://github.com/$repo/releases/latest/download"
fi
archive="beepertui_${os}_${arch}.tar.gz"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "downloading $archive"
curl -fsSL "$base/$archive" -o "$tmp/$archive"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"

want="$(grep " $archive\$" "$tmp/checksums.txt" | cut -d' ' -f1)"
if command -v shasum >/dev/null 2>&1; then
  got="$(shasum -a 256 "$tmp/$archive" | cut -d' ' -f1)"
else
  got="$(sha256sum "$tmp/$archive" | cut -d' ' -f1)"
fi
if [ -z "$want" ] || [ "$want" != "$got" ]; then
  echo "checksum mismatch for $archive" >&2
  exit 1
fi

tar -xzf "$tmp/$archive" -C "$tmp"
mkdir -p "$bin_dir"
install -m 755 "$tmp/beepertui" "$bin_dir/beepertui"
echo "installed $("$bin_dir/beepertui" --version) to $bin_dir"

case ":$PATH:" in
  *":$bin_dir:"*) ;;
  *) echo "note: $bin_dir isn't on your PATH" ;;
esac
