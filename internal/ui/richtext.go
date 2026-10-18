package ui

import (
	"html"
	"regexp"
	"strings"
)

// Beeper hands back message bodies as Matrix-flavoured HTML when there's any
// formatting or a link. This is a small tag walker for the handful of tags that
// show up in practice. x/net/html would be overkill for it.

var (
	tagRe  = regexp.MustCompile(`<(/?)([a-zA-Z][a-zA-Z0-9-]*)((?:\s[^>]*)?)\s*/?>`)
	hrefRe = regexp.MustCompile(`(?i)href\s*=\s*"([^"]*)"|href\s*=\s*'([^']*)'`)
)

type span struct {
	text   string
	href   string
	bold   bool
	italic bool
	code   bool
	strike bool
	quote  bool
}

// parseRich flattens the body into styled spans. Text with no tags passes through untouched.
func parseRich(s string) []span {
	if !strings.ContainsAny(s, "<&") {
		return []span{{text: s}}
	}
	var (
		out                               []span
		bold, italic, code, strike, quote int
		skip                              int // inside <mx-reply>, the quoted reply fallback
		href                              string
	)
	emit := func(t string) {
		if skip > 0 || t == "" {
			return
		}
		out = append(out, span{text: html.UnescapeString(t), href: href, bold: bold > 0, italic: italic > 0, code: code > 0, strike: strike > 0, quote: quote > 0})
	}
	nl := func() {
		if skip == 0 && len(out) > 0 && !strings.HasSuffix(out[len(out)-1].text, "\n") {
			out = append(out, span{text: "\n"})
		}
	}
	last := 0
	for _, loc := range tagRe.FindAllStringSubmatchIndex(s, -1) {
		emit(s[last:loc[0]])
		last = loc[1]
		closing := loc[3] > loc[2]
		name := strings.ToLower(s[loc[4]:loc[5]])
		attrs := s[loc[6]:loc[7]]
		d := 1
		if closing {
			d = -1
		}
		switch name {
		case "mx-reply":
			skip = max(0, skip+d)
		case "b", "strong":
			bold = max(0, bold+d)
		case "i", "em":
			italic = max(0, italic+d)
		case "code":
			code = max(0, code+d)
		case "s", "del", "strike":
			strike = max(0, strike+d)
		case "blockquote":
			quote = max(0, quote+d)
			nl()
		case "pre":
			code = max(0, code+d)
			nl()
		case "a":
			if closing {
				href = ""
			} else if mm := hrefRe.FindStringSubmatch(attrs); mm != nil {
				href = html.UnescapeString(mm[1] + mm[2])
			}
		case "br":
			if skip == 0 {
				out = append(out, span{text: "\n"})
			}
		case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "tr":
			nl()
		case "li":
			if !closing {
				nl()
				emit("- ")
			}
		}
	}
	emit(s[last:])
	// trailing newlines from block tags just waste a line
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1].text) == "" {
		out = out[:len(out)-1]
	}
	return out
}

func isMention(href string) bool {
	return strings.Contains(href, "matrix.to/#/@")
}

// plainText is what yank, edit and one-line previews use.
func plainText(s string) string {
	var b strings.Builder
	for _, sp := range parseRich(s) {
		if isMention(sp.href) && !strings.HasPrefix(sp.text, "@") {
			b.WriteString("@")
		}
		b.WriteString(sp.text)
	}
	return b.String()
}

// richLinks returns every href in the body, for the open action.
func richLinks(s string) []string {
	var out []string
	for _, sp := range parseRich(s) {
		if sp.href != "" && !isMention(sp.href) {
			out = append(out, sp.href)
		}
	}
	return out
}

// renderRich styles the body for the terminal. Links become OSC 8 hyperlinks,
// so they're clickable in terminals that support it.
func renderRich(s string) string {
	var b strings.Builder
	for _, sp := range parseRich(s) {
		st := stText
		text := sp.text
		switch {
		case isMention(sp.href):
			st = stAccent.Bold(true)
			if !strings.HasPrefix(text, "@") {
				text = "@" + text
			}
		case sp.href != "":
			st = stAccent.Underline(true).Hyperlink(sp.href)
		}
		if sp.bold {
			st = st.Bold(true)
		}
		if sp.italic {
			st = st.Italic(true)
		}
		if sp.strike {
			st = st.Strikethrough(true)
		}
		if sp.code {
			st = st.Foreground(colOther).Background(colSel)
		}
		if sp.quote {
			st = st.Foreground(colMuted)
		}
		// render line by line so styles don't bleed across wraps
		parts := strings.Split(text, "\n")
		for i, p := range parts {
			if i > 0 {
				b.WriteString("\n")
			}
			if p != "" {
				b.WriteString(st.Render(p))
			}
		}
	}
	return b.String()
}
