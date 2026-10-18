package ui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nkapila6/beepertui/internal/beeper"
)

// Read receipts. Beeper's "seen" field is loosely typed by network: a bool,
// a timestamp, or (mostly in groups) a map of participant id to bool or
// timestamp.

type seenBy struct {
	id string
	at time.Time // zero when the network only says yes
}

type seenState struct {
	seen bool
	at   time.Time
	by   []seenBy // per person, when the network reports it
}

func parseSeen(raw json.RawMessage) seenState {
	var st seenState
	if len(raw) == 0 || string(raw) == "null" {
		return st
	}
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		st.seen = b
		return st
	}
	var ts time.Time
	if json.Unmarshal(raw, &ts) == nil {
		st.seen, st.at = true, ts
		return st
	}
	var per map[string]json.RawMessage
	if json.Unmarshal(raw, &per) == nil {
		for id, v := range per {
			e := parseSeen(v)
			if e.seen {
				st.by = append(st.by, seenBy{id: id, at: e.at})
			}
		}
		sort.Slice(st.by, func(a, b int) bool { return st.by[a].at.Before(st.by[b].at) })
		st.seen = len(st.by) > 0
	}
	return st
}

// lastOwnMessage is the index of your newest message in the chat, or -1.
func (m *Model) lastOwnMessage() int {
	for i := len(m.msgs) - 1; i >= 0; i-- {
		if m.msgs[i].IsSender {
			return i
		}
	}
	return -1
}

// others is how many people besides you are in the chat, when known.
func (m *Model) others() int {
	if m.cur == nil {
		return 0
	}
	n := m.cur.Participants.Total
	if n == 0 {
		n = len(m.cur.Participants.Items)
	}
	return max(0, n-1)
}

// seenLine is the short status under a message of yours.
func (m *Model) seenLine(msg beeper.Message) string {
	if msg.SendStatus != nil {
		switch {
		case msg.SendStatus.Status == "PENDING":
			return "sending..."
		case strings.HasPrefix(msg.SendStatus.Status, "FAIL"):
			return "not sent: " + firstNonEmpty(msg.SendStatus.Message, "failed")
		}
	}
	st := parseSeen(msg.Seen)
	switch {
	case len(st.by) > 0:
		names := make([]string, 0, len(st.by))
		for _, b := range st.by {
			names = append(names, firstWord(m.personName(b.id)))
		}
		line := "seen by " + joinNames(names, 4)
		if n := m.others(); n > 1 {
			line += fmt.Sprintf(" · %d of %d", len(st.by), n)
		}
		return line
	case st.seen && !st.at.IsZero():
		return "seen " + shortTime(st.at)
	case st.seen:
		return "seen"
	}
	return "sent"
}

// seenDetail is the hover tooltip: everyone who saw it, with times when known.
func (m *Model) seenDetail(msg beeper.Message) []string {
	st := parseSeen(msg.Seen)
	if len(st.by) == 0 {
		return []string{m.seenLine(msg)}
	}
	out := []string{stBold.Render("seen by")}
	for _, b := range st.by {
		line := m.personName(b.id)
		if !b.at.IsZero() {
			line = padRight(line, 18) + stDim.Render(" "+shortTime(b.at))
		}
		out = append(out, line)
	}
	if n := m.others(); n > len(st.by) {
		out = append(out, stDim.Render(fmt.Sprintf("not yet: %d", n-len(st.by))))
	}
	return out
}

func firstWord(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return s
}

// joinNames lists up to n names, then "+k".
func joinNames(names []string, n int) string {
	if len(names) <= n {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:n], ", ") + fmt.Sprintf(" +%d", len(names)-n)
}
