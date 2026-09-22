package environment

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/RisPNG/easyvenv/internal/process"
)

var (
	ErrNotFound    = errors.New("environment does not exist")
	ErrInvalid     = errors.New("not a complete Python virtual environment")
	ErrMiseMissing = errors.New("mise is required for an explicit Python version")
	versionPattern = regexp.MustCompile(`^(3(\.[0-9]+){0,2}|latest)$`)
)

const marker = ".easyvenv-creating"
const metadataFile = ".easyvenv.json"

type Provisioner interface {
	ResolveVersion(context.Context, string) (string, error)
	InstallVersion(context.Context, string, io.Writer) error
	ExecWithVersion(context.Context, string, []string, io.Writer) error
	InstallMise(context.Context, io.Writer) error
}

type Interpreter struct {
	Executable string `json:"executable"`
	Version    string `json:"version"`
	Source     string `json:"source"`
	Requested  string `json:"requested,omitempty"`
}

type Metadata struct {
	Version   string    `json:"version"`
	Source    string    `json:"source,omitempty"`
	Requested string    `json:"requested,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

type Environment struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Bin    string `json:"bin"`
	Python string `json:"python"`
	Active bool   `json:"active"`
	Metadata
}

type Service struct {
	Root        string
	Provisioner Provisioner
}

func New(provisioner Provisioner) (*Service, error) {
	root := os.Getenv("WORKON_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		root = filepath.Join(home, ".venvs")
	}
	if !filepath.IsAbs(root) {
		return nil, errors.New("WORKON_HOME must be an absolute path so environments work from every directory")
	}
	return &Service{Root: filepath.Clean(root), Provisioner: provisioner}, nil
}

func (s *Service) EnvironmentPath(name string) (string, error) {
	if name == "" || name == "." || name == ".." || strings.HasPrefix(name, "-") || strings.HasPrefix(name, ".easyvenv") || strings.ContainsAny(name, `/\:<>"|?*`) || strings.TrimSpace(name) != name || strings.HasSuffix(name, ".") {
		return "", errors.New("use a simple environment name without path components or reserved characters")
	}
	for _, c := range name {
		if c < 32 || c == 127 {
			return "", errors.New("environment names cannot contain control characters")
		}
	}
	base := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9') {
		return "", errors.New("environment name is reserved on Windows")
	}
	return filepath.Join(s.Root, name), nil
}

func (s *Service) ResolvePython(ctx context.Context, requested string) (Interpreter, error) {
	if requested != "" {
		if !versionPattern.MatchString(requested) {
			return Interpreter{}, errors.New("Python version must be 3, 3.x, 3.x.y, or latest")
		}
		version, err := s.Provisioner.ResolveVersion(ctx, requested)
		return Interpreter{Version: version, Source: "mise", Requested: requested}, err
	}
	for _, name := range []string{"python", "python3"} {
		path, err := exec.LookPath(name)
		if errors.Is(err, exec.ErrNotFound) {
			continue
		}
		if err != nil {
			return Interpreter{}, err
		}
		cmd := exec.CommandContext(ctx, path, "-c", `import json,sys; print(json.dumps({"executable":sys.executable,"version":".".join(map(str,sys.version_info[:3]))}))`)
		output, err := cmd.Output()
		if err != nil {
			return Interpreter{}, fmt.Errorf("inspect %s: %w", path, err)
		}
		var python Interpreter
		if err := json.Unmarshal(output, &python); err != nil {
			return Interpreter{}, fmt.Errorf("inspect %s: %w", path, err)
		}
		if !strings.HasPrefix(python.Version, "3.") {
			return Interpreter{}, fmt.Errorf("%s is not Python 3", path)
		}
		python.Source = "path"
		return python, nil
	}
	return Interpreter{}, errors.New("Python was not found on PATH; select Python first or supply a version, for example: venv create myproject 3.11")
}

func (s *Service) Inspect(name string) (Environment, error) {
	path, err := s.EnvironmentPath(name)
	if err != nil {
		return Environment{}, err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return Environment{}, fmt.Errorf("%s: %w", name, ErrNotFound)
	}
	if err != nil {
		return Environment{}, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return Environment{}, fmt.Errorf("%s: %w", name, ErrInvalid)
	}
	if _, err := os.Stat(filepath.Join(path, marker)); err == nil {
		return Environment{}, fmt.Errorf("%s: creation is unfinished: %w", name, ErrInvalid)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Environment{}, err
	}
	cfg, err := os.Open(filepath.Join(path, "pyvenv.cfg"))
	if errors.Is(err, os.ErrNotExist) {
		return Environment{}, fmt.Errorf("%s: %w", name, ErrInvalid)
	}
	if err != nil {
		return Environment{}, err
	}
	defer cfg.Close()
	env := Environment{Name: name, Path: path, Bin: filepath.Join(path, "bin")}
	env.Python = filepath.Join(env.Bin, "python")
	if runtime.GOOS == "windows" {
		env.Bin = filepath.Join(path, "Scripts")
		env.Python = filepath.Join(env.Bin, "python.exe")
	}
	pythonInfo, err := os.Stat(env.Python)
	if err != nil || pythonInfo.IsDir() || (runtime.GOOS != "windows" && pythonInfo.Mode()&0111 == 0) {
		return Environment{}, fmt.Errorf("%s: Python executable is missing: %w", name, ErrInvalid)
	}
	scanner := bufio.NewScanner(cfg)
	home := ""
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if ok {
			switch strings.TrimSpace(key) {
			case "home":
				home = strings.TrimSpace(value)
			case "version", "version_info":
				env.Version = strings.TrimSpace(value)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return Environment{}, err
	}
	if home == "" {
		return Environment{}, fmt.Errorf("%s: pyvenv.cfg has no interpreter home: %w", name, ErrInvalid)
	}
	if data, err := os.ReadFile(filepath.Join(path, metadataFile)); err == nil {
		var metadata Metadata
		if json.Unmarshal(data, &metadata) == nil {
			env.Source, env.Requested, env.CreatedAt = metadata.Source, metadata.Requested, metadata.CreatedAt
		}
	}
	if active := os.Getenv("VIRTUAL_ENV"); active != "" {
		activeInfo, err := os.Stat(active)
		env.Active = err == nil && os.SameFile(info, activeInfo)
	}
	return env, nil
}

func (s *Service) List() ([]Environment, error) {
	entries, err := os.ReadDir(s.Root)
	if errors.Is(err, os.ErrNotExist) {
		return []Environment{}, nil
	}
	if err != nil {
		return nil, err
	}
	environments := make([]Environment, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".easyvenv") {
			continue
		}
		if _, err := s.EnvironmentPath(entry.Name()); err != nil {
			continue
		}
		env, err := s.Inspect(entry.Name())
		if errors.Is(err, ErrInvalid) || errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		environments = append(environments, env)
	}
	return environments, nil
}

