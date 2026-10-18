package ui

import "charm.land/lipgloss/v2"

var (
	colAccent = lipgloss.Color("#89b4fa")
	colMuted  = lipgloss.Color("#6c7086")
	colText   = lipgloss.Color("#cdd6f4")
	colSelf   = lipgloss.Color("#a6e3a1")
	colOther  = lipgloss.Color("#f9e2af")
	colErr    = lipgloss.Color("#f38ba8")
	colLabel  = lipgloss.Color("#ff007c")
	colLabelF = lipgloss.Color("#1e1e2e")
	colSel    = lipgloss.Color("#313244")

	stPane      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colMuted)
	stPaneFocus = stPane.BorderForeground(colAccent)

	stDim      = lipgloss.NewStyle().Foreground(colMuted)
	stBold     = lipgloss.NewStyle().Bold(true).Foreground(colText)
	stText     = lipgloss.NewStyle().Foreground(colText)
	stSelf     = lipgloss.NewStyle().Bold(true).Foreground(colSelf)
	stOther    = lipgloss.NewStyle().Bold(true).Foreground(colOther)
	stErr      = lipgloss.NewStyle().Foreground(colErr)
	stAccent   = lipgloss.NewStyle().Foreground(colAccent)
	stCursor   = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	stSelRow   = lipgloss.NewStyle().Background(colSel)
	stLabel    = lipgloss.NewStyle().Background(colLabel).Foreground(colLabelF).Bold(true)
	stTab      = lipgloss.NewStyle().Foreground(colMuted).Padding(0, 1)
	stTabOn    = lipgloss.NewStyle().Foreground(colLabelF).Background(colAccent).Bold(true).Padding(0, 1)
	stMode     = lipgloss.NewStyle().Foreground(colLabelF).Background(colAccent).Bold(true).Padding(0, 1)
	stModeIns  = stMode.Background(colSelf)
	stModeAlt  = stMode.Background(colLabel)
	stOverlay  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colAccent).Padding(0, 1)
	stUnread   = lipgloss.NewStyle().Foreground(colLabelF).Background(colAccent).Bold(true)
	stReaction = lipgloss.NewStyle().Foreground(colText).Background(colSel)
)
