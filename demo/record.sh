#!/usr/bin/env bash
# Re-records the README clips against the fake API in demo/server.
# Needs vhs (brew install vhs). Uses a stand-in pbcopy/open so recording
# never touches your clipboard or browser.
set -euo pipefail
cd "$(dirname "$0")"

work="$(mktemp -d)"
trap 'kill "${server:-}" 2>/dev/null || true; rm -rf "$work"' EXIT

mkdir -p "$work/bin" ../assets/demo
go build -o "$work/bin/beepertui" ..
go build -o "$work/server" ./server
printf '#!/bin/sh\ncat >/dev/null\n' >"$work/bin/pbcopy"
printf '#!/bin/sh\nexit 0\n' >"$work/bin/open"
chmod +x "$work/bin/pbcopy" "$work/bin/open"

image="${DEMO_IMAGE:-}"
if [ -z "$image" ] && command -v sips >/dev/null; then
  pic="$(ls /System/Library/Desktop\ Pictures/*.heic 2>/dev/null | head -1 || true)"
  [ -n "$pic" ] && sips -s format png -Z 900 "$pic" --out "$work/image.png" >/dev/null && image="$work/image.png"
fi

for tape in "${@:-tour flash switch compose media more search}"; do
  for t in $tape; do
    # fresh fake data for every clip
    "$work/server" -image "$image" >/dev/null 2>&1 &
    server=$!
    sleep 0.5
    echo "recording $t"
    PATH="$work/bin:$PATH" vhs "tapes/$t.tape"
    kill "$server"
    wait "$server" 2>/dev/null || true
  done
done
