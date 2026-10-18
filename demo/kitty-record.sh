#!/usr/bin/env bash
# Records the README clips in a real kitty window, so images and GIFs show at
# full quality (VHS renders in xterm.js, which can't draw kitty graphics).
# macOS only: it screen-records the window's area, so keep it visible and
# don't type into it while it runs.
#
#   demo/kitty-record.sh            # every scene
#   demo/kitty-record.sh media      # just one
set -euo pipefail
cd "$(dirname "$0")"

KITTY=/Applications/kitty.app/Contents/MacOS/kitty
KITTEN=/Applications/kitty.app/Contents/MacOS/kitten
OUT=../assets/demo
work="$(mktemp -d)"
sock="${TMPDIR:-/tmp}/bt-rec.sock"
trap 'kill "${rec:-}" 2>/dev/null || true; pkill -f "kitty --title beepertui-rec" 2>/dev/null || true; kill "${server:-}" 2>/dev/null || true; rm -rf "$work" "$sock"' EXIT

mkdir -p "$work/bin" "$OUT"
go build -o "$work/bin/beepertui" ..
go build -o "$work/server" ./server
printf '#!/bin/sh\ncat >/dev/null\n' >"$work/bin/pbcopy"
printf '#!/bin/sh\nexit 0\n' >"$work/bin/open"
chmod +x "$work/bin/pbcopy" "$work/bin/open"
# where the kitty window is: screen index (ffmpeg's "Capture screen N") and pixel rect
cat >"$work/screen.swift" <<'EOF'
import CoreGraphics
var count: UInt32 = 0
CGGetActiveDisplayList(0, nil, &count)
var ids = [CGDirectDisplayID](repeating: 0, count: Int(count))
CGGetActiveDisplayList(count, &ids, &count)
let list = CGWindowListCopyWindowInfo([.optionOnScreenOnly], kCGNullWindowID) as! [[String: Any]]
for w in list {
  guard let n = w["kCGWindowName"] as? String, n.contains("beepertui-rec"), let b = w["kCGWindowBounds"] as? [String: Any],
        let x = b["X"] as? Double, let y = b["Y"] as? Double, let ww = b["Width"] as? Double, let hh = b["Height"] as? Double else { continue }
  for (i, d) in ids.enumerated() {
    let db = CGDisplayBounds(d)
    if db.contains(CGPoint(x: x + 1, y: y + 1)) {
      let s = Double(CGDisplayCopyDisplayMode(d)?.pixelWidth ?? Int(db.width)) / Double(db.width)
      print(i, Int((x - db.minX) * s), Int((y - db.minY) * s), Int(ww * s) / 2 * 2, Int(hh * s) / 2 * 2)
    }
  }
}
EOF

image="${DEMO_IMAGE:-}"
if [ -z "$image" ]; then
  pic="$(ls /System/Library/Desktop\ Pictures/*.heic 2>/dev/null | head -1 || true)"
  [ -n "$pic" ] && sips -s format png -Z 900 "$pic" --out "$work/image.png" >/dev/null && image="$work/image.png"
fi

# helpers the scenes use
key() { for k in "$@"; do "$KITTEN" @ --to "unix:$sock" send-key "$k"; sleep 0.35; done; }
keys() { local n=$1; shift; for _ in $(seq "$n"); do key "$@"; done; }
say() { "$KITTEN" @ --to "unix:$sock" send-text -- "$1"; sleep 0.3; }
pause() { sleep "$1"; }

start_scene() {
  "$work/server" -image "$image" >/dev/null 2>&1 &
  server=$!
  rm -f "$sock"
  "$KITTY" --title beepertui-rec -o allow_remote_control=yes --listen-on "unix:$sock" \
    -o initial_window_width=150c -o initial_window_height=44c -o remember_window_size=no \
    -o confirm_os_window_close=0 -o font_size="${DEMO_FONT_SIZE:-16}" -o hide_window_decorations=titlebar-only \
    -o window_padding_width=12 \
    sh -c "PATH=$work/bin:\$PATH XDG_CONFIG_HOME=$work/xdg BEEPERTUI_NERDFONT=1 BEEPER_URL=http://127.0.0.1:23999 BEEPER_ACCESS_TOKEN=demo beepertui" \
    >/dev/null 2>&1 &
  sleep 3.5
  "$@" # setup keys before recording starts
  sleep 2
  read -r idx x y w h <<<"$(swift "$work/screen.swift" 2>/dev/null | head -1)"
  ffmpeg -v error -y -f avfoundation -pixel_format bgr0 -framerate 30 -capture_cursor 0 -i "Capture screen ${idx}:none" \
    -vf "crop=$w:$h:$x:$y" -c:v libx264 -preset ultrafast -crf 16 "$work/$scene.mkv" </dev/null &
  rec=$!
  sleep 1.5
}

