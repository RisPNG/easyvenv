package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/RisPNG/easyvenv/internal/environment"
)

func TestMain(m *testing.M) {
	if os.Getenv("EASYVENV_TEST_PROCESS") == "1" {
		err := Run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr, "test")
		if err != nil {
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				os.Exit(exit.ExitCode())
			}
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func cliCommand(t *testing.T, root string, args ...string) *exec.Cmd {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, args...)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if name != "WORKON_HOME" && name != "VIRTUAL_ENV" && name != "_EASYVENV_STATE" && name != "EASYVENV_TEST_PROCESS" {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "EASYVENV_TEST_PROCESS=1", "WORKON_HOME="+root)
	return cmd
}

func TestCLIEnvironmentWorkflow(t *testing.T) {
	root := filepath.Join(t.TempDir(), "environments")
	output, err := cliCommand(t, root, "mk", "first").CombinedOutput()
	if err != nil {
		t.Fatalf("create: %v: %s", err, output)
	}
	output, err = cliCommand(t, root, "ls", "--json").Output()
	if err != nil {
		t.Fatal(err)
	}
	var envs []environment.Environment
	if err := json.Unmarshal(output, &envs); err != nil || len(envs) != 1 || envs[0].Name != "first" {
		t.Fatalf("list: %s %v", output, err)
	}
	output, err = cliCommand(t, root, "show", "first", "--json").Output()
	if err != nil {
		t.Fatal(err)
	}
	var env environment.Environment
	if err := json.Unmarshal(output, &env); err != nil || env.Path != filepath.Join(root, "first") {
		t.Fatalf("inspect: %s %v", output, err)
	}
	output, err = cliCommand(t, root, "run", "first", "python", "-c", `import sys; print(sys.argv[1]); sys.exit(23)`, "argument with spaces").CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 23 || strings.TrimSpace(string(output)) != "argument with spaces" {
		t.Fatalf("run: %v %s", err, output)
	}
	output, err = cliCommand(t, root, "rm", "first").CombinedOutput()
	if err == nil || !strings.Contains(string(output), "--yes") {
		t.Fatalf("unattended deletion: %v %s", err, output)
	}
	output, err = cliCommand(t, root, "remove", "first", "--yes").CombinedOutput()
	if err != nil {
		t.Fatalf("delete: %v %s", err, output)
	}
	output, err = cliCommand(t, root).CombinedOutput()
	if err != nil || !strings.Contains(string(output), "Usage:") {
		t.Fatalf("noninteractive help: %v %s", err, output)
	}
	output, err = cliCommand(t, root, "use", "first").CombinedOutput()
	if err == nil || !strings.Contains(string(output), "shell integration") {
		t.Fatalf("bare activation: %v %s", err, output)
	}
}

