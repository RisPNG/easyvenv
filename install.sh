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
  config_home="${XDG_CONFIG_HOME:-$HOME/.config}"
  integration_dir="$config_home/easyvenv"
  integration="$integration_dir/init.$shell_name"
  case "$bin_dir$integration" in *"'"*) echo 'Installation paths containing a quote require --no-profile and manual shell setup.' >&2; exit 1 ;; esac
fi

case "$(uname -s)" in
  Linux) platform=linux ;;
  Darwin) platform=darwin ;;
  *) echo 'easyvenv supports Linux and macOS with this installer.' >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) architecture=amd64 ;;
  aarch64|arm64) architecture=arm64 ;;
  *) echo 'easyvenv supports x86_64 and arm64 with this installer.' >&2; exit 1 ;;
esac

asset="easyvenv-$platform-$architecture.tar.gz"
release_url="https://github.com/RisPNG/easyvenv/releases/latest/download"
temporary_dir=$(mktemp -d)
trap 'rm -rf "$temporary_dir"' 0
curl -fsSL "$release_url/$asset" -o "$temporary_dir/$asset"
curl -fsSL "$release_url/checksums.txt" -o "$temporary_dir/checksums.txt"
expected=$(awk -v name="$asset" '$2 == name { print $1 }' "$temporary_dir/checksums.txt")
if [ -z "$expected" ]; then
  echo "No checksum was published for $asset." >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$temporary_dir/$asset" | awk '{ print $1 }')
else
  actual=$(shasum -a 256 "$temporary_dir/$asset" | awk '{ print $1 }')
fi
if [ "$actual" != "$expected" ]; then
  echo "Checksum verification failed for $asset." >&2
  exit 1
fi
tar -xzf "$temporary_dir/$asset" -C "$temporary_dir"
mkdir -p "$bin_dir"
install -m 755 "$temporary_dir/easyvenv" "$bin_dir/easyvenv"
install -m 755 "$temporary_dir/venv" "$bin_dir/venv"

if [ "$configure_profile" = true ]; then
  mkdir -p "$integration_dir"
  "$bin_dir/easyvenv" init "$shell_name" > "$integration"
  case "$shell_name" in
    bash) profile="$HOME/.bashrc" ;;
    zsh) profile="${ZDOTDIR:-$HOME}/.zshrc" ;;
    fish) profile="$config_home/fish/config.fish" ;;
    nu) profile=$(nu -c '$nu.config-path') ;;
  esac
  mkdir -p "$(dirname -- "$profile")"
  case "$shell_name" in
    bash|zsh) path_line="export PATH='$bin_dir':\$PATH # easyvenv PATH" ;;
    fish) path_line="fish_add_path --path '$bin_dir' # easyvenv PATH" ;;
    nu) path_line="\$env.PATH = (\$env.PATH | prepend '$bin_dir') # easyvenv PATH" ;;
  esac
  line="source '$integration' # easyvenv shell integration"
  for entry in "$path_line" "$line"; do
    if ! [ -f "$profile" ] || ! grep -Fqx "$entry" "$profile"; then
      printf '\n%s\n' "$entry" >> "$profile"
    fi
  done
  printf 'Installed easyvenv and venv in %s. Restart your shell to load %s.\n' "$bin_dir" "$integration"
else
  printf 'Installed easyvenv and venv in %s. Add it to PATH and configure shell integration: https://github.com/RisPNG/easyvenv#manual-shell-setup\n' "$bin_dir"
fi
