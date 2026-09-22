# easyvenv

Named Python virtual environments, available from any directory. Use `venv` to open a small terminal UI, or explicit commands for scripts and everyday terminal work.

```sh
venv create myproject           # Use the Python already selected on PATH
venv create legacy 3.11.14      # Resolve/install Python through mise
venv use myproject              # Activate in this terminal
venv run legacy python app.py   # Run without activating
venv deactivate                # Restore the previous shell environment
```

Environments live in `~/.venvs`, or the absolute directory set by `WORKON_HOME`. Your working directory does not affect where environments are stored. Python environments stay at their original locations; “portable” here means the command works from any directory.

## Installation

Build from this repository with Go 1.27.1 or the pinned mise toolchain. The installer installs both `easyvenv` and `venv`. It does **not** install mise or Python.

### Linux and macOS

```sh
# If using mise for development:
mise trust
mise install

# Install and enable the venv function in your shell:
./install.sh
# Optional override: --shell bash, --shell zsh, --shell fish, --shell nu
```

The installer detects your login shell from `SHELL` (Bash, Zsh, Fish or Nushell). Use `--shell` to choose a different shell. If detection is unavailable or the shell is unsupported, it asks you to supply an override before installing anything.

The default destination is `~/.local/bin`. Use `--bin-dir /absolute/path` to change it. The installer writes an integration file under `${XDG_CONFIG_HOME:-~/.config}/easyvenv` and adds an idempotent source line to the selected shell's startup file. Start a new terminal afterward. It respects `ZDOTDIR` for Zsh.

Use `--no-profile` to install only the binaries and configure activation manually below. Add the destination to `PATH` for direct binary use and scripts. For Bash/Zsh, a typical PATH setting is:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

### Windows / PowerShell

From the repository, with Go available:

```powershell
./install.ps1
```

This installs into `$HOME/.local/bin` and adds integration to `$PROFILE`. Restart PowerShell. Use `-BinDir` to change the destination or `-NoProfile` to skip profile changes. Add the destination to your user PATH for direct binary use and scripts.

### Manual shell integration

Bash, Zsh and Fish use the Linux/macOS binary. PowerShell and Nushell also support the native Windows binary. For Unix shells inside WSL, install easyvenv inside WSL.

The binary cannot change its parent shell. These commands define a lightweight `venv` function; run one now and add it to your shell's startup file to keep it.

**Bash** (`~/.bashrc`):

```sh
eval "$(easyvenv activate bash)"
```

**Zsh** (`~/.zshrc`):

```sh
eval "$(easyvenv activate zsh)"
```

**Fish** (`~/.config/fish/config.fish`):

```fish
easyvenv activate fish | source
```

**PowerShell** (`$PROFILE`):

```powershell
(& easyvenv activate pwsh) | Out-String | Invoke-Expression
```

**Nushell** (its `config.nu`; find the location with `$nu.config-path`):

```nu
easyvenv activate nu | save --force ~/.config/nushell/easyvenv.nu
# Add this line to config.nu:
source ~/.config/nushell/easyvenv.nu
```

`easyvenv init <shell>` is equivalent to `easyvenv activate <shell>`. Integration embeds the binary's absolute path, so the `venv` function does not depend on your current directory or on Python changing PATH. Reload integration after moving the binary. When replacing the Bash prototype, remove its old `venv()` definition or load easyvenv's integration after it.

Activation sets `VIRTUAL_ENV` and `VIRTUAL_ENV_PROMPT`, prepends the environment's executable directory to `PATH`, and unsets `PYTHONHOME`. Switching replaces the previous easyvenv environment. Deactivation restores the original values, including whether each variable was unset. If you entered easyvenv from another activated environment, deactivation returns to that environment. Prompt formatting is left to your existing shell/theme; themes can use `VIRTUAL_ENV` or `VIRTUAL_ENV_PROMPT`.

## Commands

| Command | Behavior |
| --- | --- |
| `venv` | Open the TUI when attached to an interactive terminal; otherwise print help |
| `venv create <name> [version] [--yes]` | Create with current PATH Python, or mise for an explicit version |
| `venv use <name> [--yes]` | Activate; offer to create with current Python if missing |
| `venv deactivate` | Restore the shell state from before easyvenv activation |
| `venv list [--json]` | List complete environments and mark the current one |
| `venv inspect <name> [--json]` | Show interpreter, version, location, provenance and creation time when known |
| `venv delete <name> [--yes]` | Confirm and delete an inactive environment |
| `venv run <name> <command> [args...]` | Run with the environment's PATH and variables, preserving the working directory, streams and exit status |
| `easyvenv activate <shell>` | Print shell integration |
| `venv --help` / `venv --version` | Help / version |

Aliases match the prototype: `make`/`mk`, `activate`/`use`, `exit`/`quit`/`q`, `ls`, and `del`/`remove`/`rm`. `show` aliases `inspect`. The binary's `easyvenv activate bash` configures integration; the shell function's `venv activate bash` activates an environment named `bash`.

