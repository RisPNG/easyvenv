package tui

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/RisPNG/easyvenv/internal/environment"
)

func TestCreationStateTransitions(t *testing.T) {
	m := New(context.Background(), &environment.Service{Root: t.TempDir()})
	defer m.cancel()
	m.screen = createScreen
	m.busy = ""
	m.inputs[0].SetValue("project")
	next, command := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if m.busy != "Resolving Python" || command == nil {
		t.Fatal("creation did not schedule resolution")
	}
	next, command = m.Update(PythonResolvedMsg{environment.Interpreter{Source: "path", Version: "3.11.14"}})
	m = next.(Model)
	if m.busy != "Creating project" || command == nil {
		t.Fatal("resolution did not schedule creation")
	}
	next, command = m.Update(EnvironmentCreatedMsg{environment.Environment{Name: "project"}})
	m = next.(Model)
	if m.screen != listScreen || command == nil {
		t.Fatal("creation did not refresh list")
	}
	next, _ = m.Update(command())
	m = next.(Model)
	if m.busy != "" {
		t.Fatal("refresh did not finish")
	}
}

func TestMiseRequiresConfirmation(t *testing.T) {
	m := New(context.Background(), &environment.Service{Root: t.TempDir()})
	defer m.cancel()
	m.screen, m.busy = createScreen, "Resolving Python"
	next, command := m.Update(OperationFailedMsg{environment.ErrMiseMissing})
	m = next.(Model)
	if m.screen != miseScreen || m.busy != "" || command != nil {
		t.Fatal("missing mise did not request confirmation")
	}
	next, command = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = next.(Model)
	if m.screen != createScreen || command != nil {
		t.Fatal("decline should return to form")
	}
}

func TestCancellationWaitsForCleanup(t *testing.T) {
	m := New(context.Background(), &environment.Service{Root: t.TempDir()})
	defer m.cancel()
	m.busy = "Creating project"
	next, command := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = next.(Model)
	if !m.quitting || command != nil || !errors.Is(m.ctx.Err(), context.Canceled) {
		t.Fatal("cancel must wait for operation result")
	}
	_, command = m.Update(OperationFailedMsg{context.Canceled})
	if command == nil {
		t.Fatal("cleanup completion should quit")
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatal("expected quit")
	}
}

func TestViewsFitSmallTerminalAndShowErrors(t *testing.T) {
	m := New(context.Background(), &environment.Service{Root: t.TempDir()})
	defer m.cancel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 30, Height: 12})
	m = next.(Model)
	for _, screen := range []screen{listScreen, createScreen, detailsScreen, deleteScreen, miseScreen} {
		m.screen = screen
		_ = m.View()
	}
	next, _ = m.Update(OperationFailedMsg{errors.New("creation failed")})
	m = next.(Model)
	if m.failure == nil || m.busy != "" {
		t.Fatal("error missing")
	}
}
