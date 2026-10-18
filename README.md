# beepertui

A terminal client for Beeper, talking to the Beeper Desktop app running on your Mac. Vim-ish keys, and flash.nvim-style labels for picking a message to yank, reply to, react to, edit or delete.

Beeper Desktop does the real work (bridges, encryption). This is just a frontend for its local API on `localhost:23373`.

## Setup

1. In Beeper Desktop: Settings > Developers, turn on the Beeper Desktop API and create an access token.
2. Run it:

```bash
export BEEPER_ACCESS_TOKEN=...
go run .
```

`BEEPER_URL` overrides the API address if you've moved it.

## Keys

| key | does |
| --- | --- |
| `j` `k` `gg` `G` `C-d` `C-u` | move |
| `h` `l` `tab` | switch between chats and messages |
| `enter` | open chat / start writing |
| `i` `a` | write. `enter` sends, `esc` leaves, `C-v` pastes an image, `C-c` clears |
| `s` | flash jump anywhere: labels every chat and message on screen at once |
| `c` | flash just the chats and switch to one, from either pane |
| `C` | flash a chat, then a one-key menu: open, pin, mute, archive, low priority, read/unread, remind, info. `CC` = current chat |
| `y` `r` `R` `e` `x` `o` `U` | flash pick, then yank / reply / react / edit / delete / open / unreact |
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

## Not in here

Connecting new networks and verifying devices still happen in Beeper Desktop itself. They're one-off setup flows with QR codes and login pages that don't make sense in a terminal. `:accounts` shows which ones need attention, and enter on one brings Beeper to the front.

Live updates come over a websocket. iMessage doesn't send events, so everything also refreshes every 30 seconds.
