package ui

import "charm.land/lipgloss/v2"

// Every colour is one of the terminal's 16 ANSI slots or its default
// foreground, never a hex value. Whatever theme the terminal (or a tuios look)
// sets, the app follows it, light themes included.
var (
	colAccent = lipgloss.Blue
	colMuted  = lipgloss.BrightBlack
	colText   = lipgloss.NoColor{}
	colSelf   = lipgloss.Green
	colOther  = lipgloss.Yellow
	// bright red, since some themes park color1 near the background
	colErr    = lipgloss.BrightRed
	colLabel  = lipgloss.Magenta
	colLabelF = lipgloss.Black
	colSel    = lipgloss.BrightBlack

	stPane      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colMuted)
	stPaneFocus = stPane.BorderForeground(colAccent)

	stDim          = lipgloss.NewStyle().Foreground(colMuted)
	stBold         = lipgloss.NewStyle().Bold(true).Foreground(colText)
	stText         = lipgloss.NewStyle().Foreground(colText)
	stSelf         = lipgloss.NewStyle().Bold(true).Foreground(colSelf)
	stOther        = lipgloss.NewStyle().Bold(true).Foreground(colOther)
	stErr          = lipgloss.NewStyle().Foreground(colErr)
	stAccent       = lipgloss.NewStyle().Foreground(colAccent)
	stCursor       = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	stSelRow       = lipgloss.NewStyle().Background(colSel)
	stLabel        = lipgloss.NewStyle().Background(colLabel).Foreground(colLabelF).Bold(true)
	stTab          = lipgloss.NewStyle().Foreground(colMuted).Padding(0, 1)
	stTabOn        = lipgloss.NewStyle().Foreground(colLabelF).Background(colAccent).Bold(true).Padding(0, 1)
	stMode         = lipgloss.NewStyle().Foreground(colLabelF).Background(colAccent).Bold(true).Padding(0, 1)
	stModeIns      = stMode.Background(colSelf)
	stModeAlt      = stMode.Background(colLabel)
	stOverlay      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colAccent).Padding(0, 1)
	stUnread       = lipgloss.NewStyle().Foreground(colLabelF).Background(colAccent).Bold(true)
	stReaction     = lipgloss.NewStyle().Foreground(colText).Background(colSel)
	stReactionMine = lipgloss.NewStyle().Foreground(colLabelF).Background(colAccent)
)
