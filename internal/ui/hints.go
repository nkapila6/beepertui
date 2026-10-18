package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Vimium-style link hints: f labels every link on screen, typing a label
// selects it, then o (or enter) opens it in the browser and y yanks it.
// The same hit boxes make links clickable with the mouse.

type linkHit struct {
	x, y, w int
	url     string
}

type linkHints struct {
	hits   []linkHit
	labels []string
	typed  string
	sel    int // -1 until a label is picked
}

type msgLink struct{ display, url string }

// linksOf lists a message's links in reading order with the text they show as.
func linksOf(text string, previews []string, previewURLs []string) []msgLink {
	var out []msgLink
	seen := map[string]bool{}
	for _, sp := range parseRich(text) {
		if sp.href != "" && !isMention(sp.href) && !seen[sp.href] {
			out = append(out, msgLink{strings.TrimSpace(sp.text), sp.href})
			seen[sp.href] = true
		}
	}
	for _, u := range urlRe.FindAllString(plainText(text), -1) {
		if !seen[u] {
			out = append(out, msgLink{u, u})
			seen[u] = true
		}
	}
	for i, u := range previewURLs {
		if u != "" && previews[i] != "" && !seen[u+"#card"] {
			// the preview card's title is its own target, even when the url was already in the text
			out = append(out, msgLink{previews[i], u})
			seen[u+"#card"] = true
		}
	}
	return out
}

// linkHits finds where each visible link starts on screen, by searching the
// rendered rows of its message for the link's text.
func (m *Model) linkHits() []linkHit {
	rows := m.msgRows()
	if len(rows) == 0 {
		return nil
	}
	x0 := m.chatWidth() + 1 + gutter
	var hits []linkHit
	done := map[int]bool{}
	for k, r := range rows {
		if r.idx < 0 || done[r.idx] {
			continue
		}
		done[r.idx] = true
		msg := m.msgs[r.idx]
		var titles, urls []string
		for _, l := range msg.Links {
			titles = append(titles, oneLine(l.Title))
			urls = append(urls, l.URL)
		}
		// search forward through this message's rows so links keep their order
		row, col := k, 0
		for _, l := range linksOf(msg.Text, titles, urls) {
			needle := []rune(l.display)
			needle = needle[:min(len(needle), 12)]
			if len(needle) == 0 {
				continue
			}
			for rr := row; rr < len(rows) && rows[rr].idx == r.idx; rr++ {
				plain := ansi.Strip(rows[rr].text)
				from := 0
				if rr == row {
					from = min(col, len(plain))
				}
				if at := strings.Index(plain[from:], string(needle)); at >= 0 {
					start := from + at
					cx := lipgloss.Width(plain[:start])
					w := min(lipgloss.Width(l.display), lipgloss.Width(plain[start:]))
					hits = append(hits, linkHit{x: x0 + cx, y: 2 + rr, w: w, url: l.url})
					row, col = rr, start+len(string(needle))
					break
				}
			}
		}
	}
	return hits
}

func (m *Model) startLinkHints() {
	hits := m.linkHits()
	if len(hits) == 0 {
		m.setErr("no links on screen")
		return
	}
	m.hints = linkHints{hits: hits, labels: makeLabels(len(hits), labelChars), sel: -1}
	m.mode = modeLinks
}

func (m *Model) linkKey(key string) tea.Cmd {
	h := &m.hints
	if key == "esc" {
		m.mode = modeNormal
		return nil
	}
	if h.sel >= 0 {
		u := h.hits[h.sel].url
		switch key {
		case "o", "enter":
			m.mode = modeNormal
			return m.openLink(u)
		case "y":
			m.mode = modeNormal
			if err := copyText(u); err != nil {
				m.setStatus("yanked link (osc52)")
				return tea.SetClipboard(u)
			}
			m.setStatus("yanked " + u)
			return nil
		}
		return nil
	}
	if len(key) != 1 {
		m.mode = modeNormal
		return nil
	}
	h.typed += key
	partial := false
	for i, l := range h.labels {
		if l == h.typed {
			h.sel = i
			return nil
		}
		if strings.HasPrefix(l, h.typed) {
			partial = true
		}
	}
	if !partial {
		m.mode = modeNormal
		m.setErr("no label " + h.typed)
	}
	return nil
}

func (m *Model) openLink(u string) tea.Cmd {
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		m.setErr("not a web link: " + u)
		return nil
	}
	m.setStatus("opened " + u)
	return func() tea.Msg {
		openURL(u)
		return nil
	}
}

// linkAt returns the link under a screen position, for mouse clicks.
func (m *Model) linkAt(x, y int) (string, bool) {
	for _, h := range m.linkHits() {
		if y == h.y && x >= h.x && x < h.x+h.w {
			return h.url, true
		}
	}
	return "", false
}

// hintLayers are the floating labels drawn over each link.
func (m *Model) hintLayers() []*lipgloss.Layer {
	if m.mode != modeLinks {
		return nil
	}
	var out []*lipgloss.Layer
	for i, hit := range m.hints.hits {
		st := stLabel
		label := m.hints.labels[i]
		switch {
		case m.hints.sel == i:
			st = stModeIns
			label = "o open  y yank"
		case m.hints.sel >= 0:
			continue
		case !strings.HasPrefix(label, m.hints.typed):
			continue
		}
		out = append(out, lipgloss.NewLayer(st.Render(label)).X(hit.x).Y(hit.y).Z(2))
	}
	return out
}