Names may contain spaces or Unicode, but must be a single filename without path separators, control characters, Windows-reserved names/characters, a leading dash, or easyvenv's reserved prefix. Quote names with spaces. `WORKON_HOME` must be absolute to keep behavior consistent across directories.

```sh
venv create 'data science'
venv run 'data science' python -c 'import sys; print(sys.executable)'
venv run myproject pytest -q
venv run myproject -- python script.py --some-flag
venv list --json
venv rm oldproject --yes
```

Destructive actions and implicit creation require confirmation. In noninteractive use, pass `--yes`; easyvenv never reads piped input as approval. `--yes` on creation also approves installing mise if it is missing.

## Python selection and mise

Without a version, easyvenv looks for `python`, then `python3`, on your current PATH, inspects that interpreter, and runs its `-m venv`. It never calls mise in this path. Existing selections from pyenv, asdf, Homebrew, uv, a system installation, or another venv are respected. Python must provide the standard `venv` and `ensurepip` modules; some Linux distributions package these separately.

An explicit version (`3`, `3.11`, `3.11.14`, or `latest`) uses mise. Exact versions are used directly. Partial versions prefer an installed match and otherwise resolve through mise's remote catalog. Python is installed if needed, then creation runs through `mise exec python@<resolved-version> -- python -m venv ...`. No global/project Python settings are changed.

If mise is missing, easyvenv offers to install it at that moment: the official `https://mise.run` installer on Linux/macOS, or `winget install --id jdx.mise` on Windows. Declining leaves the environment uncreated. Existing mise on PATH or at `~/.local/bin/mise` is reused. Python installations belong to mise; deleting a virtual environment never uninstalls Python.

## Terminal UI

The UI uses Bubble Tea and Bubbles. Navigate with the arrow keys; `/` filters environments.

| Key | Action |
| --- | --- |
| `n` | Create form; Tab changes fields and Enter submits |
| Enter | Details |
| `a` | Activate the selection and close the UI |
| `d` | Delete confirmation; `y` confirms, `n`/Esc cancels |
| `r` | Refresh |
| `q` | Quit from the list |
| Esc | Return from a form/details/confirmation |
| Ctrl+C | Cancel a running operation and quit |

Activation from the UI works when launched through the shell's `venv` function. Running the binary directly prints an activation instruction after selection. Installation, resolution, creation, deletion and discovery run as background commands; the UI receives typed result messages.

## Storage and failure behavior

The filesystem is authoritative. Existing environments are discovered from `pyvenv.cfg` and their Python executable even if easyvenv's metadata is absent or damaged. `.easyvenv.json` records provenance, requested version and creation time; listing uses the version in `pyvenv.cfg`.

Creation runs directly at `WORKON_HOME/<name>`, just like the Bash prototype. An exclusive directory creation prevents overwriting existing files. A `.easyvenv-creating` marker keeps incomplete environments out of listing, activation and execution. Successful creation removes the marker; failed creation removes only the newly created directory.

A machine crash or forced kill can leave a marked directory. It stays hidden and cannot be overwritten. Inspect that directory and remove it manually before retrying. Symlinked environment directories are not followed or deleted. Deletion refuses the environment active in the invoking shell; it cannot detect activation in other terminals.

Package installation and dependency management remain with Python tooling (`pip`, `uv pip`, Poetry, PDM, etc.). easyvenv does not solve dependencies, manage lockfiles, or move environments between machines.

## Development

```sh
mise trust
mise install
mise exec -- go test ./...
mise exec -- go vet ./...
mise exec -- go test -race ./...
mise run build
```

The test suite creates disposable real Python environments, checks executable entry points, metadata-free discovery, failure cleanup, concurrent creation, mise argument handling, CLI behavior, and shell switching/restoration. Shell integration tests run for every supported shell available on PATH. CI covers Linux, macOS and Windows and builds amd64/arm64 binaries.

| Directory | Responsibility |
| --- | --- |
| `cmd/easyvenv` | Process entrypoint, cancellation and exit status |
| `internal/environment` | Interpreter selection and environment lifecycle; no UI dependency |
| `internal/provision` | mise discovery, resolution, installation and execution |
| `internal/process` | Background subprocess cancellation and descendant cleanup |
| `internal/shell` | Shell wrappers and environment deltas |
| `internal/cli` | Arguments, confirmations and command output |
| `internal/tui` | Bubble Tea model, view, typed messages and centralized theme |

The implementation follows the plan's small service/frontend boundary. Reference material included [Bubble Tea's package-manager example](https://github.com/charmbracelet/bubbletea/tree/main/examples/package-manager), [Crush's UI guidance](https://github.com/charmbracelet/crush/blob/main/internal/ui/AGENTS.md), [Superfile](https://github.com/yorukot/superfile), [mise activation](https://github.com/jdx/mise/blob/main/src/cli/activate.rs), [venv_manager](https://github.com/jacopobonomi/venv_manager), and [UVE](https://github.com/robert-mcdermott/uve). Their larger application architectures and package-management features are outside this tool's scope.
