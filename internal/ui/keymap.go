package ui

import (
	"fmt"
	"strings"

	"github.com/nkapila6/beepertui/internal/config"
)

// Remappable keys. Handlers switch on action names, never on key strings,
// so a bind changed in config.toml (key.<action> = "k1, k2") works everywhere.
// Sequences like "g g" are two presses. esc and ctrl+c stay fixed.

const (
	secAnywhere = "Anywhere"
	secChat     = "Chat (list cursor, or the open chat)"
	secChats    = "Chat list"
	secMsgs     = "Messages"
	secWriting  = "Writing"
)

func kb(action, keys, section, help string) config.KeyBind {
	return config.KeyBind{Action: action, Keys: keys, Section: section, Help: help}
}

var keyDefs = []config.KeyBind{
	kb("quit", "q", secAnywhere, "quit"),
	kb("help", "?", secAnywhere, "show every key"),
	kb("switch_pane", "tab", secAnywhere, "switch between chats and messages"),
	kb("focus_chats", "h, left", secAnywhere, "go to the chat list"),
	kb("focus_msgs", "l, right", secAnywhere, "go to the messages"),
	kb("down", "j, down", secAnywhere, "move down"),
	kb("up", "k, up", secAnywhere, "move up (at the top of a chat, loads older)"),
	kb("half_down", "ctrl+d", secAnywhere, "half page down"),
	kb("half_up", "ctrl+u", secAnywhere, "half page up"),
	kb("top", "g g", secAnywhere, "top / oldest"),
	kb("bottom", "G", secAnywhere, "bottom / newest"),
	kb("next_chat", "J", secAnywhere, "next chat"),
	kb("prev_chat", "K", secAnywhere, "previous chat"),
	kb("write", "i, a", secAnywhere, "write a message"),
	kb("search", "/", secAnywhere, "chat list: filter chats, messages: search this chat"),
	kb("command", ":", secAnywhere, "command line"),
	kb("prev_inbox", "[", secAnywhere, "previous inbox"),
	kb("next_inbox", "]", secAnywhere, "next inbox"),
	kb("refresh", "ctrl+r", secAnywhere, "refresh"),
	kb("jump", "s", secAnywhere, "flash: jump to anything on screen"),
	kb("chats", "c", secAnywhere, "flash: switch chat (again for the chat list popup)"),
	kb("chat_action", "C", secAnywhere, "flash: pick a chat and act on it (again for the open chat)"),
	kb("link_hints", "f", secAnywhere, "link hints, then o opens / y yanks"),
	kb("paste", "ctrl+v", secAnywhere, "paste an image or file"),
	kb("toggle_sidebar", `ctrl+", ctrl+shift+', ctrl+b`, secAnywhere, "hide / show the chat list"),
	kb("new_dm", "n", secAnywhere, "new DM"),
	kb("new_group", "N", secAnywhere, "new group"),

	kb("pin", "p", secChat, "pin / unpin"),
	kb("mute", "m", secChat, "mute / unmute"),
	kb("low_priority", "L", secChat, "low priority / primary"),
	kb("archive", "A", secChat, "archive / unarchive"),
	kb("unread", "u", secChat, "mark read / unread"),
	kb("remind", "!", secChat, "set a reminder"),
	kb("open_in_beeper", "O", secChat, "open in Beeper Desktop"),

	kb("open_chat", "enter", secChats, "open chat"),

	kb("yank", "y", secMsgs, "flash: yank text (again for the cursor)"),
	kb("reply", "r", secMsgs, "flash: reply"),
	kb("react", "R", secMsgs, "flash: react"),
	kb("edit", "e", secMsgs, "flash: edit (yours)"),
	kb("delete", "x", secMsgs, "flash: delete (yours, asks first)"),
	kb("open", "o", secMsgs, "flash: open link or attachment"),
	kb("unreact", "U", secMsgs, "flash: remove your reaction"),
	kb("view", "v", secMsgs, "flash: view an image big"),
	kb("download", "d", secMsgs, "flash: download to ~/Downloads"),
	kb("forward", "F", secMsgs, "flash: forward to another chat"),
	kb("select", "enter", secMsgs, "on an image: viewer, otherwise write"),

	kb("send", "enter", secWriting, "send"),
	kb("paste_image", "ctrl+v", secWriting, "attach clipboard image or copied file"),
	kb("media_picker", "ctrl+g", secWriting, "media picker"),
	kb("emoji_picker", "ctrl+e", secWriting, "emoji picker"),
	kb("attach_file", "ctrl+o", secWriting, "pick a file to attach"),
}

