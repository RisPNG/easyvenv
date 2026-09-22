package shell

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/RisPNG/easyvenv/internal/environment"
)

const stateKey = "_EASYVENV_STATE"

var variables = []string{"PATH", "VIRTUAL_ENV", "VIRTUAL_ENV_PROMPT", "PYTHONHOME"}

type State map[string]*string

func Integration(name, binary string) (string, error) {
	if runtime.GOOS == "windows" && (name == "bash" || name == "zsh" || name == "fish") {
		return "", errors.New("use PowerShell or Nushell with the Windows binary; for Bash, Zsh or Fish in WSL, install the Linux binary inside WSL")
	}
	switch name {
	case "bash", "zsh":
		quoted := "'" + strings.ReplaceAll(binary, "'", "'\"'\"'") + "'"
		return fmt.Sprintf(`venv() {
  case "${1-}" in
    ''|use|activate|deactivate|exit|quit|q)
      local easyvenv_code
      easyvenv_code="$(command %s shell %s "$@")" || return $?
      eval "$easyvenv_code"
      ;;
    *) command %s "$@" ;;
  esac
}
`, quoted, name, quoted), nil
	case "fish":
		quoted := "'" + strings.NewReplacer(`\`, `\\`, "'", `\'`).Replace(binary) + "'"
		return fmt.Sprintf(`function venv
  set -l easyvenv_action $argv[1]
  if test (count $argv) -eq 0; or contains -- "$easyvenv_action" use activate deactivate exit quit q
    set -l easyvenv_code (command %s shell fish $argv | string collect)
    set -l easyvenv_status $pipestatus[1]
    if test $easyvenv_status -ne 0
      return $easyvenv_status
    end
    if test -n "$easyvenv_code"
      eval "$easyvenv_code"
    end
  else
    command %s $argv
  end
end
`, quoted, quoted), nil
	case "pwsh", "powershell":
		quoted := "'" + strings.ReplaceAll(binary, "'", "''") + "'"
		return fmt.Sprintf(`function global:venv {
  if ($args.Count -eq 0 -or $args[0] -in @('use', 'activate', 'deactivate', 'exit', 'quit', 'q')) {
    $easyvenvCode = & %s shell pwsh @args
    $easyvenvStatus = $LASTEXITCODE
    if ($easyvenvStatus -ne 0) { $global:LASTEXITCODE = $easyvenvStatus; return }
    if ($easyvenvCode) { Invoke-Expression ($easyvenvCode -join "`+"`n"+`") }
    $global:LASTEXITCODE = 0
  } else {
    & %s @args
  }
}
`, quoted, quoted), nil
	case "nu", "nushell":
		delimiter := "#"
		for strings.Contains(binary, "'"+delimiter) {
			delimiter += "#"
		}
		quoted := "r" + delimiter + "'" + binary + "'" + delimiter
		return fmt.Sprintf(`def --env --wrapped venv [...args: string] {
  let action = ($args | get -o 0 | default '')
  if ($action in ['' use activate deactivate exit quit q]) {
    let code = (do --capture-errors { ^%s shell nu ...$args })
    let delta = ($code | from json)
    for entry in ($delta | transpose key value) {
      if $entry.value == null { hide-env -i $entry.key } else { load-env {($entry.key): $entry.value} }
    }
  } else {
    ^%s ...$args
  }
}
`, quoted, quoted), nil
	default:
		return "", fmt.Errorf("unsupported shell %q; use bash, zsh, fish, pwsh, or nu", name)
	}
}

func Delta(env *environment.Environment) (State, error) {
	baseline := State{}
	saved := os.Getenv(stateKey)
	if saved != "" {
		data, err := base64.RawURLEncoding.DecodeString(saved)
		if err != nil {
			return nil, fmt.Errorf("read saved shell state: %w", err)
		}
		if err := json.Unmarshal(data, &baseline); err != nil {
			return nil, fmt.Errorf("read saved shell state: %w", err)
		}
		for _, key := range variables {
			if _, ok := baseline[key]; !ok {
				return nil, errors.New("saved shell state is incomplete")
			}
		}
	} else {
		for _, key := range variables {
			if value, ok := os.LookupEnv(key); ok {
				baseline[key] = &value
			} else {
				baseline[key] = nil
			}
		}
	}
	delta := State{}
	if env == nil {
		if saved == "" {
			return delta, nil
		}
		for _, key := range variables {
			delta[key] = baseline[key]
		}
		delta[stateKey] = nil
		return delta, nil
	}
	if saved == "" {
		data, err := json.Marshal(baseline)
		if err != nil {
			return nil, err
		}
		saved = base64.RawURLEncoding.EncodeToString(data)
	}
	delta[stateKey] = &saved
	path := env.Bin
	if baseline["PATH"] != nil {
		path += string(os.PathListSeparator) + *baseline["PATH"]
	}
	delta["PATH"], delta["VIRTUAL_ENV"], delta["VIRTUAL_ENV_PROMPT"], delta["PYTHONHOME"] = &path, &env.Path, &env.Name, nil
	return delta, nil
}

func Render(name string, delta State) (string, error) {
	if name == "nu" || name == "nushell" {
		values := map[string]any{}
		for key, value := range delta {
			if key == "PATH" && value != nil {
				values[key] = filepath.SplitList(*value)
			} else {
				values[key] = value
			}
		}
		data, err := json.Marshal(values)
		return string(data) + "\n", err
	}
	var code strings.Builder
	for _, key := range append([]string{stateKey}, variables...) {
		value, present := delta[key]
		if !present {
			continue
		}
		switch name {
		case "bash", "zsh":
			if value == nil {
				fmt.Fprintf(&code, "unset %s\n", key)
			} else {
				fmt.Fprintf(&code, "export %s='%s'\n", key, strings.ReplaceAll(*value, "'", "'\"'\"'"))
			}
		case "fish":
			if value == nil {
				fmt.Fprintf(&code, "set -e %s; or true\n", key)
			} else {
				values := []string{*value}
				if key == "PATH" {
					values = filepath.SplitList(*value)
				}
				fmt.Fprintf(&code, "set -gx %s", key)
				for _, part := range values {
					fmt.Fprintf(&code, " '%s'", strings.NewReplacer(`\`, `\\`, "'", `\'`).Replace(part))
				}
				code.WriteByte('\n')
			}
		case "pwsh", "powershell":
			if value == nil {
				fmt.Fprintf(&code, "Remove-Item Env:\\%s -ErrorAction SilentlyContinue\n", key)
			} else {
				fmt.Fprintf(&code, "$env:%s = '%s'\n", key, strings.ReplaceAll(*value, "'", "''"))
			}
		default:
			return "", fmt.Errorf("unsupported shell %q", name)
		}
	}
	return code.String(), nil
}
