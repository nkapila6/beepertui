package ui

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"
)

const composeMaxLines = 6

// newCompose is a multi-line box that grows with its content. Enter sends
// (handled before it gets here); shift+enter is a newline where the terminal
// can tell it apart from enter (kitty keyboard protocol: Ghostty, kitty,
// WezTerm), with alt+enter and ctrl+j as fallbacks that work anywhere.
func newCompose() textarea.Model {
	t := textarea.New()
	t.Prompt = ""
	t.Placeholder = "i to write a message"
	t.ShowLineNumbers = false
	t.CharLimit = 0
	t.DynamicHeight = true
	t.MinHeight = 1
	t.MaxHeight = composeMaxLines
	t.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("shift+enter", "alt+enter", "ctrl+j"))

	plain := lipgloss.NewStyle()
	st := t.Styles()
	for _, s := range []*textarea.StyleState{&st.Focused, &st.Blurred} {
		s.Base = plain
		s.CursorLine = plain
		s.Text = plain
		s.EndOfBuffer = plain
		s.Placeholder = stDim
		s.Prompt = plain
	}
	t.SetStyles(st)
	return t
}

// composeLines is how tall the input currently is.
func (m *Model) composeLines() int {
	return clamp(m.compose.Height(), 1, composeMaxLines)
}
