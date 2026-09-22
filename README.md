# easyvenv

Create and switch between named Python environments from any directory.

```sh
venv create myproject
venv use myproject
python app.py
venv deactivate
```

Keep your environments in one place, choose a Python version when you need one, and use either terminal commands or an interactive menu.

## Install

You need Go 1.27.1 or newer to run the installer. To create environments, you can use Python already installed on your computer or let easyvenv set up a version through mise.

Download this repository, or clone it:

```sh
git clone https://github.com/RisPNG/easyvenv.git
cd easyvenv
```

### Linux and macOS

```sh
./install.sh
```

The installer detects your login shell from `$SHELL`, installs into `~/.local/bin`, and adds shell setup to your startup file. **Open a new terminal after installation**, then run `venv`.

Bash, Zsh, Fish and Nushell are supported. You can override detection or change the installation location:

```sh
./install.sh --shell zsh
./install.sh --bin-dir /your/bin/directory
```

Use `--no-profile` if you want to configure your shell yourself.

### Windows / PowerShell

```powershell
./install.ps1
```

The installer installs into `$HOME/.local/bin` and adds shell setup to your PowerShell profile. **Restart PowerShell**, then run `venv`.

Use `-BinDir` to change the installation location or `-NoProfile` to configure your shell yourself. PowerShell and Nushell support native Windows environments. If you use Bash, Zsh or Fish inside WSL, follow the Linux instructions inside WSL.

If you already use mise to manage Go, prepare the repository's toolchain before running either installer:

```sh
mise trust
mise install
```

The installers also provide an `easyvenv` command for shell setup. Add the installation directory to your `PATH` to use it, or to use `venv` in scripts. For Bash and Zsh with the default location, add this to your startup file:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

## Everyday use

### Create an environment

```sh
venv create myproject
```

This uses your current Python: `python`, or `python3` if `python` is unavailable. Your existing Python setup is respected, whether it comes from your system, Homebrew, pyenv, asdf, uv or mise.

To choose a version:

```sh
venv create legacy 3.11.14
```

Supplying a version tells easyvenv to use mise. If mise is missing, easyvenv asks before installing it, then installs the requested Python version if needed. Installing easyvenv itself does not install mise.

You can also request `3`, `3.11` or `latest`. These prefer a matching Python version already installed through mise; otherwise, mise finds an available version.

### Activate or switch environments

```sh
venv use myproject
venv use legacy
venv deactivate
```

`use` switches your current terminal to that environment. If the name does not exist, easyvenv offers to create it using your current Python. `deactivate` restores your previous environment.

Your prompt's appearance depends on your shell theme. Use `venv list` to see which environment is active.

### Run a command without activating

```sh
venv run myproject python app.py
venv run myproject python -m pip install requests
```

The command runs from your current working directory using the selected environment. easyvenv manages environments; continue using pip or your preferred Python tooling to install packages.

### List, inspect and delete

```sh
venv list
venv inspect myproject
venv delete oldproject
```

Deletion asks for confirmation. Deactivate an environment before deleting it, and make sure you are finished using it in other terminals too. Deleting an environment removes its installed packages, but keeps the Python version provisioned through mise.

## Interactive menu

Run `venv` without arguments to browse your environments. Use the arrow keys to move and `/` to search.

| Key | Action |
| --- | --- |
| `n` | Create an environment |
| Enter | View details |
| `a` | Activate the selected environment and close the menu |
| `d` | Delete the selected environment after confirmation |
| `r` | Refresh the list |
| `q` | Quit |
| Esc | Go back |
| Ctrl+C | Cancel the current operation and quit |

In the create form, Tab moves between fields and Enter creates the environment. Leave the Python version blank to use your current Python.

## Command reference

| Command | What it does |
| --- | --- |
| `venv` | Open the interactive menu; print help when used without a terminal |
| `venv create <name> [version]` | Create an environment |
| `venv use <name>` | Activate or switch environments |
| `venv deactivate` | Return to your previous environment |
| `venv list [--json]` | List environments |
| `venv inspect <name> [--json]` | Show an environment's location, Python version and creation details |
| `venv delete <name>` | Delete an environment |
| `venv run <name> <command> [args...]` | Run a command in an environment |
| `venv --help` | Show help |
| `venv --version` | Show the installed version |

Shortcuts are available: `make` or `mk` for `create`; `activate` for `use`; `exit`, `quit` or `q` for `deactivate`; `ls` for `list`; `show` for `inspect`; and `del`, `remove` or `rm` for `delete`.

For scripts, use `--yes` with `create`, `use` or `delete` to approve any confirmation. This also approves installing mise when a requested Python version needs it.

```sh
venv create legacy 3.11.14 --yes
venv list --json
venv rm oldproject --yes
```

## Where environments are stored

Environments live in `~/.venvs`, so `venv use myproject` works whether you are in a project folder, your home directory or somewhere else.

To use another location, set `WORKON_HOME` to an absolute path in your shell's startup file. For example, in Bash or Zsh:

```sh
export WORKON_HOME="$HOME/python-environments"
```

Changing this setting selects a different storage location; it does not move existing environments. Keep environments at the paths where they were created.

Use simple names such as `myproject` or `data-science`. Spaces and Unicode are supported; quote names containing spaces. Names cannot contain path separators, reserved filename characters, or start with a dash or `.easyvenv`.

## Manual shell setup

Skip this section if the installer already configured your shell. Otherwise, ensure `easyvenv` is on `PATH`, then add the appropriate line to your startup file:

| Shell | Startup file | Setup |
| --- | --- | --- |
| Bash | `~/.bashrc` | `eval "$(easyvenv activate bash)"` |
| Zsh | `~/.zshrc`, or the file under `ZDOTDIR` | `eval "$(easyvenv activate zsh)"` |
| Fish | `~/.config/fish/config.fish` | `easyvenv activate fish \| source` |
| PowerShell | `$PROFILE` | `(& easyvenv activate pwsh) \| Out-String \| Invoke-Expression` |

For Nushell, save the output of `easyvenv activate nu` to a file and add a `source` line for that file to your `config.nu`. Run `$nu.config-path` to find your configuration file.

Restart your terminal afterward. `easyvenv init <shell>` is an alternative spelling for these setup commands.

## Troubleshooting

- **`venv` is not found:** open a new terminal after installation. For scripts or manual setup, check that the installation directory is on `PATH`.
- **Activation asks for shell integration:** follow the manual setup above. Activation from the interactive menu also needs this setup.
- **An older `venv` command still runs:** remove or rename any existing `venv` alias or function in your shell configuration, then restart the terminal.
- **Python is not found:** install/select Python first, or supply a version such as `venv create myproject 3.11` to use mise.
- **Python reports a missing `venv` or `ensurepip` module:** install your distribution's Python virtual-environment support package, or create the environment with a version provisioned through mise.
- **A failed creation left a folder behind:** ordinary failures are cleaned up automatically. After a crash or forced termination, check `WORKON_HOME` for the affected folder. A folder containing `.easyvenv-creating` is unfinished; remove that partial folder before retrying.
