package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/muesli/cancelreader"
	"golang.org/x/term"

	"github.com/RisPNG/easyvenv/internal/environment"
	"github.com/RisPNG/easyvenv/internal/provision"
	"github.com/RisPNG/easyvenv/internal/shell"
	"github.com/RisPNG/easyvenv/internal/tui"
)

const usage = `easyvenv — named Python environments, from any directory

Usage:
  venv                              Open the interactive environment manager
  venv create <name> [version]       Create using PATH Python, or mise for a version
  venv use <name> [--yes]            Activate; offer to create if missing
  venv deactivate                   Restore the previous shell environment
  venv list [--json]                 List environments
  venv inspect <name> [--json]       Show environment details
  venv delete <name> [--yes]         Delete an inactive environment
  venv run <name> <command> [args]   Run a command inside an environment
  easyvenv activate <shell>          Print shell integration (also: init)
  easyvenv version                   Show the installed version

Aliases: create/make/mk, use/activate, deactivate/exit/quit/q,
         list/ls, inspect/show, delete/del/remove/rm

Shells: bash, zsh, fish, pwsh, nu. See README for setup.
Storage: WORKON_HOME, or ~/.venvs. No version means no mise.
Use --yes for unattended create/use/delete (including mise installation).
`

type App struct {
	Service     *environment.Service
	Input       *os.File
	Output      io.Writer
	Error       io.Writer
	Interactive bool
}

func Run(ctx context.Context, args []string, input *os.File, output, stderr *os.File, version string) error {
	if len(args) > 0 {
		switch args[0] {
		case "help", "--help", "-h":
			fmt.Fprint(output, usage)
			return nil
		case "version", "--version":
			fmt.Fprintln(output, "easyvenv", version)
			return nil
		case "init", "activate":
			if len(args) == 2 && (args[0] == "init" || strings.Contains("|bash|zsh|fish|pwsh|powershell|nu|nushell|", "|"+args[1]+"|")) {
				binary, err := os.Executable()
				if err != nil {
					return err
				}
				code, err := shell.Integration(args[1], binary)
				if err != nil {
					return err
				}
				_, err = fmt.Fprint(output, code)
				return err
			}
		}
	}
	service, err := environment.New(&provision.Mise{})
	if err != nil {
		return err
	}
	app := &App{Service: service, Input: input, Output: output, Error: stderr, Interactive: term.IsTerminal(int(input.Fd()))}
	if len(args) > 0 && args[0] == "shell" {
		if len(args) < 2 {
			return errors.New("usage: easyvenv shell <shell> [use <name>|deactivate]")
		}
		if _, err := shell.Integration(args[1], "easyvenv"); err != nil {
			return err
		}
		return app.ApplyShell(ctx, args[1], args[2:])
	}
	if len(args) == 0 {
		if !app.Interactive || !term.IsTerminal(int(output.Fd())) {
			fmt.Fprint(output, usage)
			return nil
		}
		name, err := tui.Run(ctx, service, input, stderr)
		if err == nil && name != "" {
			fmt.Fprintf(output, "Activate %q with: venv use %q (requires shell integration)\n", name, name)
		}
		return err
	}
	return app.Execute(ctx, args)
}

func (a *App) Confirm(ctx context.Context, question string, yes bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if yes {
		return nil
	}
	if !a.Interactive {
		return errors.New(question + ": confirmation requires a terminal; pass --yes to approve")
	}
	reader, err := cancelreader.NewReader(a.Input)
	if err != nil {
		return err
	}
	defer reader.Close()
	stop := context.AfterFunc(ctx, func() { reader.Cancel() })
	defer stop()
	fmt.Fprint(a.Error, question, " [y/N]: ")
	answer, err := bufio.NewReader(reader).ReadString('\n')
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" && answer != "yes" {
		return errors.New("cancelled")
	}
	return nil
}