end_scene() {
  sleep 1
  kill -INT "$rec" 2>/dev/null || true # ffmpeg finishes the file on SIGINT
  wait "$rec" 2>/dev/null || true
  "$KITTEN" @ --to "unix:$sock" send-key q 2>/dev/null || true
  sleep 0.5
  pkill -f "kitty --title beepertui-rec" 2>/dev/null || true
  kill "$server" 2>/dev/null || true
  wait "$server" 2>/dev/null || true
  # mp4 as recorded, and a gif with its own palette so pictures keep their colours
  ffmpeg -v error -y -i "$work/$scene.mkv" -vf "fps=30,scale=1600:-2:flags=lanczos" -c:v libx264 -crf 22 -pix_fmt yuv420p -movflags +faststart "$OUT/$scene.mp4"
  ffmpeg -v error -y -i "$work/$scene.mkv" -vf "fps=12,scale=1100:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=192:stats_mode=diff[p];[b][p]paletteuse=dither=sierra2_4a:diff_mode=rectangle" "$OUT/$scene.gif"
  echo "  $(du -h "$OUT/$scene.gif" | cut -f1) gif, $(du -h "$OUT/$scene.mp4" | cut -f1) mp4"
}

nothing() { :; }
open_design() { key enter; sleep 1.5; }
open_hike() { say J; sleep 1.5; }

scene_tour() {
  start_scene nothing
  pause 1; keys 3 down; keys 3 up
  key enter; pause 3            # sender colours, rules, link card, seen-by
  keys 2 up; pause 3            # the gif plays, who reacted
  keys 2 up; pause 1.5
  keys 4 down; pause 1
  say "]"; pause 1; say "["; pause 0.6
  say "?"; pause 3.5; key escape
  end_scene
}

scene_flash() {
  start_scene open_design
  say "r"; pause 1.6; say "a"; pause 0.4
  say "on it, looks great"; key enter; pause 1.2; key escape; pause 0.5
  say "R"; pause 1.4; say "d"; pause 0.8
  say "fire"; pause 1; key enter; pause 1.2
  say "yy"; pause 1; say "s"; pause 1.8; key escape
  end_scene
}

scene_switch() {
  start_scene open_design
  say "cc"; pause 1.2; say "hke"; pause 1; key enter; pause 1.8
  say "J"; pause 1.2; say "J"; pause 1.2; say "K"; pause 1
  say "c"; pause 1.6; say "f"; pause 1.6
  say "CC"; pause 1.8; say "p"; pause 1.5
  end_scene
}

scene_compose() {
  start_scene open_hike
  say "i"; say "morning @om"; pause 1.2; key tab
  say "still on for 7?"; key ctrl+j
  say "I'll bring coffee :coff"; pause 1.2; key tab; pause 1
  key enter; pause 1.4; key escape; pause 0.5
  say "K"; pause 1.5; keys 5 up; pause 0.6
  say "f"; pause 1.6; say "a"; pause 1.4; say "y"; pause 1.2
  keys 4 down; key enter; pause 4.5; key escape
  end_scene
}

scene_media() {
  start_scene open_hike
  say ":media cat"; key enter; pause 7
  keys 3 down; pause 1.5
  key tab tab; key ctrl+u; say "panic fire"; pause 3
  key enter; pause 1; say "the build at 5pm / this is fine"; pause 4
  key enter; pause 2.5
  key ctrl+g; pause 0.8; key tab; key ctrl+u; say "wave"; pause 5
  key down; pause 3.5; key enter; pause 2.5; key escape
  end_scene
}

scene_more() {
  start_scene open_hike
  say ":emoji skull laugh"; key enter; pause 5; key down; pause 2
  key tab; key ctrl+u; pause 5
  key tab; key ctrl+u; say "compiling"; pause 5
  key tab; key ctrl+u; say "418"; pause 4; key down; pause 2
  key tab; key ctrl+u; say "ProgrammerHumor"; pause 7; key escape
  end_scene
}

scene_search() {
  start_scene open_design
  say "/"; pause 0.5; say "gradient"; key enter; pause 1.5; key enter; pause 2
  keys 3 up; pause 1.5
  say ":"; pause 2; say "re"; pause 2; keys 2 down; pause 1; key escape
  end_scene
}

for scene in ${@:-tour flash switch compose media more search}; do
  echo "recording $scene"
  "scene_$scene"
done