func (s *Service) Create(ctx context.Context, name string, python Interpreter, output io.Writer) (env Environment, err error) {
	path, err := s.EnvironmentPath(name)
	if err != nil {
		return env, err
	}
	if err = os.MkdirAll(s.Root, 0755); err != nil {
		return env, err
	}
	if err = os.Mkdir(path, 0755); err != nil {
		return env, fmt.Errorf("create %s: %w", name, err)
	}
	complete := false
	defer func() {
		if !complete {
			err = errors.Join(err, os.RemoveAll(path))
		}
	}()
	if err = os.WriteFile(filepath.Join(path, marker), nil, 0600); err != nil {
		return env, err
	}
	args := []string{"-m", "venv", path}
	if python.Source == "mise" {
		err = s.Provisioner.ExecWithVersion(ctx, python.Version, args, output)
	} else {
		cmd := exec.CommandContext(ctx, python.Executable, args...)
		cmd.Stdout, cmd.Stderr = output, output
		err = process.RunBackground(cmd)
	}
	if err != nil {
		return env, fmt.Errorf("create %s: %w", name, err)
	}
	if err = ctx.Err(); err != nil {
		return env, err
	}
	data, err := json.Marshal(Metadata{Version: python.Version, Source: python.Source, Requested: python.Requested, CreatedAt: time.Now().UTC()})
	if err != nil {
		return env, err
	}
	if err = os.WriteFile(filepath.Join(path, metadataFile), append(data, '\n'), 0644); err != nil {
		return env, err
	}
	if err = os.Remove(filepath.Join(path, marker)); err != nil {
		return env, err
	}
	env, err = s.Inspect(name)
	if err != nil {
		return env, err
	}
	complete = true
	return env, nil
}

func (s *Service) Delete(name string) error {
	env, err := s.Inspect(name)
	if err != nil {
		return err
	}
	if env.Active {
		return fmt.Errorf("%s is active; deactivate it before deleting", name)
	}
	return os.RemoveAll(env.Path)
}

func (s *Service) Run(ctx context.Context, name string, args []string, input io.Reader, output, stderr io.Writer) error {
	env, err := s.Inspect(name)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return errors.New("usage: venv run <name> <command> [arguments...]")
	}
	vars := os.Environ()
	for i := len(vars) - 1; i >= 0; i-- {
		key, _, _ := strings.Cut(vars[i], "=")
		if strings.EqualFold(key, "PATH") || strings.EqualFold(key, "PYTHONHOME") || strings.EqualFold(key, "VIRTUAL_ENV") || strings.EqualFold(key, "VIRTUAL_ENV_PROMPT") {
			vars = append(vars[:i], vars[i+1:]...)
		}
	}
	path := env.Bin + string(os.PathListSeparator) + os.Getenv("PATH")
	vars = append(vars, "PATH="+path, "VIRTUAL_ENV="+env.Path, "VIRTUAL_ENV_PROMPT="+env.Name)
	executable := args[0]
	if !strings.ContainsAny(executable, `/\`) {
		extensions := []string{""}
		if runtime.GOOS == "windows" {
			extensions = append(extensions, filepath.SplitList(os.Getenv("PATHEXT"))...)
		}
		found := false
		for _, dir := range filepath.SplitList(path) {
			for _, extension := range extensions {
				candidate := filepath.Join(dir, executable+extension)
				info, statErr := os.Stat(candidate)
				if statErr == nil && !info.IsDir() && (runtime.GOOS == "windows" || info.Mode()&0111 != 0) {
					executable, err = filepath.Abs(candidate)
					if err != nil {
						return err
					}
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return fmt.Errorf("command %q was not found in the environment or PATH", args[0])
		}
	}
	cmd := exec.CommandContext(ctx, executable, args[1:]...)
	cmd.Env, cmd.Stdin, cmd.Stdout, cmd.Stderr = vars, input, output, stderr
	return cmd.Run()
}
