package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/RisPNG/easyvenv/internal/environment"
)

func TestSwitchAndRestoreShellState(t *testing.T) {
	t.Setenv(stateKey, "")
	original := map[string]string{"PATH": "/one:/two", "VIRTUAL_ENV": "/external", "VIRTUAL_ENV_PROMPT": "external", "PYTHONHOME": "/custom"}
	for key, value := range original {
		t.Setenv(key, value)
	}
	first := &environment.Environment{Name: "first", Path: "/envs/first", Bin: "/envs/first/bin"}
	delta, err := Delta(first)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range delta {
		if value != nil {
			t.Setenv(key, *value)
		} else {
			t.Setenv(key, "")
			os.Unsetenv(key)
		}
	}
	second := &environment.Environment{Name: "second", Path: "/envs/second", Bin: "/envs/second/bin"}
	delta, err = Delta(second)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(*delta["PATH"], first.Bin) {
		t.Fatal("previous environment remains in PATH")
	}
	for key, value := range delta {
		if value != nil {
			t.Setenv(key, *value)
		} else {
			os.Unsetenv(key)
		}
	}
	delta, err = Delta(nil)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range original {
		if delta[key] == nil || *delta[key] != want {
			t.Errorf("%s: got %v want %q", key, delta[key], want)
		}
	}
	if delta[stateKey] != nil {
		t.Fatal("saved state not cleared")
	}
}

func TestUnsetVariablesRemainUnset(t *testing.T) {
	t.Setenv(stateKey, "")
	for _, key := range []string{"VIRTUAL_ENV", "VIRTUAL_ENV_PROMPT", "PYTHONHOME"} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
	delta, err := Delta(&environment.Environment{Name: "a", Path: "/a", Bin: "/a/bin"})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(stateKey, *delta[stateKey])
	delta, err = Delta(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"VIRTUAL_ENV", "VIRTUAL_ENV_PROMPT", "PYTHONHOME"} {
		if delta[key] != nil {
			t.Errorf("%s should be unset", key)
		}
	}
}

func TestBashQuotingDoesNotExecuteEnvironmentNames(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash test")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip(err)
	}
	value := filepath.Join(t.TempDir(), "a'$(touch INJECTED)`touch INJECTED` with spaces")
	code, err := Render("bash", State{"VIRTUAL_ENV": &value})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bash, "--noprofile", "--norc", "-c", code+`printf '%s' "$VIRTUAL_ENV"`)
	cmd.Dir = t.TempDir()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v %s", err, output)
	}
	if string(output) != value {
		t.Fatalf("got %q want %q", output, value)
	}
	if _, err := os.Stat(filepath.Join(cmd.Dir, "INJECTED")); !os.IsNotExist(err) {
		t.Fatal("shell injection")
	}
}

func TestFailedShellCommandIsNotEvaluated(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash test")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip(err)
	}
	fake := filepath.Join(t.TempDir(), "fake easyvenv")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nprintf 'touch INJECTED\\n'\nexit 7\n"), 0755); err != nil {
		t.Fatal(err)
	}
	code, err := Integration("bash", fake)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bash, "--noprofile", "--norc", "-c", code+"\nvenv use example\nexit $?\n")
	cmd.Dir = t.TempDir()
	err = cmd.Run()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 7 {
		t.Fatalf("status: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cmd.Dir, "INJECTED")); !os.IsNotExist(err) {
		t.Fatal("evaluated output of failed command")
	}
}

func TestShellFormats(t *testing.T) {
	value := "value'with\\quotes"
	for _, name := range []string{"bash", "zsh", "fish", "pwsh", "powershell", "nu", "nushell"} {
		if runtime.GOOS == "windows" && (name == "bash" || name == "zsh" || name == "fish") {
			continue
		}
		if _, err := Integration(name, "/path with spaces/easyvenv"); err != nil {
			t.Fatal(err)
		}
		code, err := Render(name, State{"VIRTUAL_ENV": &value, "PYTHONHOME": nil})
		if err != nil || code == "" {
			t.Fatalf("%s: %s %v", name, code, err)
		}
	}
	if _, err := Integration("unknown", "easyvenv"); err == nil {
		t.Fatal("accepted unsupported shell")
	}
}