func (a *App) Create(ctx context.Context, name, requested string, yes bool) (environment.Environment, error) {
	path, err := a.Service.EnvironmentPath(name)
	if err != nil {
		return environment.Environment{}, err
	}
	if _, err := os.Lstat(path); err == nil {
		return environment.Environment{}, fmt.Errorf("environment %q already exists", name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return environment.Environment{}, err
	}
	python, err := a.Service.ResolvePython(ctx, requested)
	if errors.Is(err, environment.ErrMiseMissing) {
		if err = a.Confirm(ctx, "Install mise to provision the requested Python version?", yes); err != nil {
			return environment.Environment{}, err
		}
		if err = a.Service.Provisioner.InstallMise(ctx, a.Error); err != nil {
			return environment.Environment{}, err
		}
		python, err = a.Service.ResolvePython(ctx, requested)
	}
	if err != nil {
		return environment.Environment{}, err
	}
	if python.Source == "mise" {
		fmt.Fprintf(a.Error, "Preparing Python %s through mise...\n", python.Version)
		if err := a.Service.Provisioner.InstallVersion(ctx, python.Version, a.Error); err != nil {
			return environment.Environment{}, fmt.Errorf("install Python: %w", err)
		}
	}
	fmt.Fprintf(a.Error, "Creating %s with Python %s...\n", name, python.Version)
	return a.Service.Create(ctx, name, python, a.Error)
}

func (a *App) Execute(ctx context.Context, args []string) error {
	command := args[0]
	if command == "run" {
		if len(args) < 3 {
			return errors.New("usage: venv run <name> <command> [arguments...]")
		}
		rest := args[2:]
		if rest[0] == "--" {
			rest = rest[1:]
		}
		return a.Service.Run(ctx, args[1], rest, a.Input, a.Output, a.Error)
	}
	positional := []string{}
	yes, asJSON := false, false
	for _, arg := range args[1:] {
		switch arg {
		case "--yes", "-y":
			yes = true
		case "--json":
			asJSON = true
		case "--help", "-h":
			fmt.Fprint(a.Output, usage)
			return nil
		default:
			if strings.HasPrefix(arg, "-") {
				return fmt.Errorf("unknown option %q", arg)
			}
			positional = append(positional, arg)
		}
	}
	switch command {
	case "create", "make", "mk":
		if len(positional) < 1 || len(positional) > 2 || asJSON {
			return errors.New("usage: venv create <name> [version] [--yes]")
		}
		requested := ""
		if len(positional) == 2 {
			requested = positional[1]
		}
		env, err := a.Create(ctx, positional[0], requested, yes)
		if err != nil {
			return err
		}
		fmt.Fprintf(a.Output, "Created %s at %s\n", env.Name, env.Path)
	case "list", "ls":
		if len(positional) != 0 || yes {
			return errors.New("usage: venv list [--json]")
		}
		envs, err := a.Service.List()
		if err != nil {
			return err
		}
		if asJSON {
			return json.NewEncoder(a.Output).Encode(envs)
		}
		if len(envs) == 0 {
			fmt.Fprintln(a.Output, "No environments found.")
			return nil
		}
		table := tabwriter.NewWriter(a.Output, 0, 4, 2, ' ', 0)
		fmt.Fprintln(table, "NAME\tPYTHON\tSOURCE\tSTATUS")
		for _, env := range envs {
			active := ""
			if env.Active {
				active = "active"
			}
			source := env.Source
			if source == "" {
				source = "external"
			}
			fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", env.Name, env.Version, source, active)
		}
		return table.Flush()
	case "inspect", "show":
		if len(positional) != 1 || yes {
			return errors.New("usage: venv inspect <name> [--json]")
		}
		env, err := a.Service.Inspect(positional[0])
		if err != nil {
			return err
		}
		if asJSON {
			return json.NewEncoder(a.Output).Encode(env)
		}
		fmt.Fprintf(a.Output, "Name: %s\nPath: %s\nPython: %s\nVersion: %s\nActive: %t\n", env.Name, env.Path, env.Python, env.Version, env.Active)
		if env.Source != "" {
			fmt.Fprintf(a.Output, "Source: %s\n", env.Source)
		}
		if env.Requested != "" {
			fmt.Fprintf(a.Output, "Requested: %s\n", env.Requested)
		}
		if !env.CreatedAt.IsZero() {
			fmt.Fprintf(a.Output, "Created: %s\n", env.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
		}
	case "delete", "del", "remove", "rm":
		if len(positional) != 1 || asJSON {
			return errors.New("usage: venv delete <name> [--yes]")
		}
		env, err := a.Service.Inspect(positional[0])
		if err != nil {
			return err
		}
		if env.Active {
			return fmt.Errorf("%s is active; deactivate it before deleting", env.Name)
		}
		if err := a.Confirm(ctx, fmt.Sprintf("Delete environment %q at %s?", env.Name, env.Path), yes); err != nil {
			return err
		}
		if err := a.Service.Delete(env.Name); err != nil {
			return err
		}
		fmt.Fprintf(a.Output, "Deleted %s\n", env.Name)
	case "use", "activate", "deactivate", "exit", "quit", "q":
		return errors.New("activation needs shell integration; load the output of easyvenv init <shell> as described in README (supported shells: bash, zsh, fish, pwsh, nu)")
	default:
		return fmt.Errorf("unknown command %q; run venv --help", command)
	}
	return nil
}

func (a *App) ApplyShell(ctx context.Context, name string, args []string) error {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			fmt.Fprint(a.Error, usage)
			code, err := shell.Render(name, shell.State{})
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(a.Output, code)
			return err
		}
	}
	var env *environment.Environment
	if len(args) == 0 {
		if !a.Interactive {
			fmt.Fprint(a.Error, usage)
		} else {
			selected, err := tui.Run(ctx, a.Service, a.Input, a.Error)
			if err != nil {
				return err
			}
			if selected != "" {
				found, err := a.Service.Inspect(selected)
				if err != nil {
					return err
				}
				env = &found
			}
		}
		if env == nil {
			code, err := shell.Render(name, shell.State{})
			if err != nil {
				return err
			}
			fmt.Fprint(a.Output, code)
			return nil
		}
	} else {
		switch args[0] {
		case "use", "activate":
			positional := []string{}
			yes := false
			for _, arg := range args[1:] {
				if arg == "--yes" || arg == "-y" {
					yes = true
				} else {
					positional = append(positional, arg)
				}
			}
			if len(positional) != 1 {
				return errors.New("usage: venv use <name> [--yes]")
			}
			found, err := a.Service.Inspect(positional[0])
			if errors.Is(err, environment.ErrNotFound) {
				envs, listErr := a.Service.List()
				if listErr != nil {
					return listErr
				}
				if len(envs) > 0 {
					fmt.Fprintln(a.Error, "Available environments:")
					for _, item := range envs {
						fmt.Fprintln(a.Error, "  "+item.Name)
					}
				}
				if err := a.Confirm(ctx, fmt.Sprintf("Create and activate %q?", positional[0]), yes); err != nil {
					return err
				}
				found, err = a.Create(ctx, positional[0], "", yes)
			}
			if err != nil {
				return err
			}
			env = &found
		case "deactivate", "exit", "quit", "q":
			if len(args) != 1 {
				return errors.New("usage: venv deactivate")
			}
		default:
			return errors.New("shell command must be use or deactivate")
		}
	}
	delta, err := shell.Delta(env)
	if err != nil {
		return err
	}
	code, err := shell.Render(name, delta)
	if err != nil {
		return err
	}
	if env != nil {
		fmt.Fprintf(a.Error, "Activated %s\n", env.Name)
	} else if len(delta) > 0 {
		fmt.Fprintln(a.Error, "Deactivated environment")
	} else {
		fmt.Fprintln(a.Error, "No easyvenv environment is active.")
	}
	_, err = fmt.Fprint(a.Output, code)
	return err
}
