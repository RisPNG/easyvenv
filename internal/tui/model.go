package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/RisPNG/easyvenv/internal/environment"
)

type screen int

const (
	listScreen screen = iota
	createScreen
	detailsScreen
	deleteScreen
	miseScreen
)

type EnvironmentsListedMsg struct{ Environments []environment.Environment }
type PythonResolvedMsg struct{ Python environment.Interpreter }
type PythonInstalledMsg struct{ Python environment.Interpreter }
type MiseInstalledMsg struct{}
type EnvironmentCreatedMsg struct{ Environment environment.Environment }
type EnvironmentDeletedMsg struct{ Name string }
type OperationFailedMsg struct{ Err error }
type cancellationRequestedMsg struct{}

type item struct{ environment.Environment }

func (i item) Title() string {
	if i.Active {
		return i.Name + " (active)"
	}
	return i.Name
}
func (i item) Description() string { return "Python " + i.Version + " · " + i.Path }
func (i item) FilterValue() string { return i.Name }

type Model struct {
	service    *environment.Service
	ctx        context.Context
	cancel     context.CancelFunc
	screen     screen
	list       list.Model
	inputs     [2]textinput.Model
	focus      int
	spinner    spinner.Model
	help       help.Model
	selected   environment.Environment
	activation string
	busy       string
	notice     string
	failure    error
	quitting   bool
	width      int
}

func New(ctx context.Context, service *environment.Service) Model {
	ctx, cancel := context.WithCancel(ctx)
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = theme.Title.BorderLeftForeground(theme.Title.GetForeground())
	delegate.Styles.SelectedDesc = theme.Muted
	entries := list.New([]list.Item{}, delegate, 76, 18)
	entries.Title = "Environments"
	entries.SetShowHelp(false)
	entries.DisableQuitKeybindings()
	name, version := textinput.New(), textinput.New()
	name.Placeholder, version.Placeholder = "myproject", "Current Python from PATH (optional)"
	name.CharLimit, version.CharLimit = 128, 32
	name.SetWidth(48)
	version.SetWidth(48)
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = theme.Accent
	return Model{service: service, ctx: ctx, cancel: cancel, list: entries, inputs: [2]textinput.Model{name, version}, spinner: spin, help: help.New(), width: 80, busy: "Loading environments"}
}

func Run(ctx context.Context, service *environment.Service, input io.Reader, output io.Writer) (string, error) {
	model := New(ctx, service)
	defer model.cancel()
	final, err := tea.NewProgram(model, tea.WithInput(input), tea.WithOutput(output), tea.WithoutSignalHandler()).Run()
	if err != nil {
		return "", err
	}
	return final.(Model).activation, nil
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refresh(), m.spinner.Tick, func() tea.Msg {
		<-m.ctx.Done()
		return cancellationRequestedMsg{}
	})
}

func (m Model) refresh() tea.Cmd {
	return func() tea.Msg {
		envs, err := m.service.List()
		if err != nil {
			return OperationFailedMsg{err}
		}
		return EnvironmentsListedMsg{envs}
	}
}

func (m Model) resolvePython() tea.Cmd {
	name, requested := m.inputs[0].Value(), m.inputs[1].Value()
	return func() tea.Msg {
		path, err := m.service.EnvironmentPath(name)
		if err != nil {
			return OperationFailedMsg{err}
		}
		if _, err := os.Lstat(path); err == nil {
			return OperationFailedMsg{fmt.Errorf("environment %q already exists", name)}
		} else if !errors.Is(err, os.ErrNotExist) {
			return OperationFailedMsg{err}
		}
		python, err := m.service.ResolvePython(m.ctx, requested)
		if err != nil {
			return OperationFailedMsg{err}
		}
		return PythonResolvedMsg{python}
	}
}

