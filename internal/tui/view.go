package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m Model) View() tea.View {
	var body strings.Builder
	body.WriteString(theme.Title.Render("easyvenv") + "  " + theme.Muted.Render("Named Python environments") + "\n\n")
	switch m.screen {
	case listScreen:
		body.WriteString(m.list.View())
	case createScreen:
		body.WriteString(theme.Title.Render("Create environment") + "\n\nName\n" + m.inputs[0].View() + "\n\nPython version (optional)\n" + m.inputs[1].View() + "\n\n")
		body.WriteString(theme.Muted.Render("Leave the version empty to use your current Python.\nA supplied version uses mise on demand."))
	case detailsScreen:
		env := m.selected
		body.WriteString(theme.Title.Render(env.Name) + "\n\n")
		fmt.Fprintf(&body, "Path     %s\nPython   %s\nVersion  %s\nActive   %t\n", env.Path, env.Python, env.Version, env.Active)
		if env.Source != "" {
			fmt.Fprintf(&body, "Source   %s\n", env.Source)
		}
		if env.Requested != "" {
			fmt.Fprintf(&body, "Requested %s\n", env.Requested)
		}
		if !env.CreatedAt.IsZero() {
			fmt.Fprintf(&body, "Created  %s\n", env.CreatedAt.Format("2006-01-02 15:04 MST"))
		}
	case deleteScreen:
		body.WriteString(theme.Title.Render("Delete environment?") + "\n\n")
		fmt.Fprintf(&body, "%s\n%s\n\nThe environment and its installed packages will be removed.\n", m.selected.Name, m.selected.Path)
	case miseScreen:
		body.WriteString(theme.Title.Render("Install mise?") + "\n\nThe requested Python version needs mise.\nInstall it using its official installer?\n")
	}
	if m.busy != "" {
		body.WriteString("\n" + m.spinner.View() + " " + m.busy + "\n" + theme.Muted.Render("ctrl+c to cancel"))
	} else {
		if m.failure != nil {
			body.WriteString("\n" + theme.Error.Render(m.failure.Error()))
		}
		if m.notice != "" {
			body.WriteString("\n" + theme.Accent.Render(m.notice))
		}
		body.WriteString("\n\n" + m.help.View(m))
	}
	view := tea.NewView(theme.Frame.Render(lipgloss.NewStyle().Width(max(10, m.width-4)).Render(body.String())))
	view.AltScreen = true
	return view
}