// flash actions keep their letter internally (labels, status text); the key
// that starts them is whatever is bound.
var flashActions = map[string]rune{
	"jump": 's', "chats": 'c', "chat_action": 'C',
	"yank": 'y', "reply": 'r', "react": 'R', "edit": 'e', "delete": 'x', "open": 'o',
	"unreact": 'U', "view": 'v', "download": 'd', "forward": 'F',
}

func init() { config.Binds = keyDefs }

// clashes says which sections share a key space: normal mode checks the
// anywhere and chat sections together with whichever pane is focused.
func clashes(a, b string) bool {
	if a == b {
		return true
	}
	normal := map[string]bool{secAnywhere: true, secChat: true, secChats: true, secMsgs: true}
	if !normal[a] || !normal[b] {
		return false
	}
	// the two panes never see each other's keys
	return !(a == secChats && b == secMsgs || a == secMsgs && b == secChats)
}

type keymap struct {
	keys    map[string][]string // action -> keys
	section map[string]string   // action -> section
	prefix  map[string]bool     // first halves of sequences
}

func buildKeymap(c *config.Config) keymap {
	km := keymap{keys: map[string][]string{}, section: map[string]string{}, prefix: map[string]bool{}}
	for _, d := range keyDefs {
		v := d.Keys
		if c != nil {
			v = c.String(config.KeyPrefix + d.Action)
		}
		km.keys[d.Action] = config.SplitKeys(v)
		km.section[d.Action] = d.Section
		for _, k := range km.keys[d.Action] {
			if first, _, ok := strings.Cut(k, " "); ok {
				km.prefix[first] = true
			}
		}
	}
	return km
}

// action finds what key does in the given sections, first match wins.
func (km keymap) action(key string, sections ...string) string {
	for _, sec := range sections {
		for _, d := range keyDefs {
			if d.Section != sec {
				continue
			}
			for _, k := range km.keys[d.Action] {
				if k == key {
					return d.Action
				}
			}
		}
	}
	return ""
}

// conflict returns the action that already uses key where it would clash with action.
func (km keymap) conflict(action, key string) string {
	for _, d := range keyDefs {
		if d.Action == action || !clashes(km.section[action], d.Section) {
			continue
		}
		for _, k := range km.keys[d.Action] {
			if k == key {
				return d.Action
			}
		}
	}
	return ""
}

// label is the short form for hints and help: "C-r  C-b".
func (km keymap) label(action string) string {
	ks := km.keys[action]
	if len(ks) == 0 {
		return "unbound"
	}
	out := make([]string, len(ks))
	for i, k := range ks {
		out[i] = shortKey(k)
	}
	return strings.Join(out, "  ")
}

// first is the first bound key in short form, for one-key hints.
func (km keymap) first(action string) string {
	if ks := km.keys[action]; len(ks) > 0 {
		return shortKey(ks[0])
	}
	return "-"
}

func shortKey(k string) string {
	k = strings.ReplaceAll(k, " ", "")
	r := strings.NewReplacer("ctrl+shift+", "C-S-", "ctrl+", "C-", "shift+", "S-", "alt+", "A-")
	return r.Replace(k)
}

// BindClashes describes where action's keys collide with other actions, for
// `beepertui config set`. The first match wins at runtime, so these are warnings.
func BindClashes(c *config.Config, action string) []string {
	km := buildKeymap(c)
	var out []string
	for _, k := range km.keys[action] {
		if other := km.conflict(action, k); other != "" {
			out = append(out, fmt.Sprintf("%s is also bound to %s", k, other))
		}
	}
	return out
}