func (m Model) createEnvironment(python environment.Interpreter) tea.Cmd {
	name := m.inputs[0].Value()
	return func() tea.Msg {
		var output bytes.Buffer
		env, err := m.service.Create(m.ctx, name, python, &output)
		if err != nil {
			return OperationFailedMsg{fmt.Errorf("%w\n%s", err, strings.TrimSpace(output.String()))}
		}
		return EnvironmentCreatedMsg{env}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cancellationRequestedMsg:
		if m.busy != "" {
			m.quitting, m.busy = true, "Cancelling operation"
			return m, nil
		}
		return m, tea.Quit
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.list.SetSize(max(10, msg.Width-4), max(4, msg.Height-8))
		m.help.SetWidth(max(10, msg.Width-4))
		for i := range m.inputs {
			m.inputs[i].SetWidth(max(8, min(48, msg.Width-8)))
		}
	case EnvironmentsListedMsg:
		if m.quitting {
			return m, tea.Quit
		}
		items := make([]list.Item, 0, len(msg.Environments))
		for _, env := range msg.Environments {
			items = append(items, item{env})
		}
		m.busy = ""
		return m, m.list.SetItems(items)
	case OperationFailedMsg:
		m.busy = ""
		if m.quitting {
			return m, tea.Quit
		}
		if errors.Is(msg.Err, environment.ErrMiseMissing) {
			m.screen = miseScreen
			return m, nil
		}
		m.failure = msg.Err
	case PythonResolvedMsg:
		if m.quitting {
			return m, tea.Quit
		}
		if msg.Python.Source == "mise" {
			m.busy = "Preparing Python " + msg.Python.Version + " through mise"
			return m, func() tea.Msg {
				var output bytes.Buffer
				if err := m.service.Provisioner.InstallVersion(m.ctx, msg.Python.Version, &output); err != nil {
					return OperationFailedMsg{fmt.Errorf("%w\n%s", err, strings.TrimSpace(output.String()))}
				}
				return PythonInstalledMsg{msg.Python}
			}
		}
		m.busy = "Creating " + m.inputs[0].Value()
		return m, m.createEnvironment(msg.Python)
	case PythonInstalledMsg:
		if m.quitting {
			return m, tea.Quit
		}
		m.busy = "Creating " + m.inputs[0].Value()
		return m, m.createEnvironment(msg.Python)
	case MiseInstalledMsg:
		if m.quitting {
			return m, tea.Quit
		}
		m.screen, m.busy = createScreen, "Resolving Python"
		return m, m.resolvePython()
	case EnvironmentCreatedMsg:
		if m.quitting {
			return m, tea.Quit
		}
		m.screen, m.notice, m.busy = listScreen, "Created "+msg.Environment.Name, "Refreshing environments"
		return m, m.refresh()
	case EnvironmentDeletedMsg:
		if m.quitting {
			return m, tea.Quit
		}
		m.screen, m.notice, m.busy = listScreen, "Deleted "+msg.Name, "Refreshing environments"
		return m, m.refresh()
	case spinner.TickMsg:
		var command tea.Cmd
		m.spinner, command = m.spinner.Update(msg)
		return m, command
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.cancel()
			if m.busy != "" {
				m.quitting, m.busy = true, "Cancelling operation"
				return m, nil
			}
			return m, tea.Quit
		}
		if m.busy != "" {
			return m, nil
		}
		if m.screen == listScreen && m.list.FilterState() == list.Filtering {
			var command tea.Cmd
			m.list, command = m.list.Update(msg)
			return m, command
		}
		switch m.screen {
		case listScreen:
			switch msg.String() {
			case "q":
				return m, tea.Quit
			case "n":
				m.screen, m.focus, m.failure, m.notice = createScreen, 0, nil, ""
				m.inputs[0].SetValue("")
				m.inputs[1].SetValue("")
				m.inputs[1].Blur()
				return m, m.inputs[0].Focus()
			case "r":
				m.busy, m.failure = "Refreshing environments", nil
				return m, m.refresh()
			case "enter", "d", "a":
				selected, ok := m.list.SelectedItem().(item)
				if !ok {
					return m, nil
				}
				m.selected, m.failure = selected.Environment, nil
				switch msg.String() {
				case "enter":
					m.screen = detailsScreen
				case "d":
					m.screen = deleteScreen
				case "a":
					m.activation = m.selected.Name
					return m, tea.Quit
				}
			}
		case createScreen:
			switch msg.String() {
			case "esc":
				m.screen, m.failure = listScreen, nil
				return m, nil
			case "tab", "shift+tab":
				m.inputs[m.focus].Blur()
				m.focus = 1 - m.focus
				return m, m.inputs[m.focus].Focus()
			case "enter":
				m.busy, m.failure = "Resolving Python", nil
				return m, m.resolvePython()
			}
		case detailsScreen:
			switch msg.String() {
			case "esc", "q":
				m.screen = listScreen
			case "d":
				m.screen = deleteScreen
			case "a", "enter":
				m.activation = m.selected.Name
				return m, tea.Quit
			}
		case deleteScreen:
			switch msg.String() {
			case "esc", "n", "q":
				m.screen, m.failure = listScreen, nil
			case "y":
				m.busy, m.failure = "Deleting "+m.selected.Name, nil
				return m, func() tea.Msg {
					if err := m.service.Delete(m.selected.Name); err != nil {
						return OperationFailedMsg{err}
					}
					return EnvironmentDeletedMsg{m.selected.Name}
				}
			}
		case miseScreen:
			switch msg.String() {
			case "esc", "n", "q":
				m.screen = createScreen
			case "y":
				m.busy = "Installing mise"
				return m, func() tea.Msg {
					var output bytes.Buffer
					if err := m.service.Provisioner.InstallMise(m.ctx, &output); err != nil {
						return OperationFailedMsg{fmt.Errorf("%w\n%s", err, strings.TrimSpace(output.String()))}
					}
					return MiseInstalledMsg{}
				}
			}
		}
	}
	var command tea.Cmd
	switch m.screen {
	case listScreen:
		m.list, command = m.list.Update(msg)
	case createScreen:
		m.inputs[m.focus], command = m.inputs[m.focus].Update(msg)
	}
	return m, command
}

func (m Model) ShortHelp() []key.Binding {
	switch m.screen {
	case listScreen:
		return []key.Binding{key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "create")), key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "details")), key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "activate")), key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")), key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")), key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")), key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit"))}
	case createScreen:
		return []key.Binding{key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next field")), key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "create")), key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))}
	case detailsScreen:
		return []key.Binding{key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "activate")), key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")), key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))}
	default:
		return []key.Binding{key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "confirm")), key.NewBinding(key.WithKeys("n"), key.WithHelp("n / esc", "cancel"))}
	}
}
func (m Model) FullHelp() [][]key.Binding { return [][]key.Binding{m.ShortHelp()} }