func TestShellWorkflow(t *testing.T) {
	root := filepath.Join(t.TempDir(), "envs with spaces ' and $ signs")
	output, err := cliCommand(t, root, "create", "first").CombinedOutput()
	if err != nil {
		t.Fatalf("create: %v %s", err, output)
	}
	for _, name := range []string{"bash", "zsh", "fish", "pwsh", "nu"} {
		t.Run(name, func(t *testing.T) {
			if runtime.GOOS == "windows" && (name == "bash" || name == "zsh" || name == "fish") {
				t.Skip("Unix shells use the Linux binary inside WSL")
			}
			path, err := exec.LookPath(name)
			if err != nil {
				t.Skip(name + " not installed")
			}
			binary, _ := os.Executable()
			var script string
			var args []string
			switch name {
			case "bash", "zsh":
				script = `set -e
original_path="$PATH"
eval "$("$BIN" init "$TEST_SHELL")"
venv use first
[ "$VIRTUAL_ENV" = "$WORKON_HOME/first" ]
python -c 'import sys,os; assert sys.prefix == os.environ["VIRTUAL_ENV"]'
cd "$OTHER_DIR"
venv use second --yes
[ "$VIRTUAL_ENV" = "$WORKON_HOME/second" ]
case "$PATH" in *"$WORKON_HOME/first/bin"*) exit 40 ;; esac
if venv use missing; then exit 41; fi
[ "$VIRTUAL_ENV" = "$WORKON_HOME/second" ]
if venv rm second --yes; then exit 42; fi
venv deactivate
[ "$PATH" = "$original_path" ]
[ "${VIRTUAL_ENV+x}" != x ]
venv q
venv rm second --yes
`
				args = []string{"-f", "-c", script}
				if name == "bash" {
					args = []string{"--noprofile", "--norc", "-c", script}
				}
			case "fish":
				script = `set original_path $PATH
$BIN init fish | source
venv use first; or exit 30
test "$VIRTUAL_ENV" = "$WORKON_HOME/first"; or exit 31
python -c 'import sys,os; assert sys.prefix == os.environ["VIRTUAL_ENV"]'; or exit 32
cd "$OTHER_DIR"
venv use second --yes; or exit 33
venv use missing; and exit 34
test "$VIRTUAL_ENV" = "$WORKON_HOME/second"; or exit 35
venv rm second --yes; and exit 36
venv deactivate; or exit 37
test (string join : $PATH) = (string join : $original_path); or exit 38
set -q VIRTUAL_ENV; and exit 39
venv rm second --yes; or exit 40
`
				args = []string{"--no-config", "-c", script}
			case "pwsh":
				script = `$ErrorActionPreference = 'Stop'
$originalPath = $env:PATH
(& $env:BIN init pwsh) | Out-String | Invoke-Expression
venv use first
if ($LASTEXITCODE -ne 0 -or $env:VIRTUAL_ENV -ne (Join-Path $env:WORKON_HOME first)) { exit 31 }
python -c 'import sys,os; assert sys.prefix == os.environ["VIRTUAL_ENV"]'
if ($LASTEXITCODE -ne 0) { exit 32 }
Set-Location $env:OTHER_DIR
venv use second --yes
if ($LASTEXITCODE -ne 0) { exit 33 }
venv use missing
if ($LASTEXITCODE -eq 0) { exit 34 }
venv rm second --yes
if ($LASTEXITCODE -eq 0) { exit 35 }
venv deactivate
if ($env:PATH -ne $originalPath -or (Test-Path Env:VIRTUAL_ENV)) { exit 36 }
venv rm second --yes
exit $LASTEXITCODE
`
				args = []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script}
			case "nu":
				init, err := cliCommand(t, root, "init", "nu").Output()
				if err != nil {
					t.Fatal(err)
				}
				script = string(init) + `
let original_path = $env.PATH
venv use first
if $env.VIRTUAL_ENV != ($env.WORKON_HOME | path join first) { error make {msg: 'activation failed'} }
python -c 'import sys,os; assert sys.prefix == os.environ["VIRTUAL_ENV"]'
cd $env.OTHER_DIR
venv use second --yes
venv deactivate
if $env.PATH != $original_path { error make {msg: 'PATH not restored'} }
if 'VIRTUAL_ENV' in $env { error make {msg: 'VIRTUAL_ENV not cleared'} }
venv rm second --yes
`
				args = []string{"--no-config-file", "-c", script}
			}
			cmd := exec.Command(path, args...)
			cmd.Env = cliCommand(t, root).Env
			cmd.Env = append(cmd.Env, "BIN="+binary, "TEST_SHELL="+name, "OTHER_DIR="+t.TempDir())
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%v\n%s", err, output)
			}
		})
	}
}

func TestInvalidArgumentsNeverCreateEnvironments(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{{"create"}, {"create", "../bad"}, {"create", "one", "3.11", "extra"}, {"create", "one", "--bogus"}, {"list", "one"}, {"run", "one"}, {"shell", "invalid", "use", "one"}, {"shell", "bash", "use", "one", "extra"}, {"shell", "bash", "deactivate", "extra"}} {
		if err := cliCommand(t, root, args...).Run(); err == nil {
			t.Errorf("accepted %q", args)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("unexpected files: %v %v", entries, err)
	}
}

func TestCommandFromAnotherDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("covered by shell workflow on Windows")
	}
	root := t.TempDir()
	cmd := cliCommand(t, root, "create", "elsewhere")
	cmd.Dir = t.TempDir()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v %s", err, output)
	}
	if _, err := os.Stat(filepath.Join(root, "elsewhere", "pyvenv.cfg")); err != nil {
		t.Fatal(err)
	}
}

func TestConfirmationCanBeCancelled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows cancellation needs a console handle")
	}
	input, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer writer.Close()
	app := App{Input: input, Error: io.Discard, Interactive: true}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := app.Confirm(ctx, "Continue?", false); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("confirmation cancellation: %v", err)
	}
}
