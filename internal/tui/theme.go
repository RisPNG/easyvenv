package tui

import "charm.land/lipgloss/v2"

var theme = struct {
	Title  lipgloss.Style
	Muted  lipgloss.Style
	Error  lipgloss.Style
	Accent lipgloss.Style
	Frame  lipgloss.Style
}{
	Title:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("75")),
	Muted:  lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
	Error:  lipgloss.NewStyle().Foreground(lipgloss.Color("203")),
	Accent: lipgloss.NewStyle().Foreground(lipgloss.Color("78")),
	Frame:  lipgloss.NewStyle().Padding(1, 2),
}
