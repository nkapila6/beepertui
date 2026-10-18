# beepertui

A terminal client for Beeper, talking to the Beeper Desktop app on your machine. Vim-ish keys, and flash.nvim-style labels for picking a chat or message to jump to, yank, reply to, react to, edit or delete.

Works with [tuios](https://github.com/Gaurav-Gosain/tuios) themes and looks: the colours come from your terminal palette, so a tuios look restyles it.

Beeper Desktop does the real work (bridges, encryption). This is just a frontend for its local API on `localhost:23373`.

## Install

```bash
brew install nkapila6/tap/beepertui
```

or

```bash
curl -fsSL https://raw.githubusercontent.com/nkapila6/beepertui/main/install.sh | sh
```

The script drops the binary in `~/.local/bin` (set `BIN_DIR` to change that, `VERSION=0.0.1` to pin). On Windows, grab the zip from [releases](https://github.com/nkapila6/beepertui/releases). With Go installed, `go install github.com/nkapila6/beepertui@latest` works too.

macOS gets everything. Linux and Windows work too, minus `C-v` image paste (use `:attach`). Yank on Linux needs `wl-copy`, `xclip` or `xsel`, otherwise it falls back to OSC52.

## Setup

1. In Beeper Desktop: Settings > Developers, turn on the Beeper Desktop API and create an access token.
2. Run it:

```bash
export BEEPER_ACCESS_TOKEN=...
beepertui
```

`BEEPER_URL` overrides the API address if you've moved it.

## Keys

| key | does |
| --- | --- |
| `j` `k` `gg` `G` `C-d` `C-u` | move |
| `h` `l` `tab` | switch between chats and messages |
| `J` `K` | next / previous chat, from either pane |
| `enter` | open chat / start writing |
| `i` `a` | write. `enter` sends, `esc` leaves, `C-v` pastes an image, `C-c` clears, `@` mentions someone |
| `s` | flash jump anywhere: labels every chat and message on screen at once |
| `c` | flash just the chats and switch to one, from either pane |
| `C` | flash a chat, then a one-key menu: open, pin, mute, archive, low priority, read/unread, remind, info. `CC` = current chat |
| `y` `r` `R` `e` `x` `o` `U` | flash pick, then yank / reply / react / edit / delete / open / unreact |
| `v` `d` | flash pick, then view the image big / download the attachment to `~/Downloads` |
| `enter` on an image | image viewer: `d` download, `o` open, `n`/`p` next and previous image, `esc` close |
| `yy` `rr` `RR` ... | same thing on the message under the cursor |
| `p` `m` `L` `A` `u` `!` | pin, mute, low priority, archive, read/unread, reminder |
| `O` | jump to this chat (or message) in Beeper Desktop |
| `n` `N` | new DM, new group |
| `[` `]` | cycle inbox: primary, low-priority, archive, unread |
| `/` | search chats |
| `:` | command line, `tab` completes. `:help` lists everything |
| `C-r` | refresh |
| `q` | quit |

`?` opens the full key reference, split by pane, with the pane you're in highlighted. The bottom-left corner always shows a short cheat sheet for whatever has focus.

## Mouse

Click a chat to open it, click a message to select it (click an image again to view it), click an inbox tab to switch, click the compose box to write. The wheel scrolls chats, messages, and images in the viewer. Mouse capture means you need shift (or option, depending on your terminal) to select text. `:mouse off` turns it off, or set `BEEPERTUI_MOUSE=off`.

## Mentions

Type `@` while writing and the chat's members pop up above the input. Keep typing to filter (surnames work), `tab` or `enter` picks, `C-n`/`C-p` move, `esc` hides the list. Picked names go out as proper mentions.

## Images and files

While writing, `C-v` grabs whatever is on the macOS clipboard: a screenshot or copied image becomes a PNG, and a file copied in Finder is attached as is. Dragging a file into the terminal attaches it too. `:attach <path>` works when you know the path. The next `enter` sends it along with any text.

Images and video posters show inline. In Ghostty and kitty they're real images, drawn with the kitty graphics protocol. Everywhere else, tmux included, they fall back to chunky half-block thumbnails. HEIC and WebP work too. `o` opens the full thing, and `:images off` hides them. Set `BEEPERTUI_IMAGES=kitty|blocks|off` to override the detection.

## Reacting

`R` opens an emoji picker:

- `1`-`6` send a quick one (thumbs up, heart, laugh, wow, sad, pray).
- Type to search by name: `fire`, `tada`, `+1`, `:eyes:`. Your recent picks show up first.
- Paste any emoji and press enter.
- Text that matches no emoji, or anything sent with `alt+enter`, goes out as a text reaction. Other Beeper clients display it as is.

Commands worth knowing: `:search <words>` searches every message across every network, `:attach ~/file.pdf` stages a file for the next message, `:remind 2h`, `:expiry 24h` for disappearing messages, `:label`, `:accounts`, `:info`.

## Themes and tuios

tuios colours and looks are supported. There are no hardcoded colours. Everything uses your terminal's 16 ANSI colours and its default text colour, so it picks up whatever theme you run, light ones included. That makes it tuios compatible: apply a look (`tuios look apply tuios.dev/r/19000274`) and the app restyles with it. Inline images work inside tuios too. tuios forwards kitty Unicode placeholders and tells panes they're on Ghostty when the host terminal can draw them, so you get real images in a tuios pane on Ghostty or kitty, and half-block thumbnails otherwise.

## Not in here

Connecting new networks and verifying devices still happen in Beeper Desktop itself. They're one-off setup flows with QR codes and login pages that don't make sense in a terminal. `:accounts` shows which ones need attention, and enter on one brings Beeper to the front.

Live updates come over a websocket. iMessage doesn't send events, so everything also refreshes every 30 seconds.
