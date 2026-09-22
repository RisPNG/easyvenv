#!/bin/sh
set -eu

bin_dir="${HOME}/.local/bin"
shell_name="${SHELL:-}"
shell_name="${shell_name##*/}"
configure_profile=true
while [ "$#" -gt 0 ]; do
  case "$1" in
    --bin-dir) bin_dir="$2"; shift 2 ;;
    --shell) shell_name="$2"; shift 2 ;;
    --no-profile) configure_profile=false; shift ;;
    *) echo "Usage: $0 [--bin-dir /absolute/path] [--shell bash|zsh|fish|nu] [--no-profile]" >&2; exit 1 ;;
  esac
done
case "$bin_dir" in /*) ;; *) echo '--bin-dir must be absolute' >&2; exit 1 ;; esac
if [ "$configure_profile" = true ]; then
  case "$shell_name" in
    bash|zsh|fish|nu) ;;
    *) echo 'Could not detect a supported shell from SHELL. Use --shell bash|zsh|fish|nu, or --no-profile to install only the binaries.' >&2; exit 1 ;;
  esac
fi
source_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
mkdir -p "$bin_dir"
cd "$source_dir"
if command -v mise >/dev/null 2>&1; then
  mise exec -- go build -trimpath -o "$bin_dir/easyvenv" ./cmd/easyvenv
else
  go build -trimpath -o "$bin_dir/easyvenv" ./cmd/easyvenv
fi
cp "$bin_dir/easyvenv" "$bin_dir/venv"
if [ "$configure_profile" = true ]; then
  config_home="${XDG_CONFIG_HOME:-$HOME/.config}"
  integration_dir="$config_home/easyvenv"
  mkdir -p "$integration_dir"
  integration="$integration_dir/init.$shell_name"
  "$bin_dir/easyvenv" init "$shell_name" > "$integration"
  case "$shell_name" in
    bash) profile="$HOME/.bashrc" ;;
    zsh) profile="${ZDOTDIR:-$HOME}/.zshrc" ;;
    fish) profile="$config_home/fish/config.fish" ;;
    nu) profile="$config_home/nushell/config.nu" ;;
  esac
  mkdir -p "$(dirname -- "$profile")"
  case "$integration" in *"'"*) echo "Source $integration in $profile (path contains a quote)." >&2; exit 1 ;; esac
  line="source '$integration' # easyvenv shell integration"
  if ! [ -f "$profile" ] || ! grep -Fqx "$line" "$profile"; then
    printf '\n%s\n' "$line" >> "$profile"
  fi
  printf 'Installed easyvenv and venv in %s. Restart your shell to load %s.\n' "$bin_dir" "$integration"
else
  printf 'Installed easyvenv and venv in %s. Add it to PATH and configure shell integration as described in README.md.\n' "$bin_dir"
fi
