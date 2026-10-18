package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Every colour is one of the terminal's 16 ANSI slots or its default
// foreground, never a hex value (unless you set a hex accent yourself).
// Whatever theme the terminal (or a tuios look) sets, the app follows it.
var (
	colAccent color.Color = lipgloss.Blue
	colMuted  color.Color = lipgloss.BrightBlack
	colText   color.Color = lipgloss.NoColor{}
	colSelf   color.Color = lipgloss.Green
	colOther  color.Color = lipgloss.Yellow
	// bright red, since some themes park color1 near the background
	colErr   color.Color = lipgloss.BrightRed
	colLabel color.Color = lipgloss.Magenta
	colSel   color.Color = lipgloss.BrightBlack

	stPane, stPaneFocus                             lipgloss.Style
	stDim, stBold, stText, stSelf, stOther, stErr   lipgloss.Style
	stAccent, stCursor, stSelRow, stLabel, stTab    lipgloss.Style
	stTabOn, stMode, stModeIns, stModeAlt           lipgloss.Style
	stOverlay, stUnread, stReaction, stReactionMine lipgloss.Style
)

func init() { buildStyles() }

// buildStyles derives every style from the colours above. It runs again when
// the accent changes from the config or :set.
func buildStyles() {
	stPane = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colMuted)
	stPaneFocus = stPane.BorderForeground(colAccent)

	stDim = lipgloss.NewStyle().Foreground(colMuted)
	stBold = lipgloss.NewStyle().Bold(true).Foreground(colText)
	stText = lipgloss.NewStyle().Foreground(colText)
	stSelf = lipgloss.NewStyle().Bold(true).Foreground(colSelf)
	stOther = lipgloss.NewStyle().Bold(true).Foreground(colOther)
	stErr = lipgloss.NewStyle().Foreground(colErr)
	stAccent = lipgloss.NewStyle().Foreground(colAccent)
	stCursor = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	stSelRow = lipgloss.NewStyle().Background(colSel)
	stLabel = pill(colLabel)
	stTab = lipgloss.NewStyle().Foreground(colMuted).Padding(0, 1)
	stTabOn = pill(colAccent).Padding(0, 1)
	stMode = pill(colAccent).Padding(0, 1)
	stModeIns = pill(colSelf).Padding(0, 1)
	stModeAlt = pill(colLabel).Padding(0, 1)
	stOverlay = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colAccent).Padding(0, 1)
	stUnread = pill(colAccent)
	stReaction = lipgloss.NewStyle().Foreground(colText).Background(colSel)
	stReactionMine = pill(colAccent).Bold(false)
}

// pill is coloured background with the terminal's own background colour as the
// text, via reverse video. Black-on-colour depends on the theme's color0 and
// can vanish; the real background never clashes with the theme's accents.
func pill(c color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c).Reverse(true).Bold(true)
}

// fill is the colour a style paints as its background, reverse video included.
func fill(st lipgloss.Style) color.Color {
	if st.GetReverse() {
		return st.GetForeground()
	}
	return st.GetBackground()
}
