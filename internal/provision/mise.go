package provision

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/RisPNG/easyvenv/internal/environment"
	"github.com/RisPNG/easyvenv/internal/process"
)

type Mise struct{ Binary string }

func (m *Mise) Locate() (string, error) {
	if m.Binary != "" {
		return m.Binary, nil
	}
	path, err := exec.LookPath("mise")
	if err == nil {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	name := "mise"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path = filepath.Join(home, ".local", "bin", name)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return path, nil
	}
	return "", environment.ErrMiseMissing
}

func (m *Mise) ResolveVersion(ctx context.Context, requested string) (string, error) {
	binary, err := m.Locate()
	if err != nil {
		return "", err
	}
	if strings.Count(requested, ".") == 2 {
		return requested, nil
	}
	cmd := exec.CommandContext(ctx, binary, "latest", "--installed", "python@"+requested)
	output, err := cmd.Output()
	if err == nil && strings.TrimSpace(string(output)) != "" {
		return strings.TrimSpace(string(output)), nil
	}
	cmd = exec.CommandContext(ctx, binary, "latest", "python@"+requested)
	output, err = cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolve Python %s through mise: %w", requested, err)
	}
	version := strings.TrimSpace(string(output))
	if !strings.HasPrefix(version, "3.") {
		return "", fmt.Errorf("mise did not resolve a Python 3 version for %q", requested)
	}
	return version, nil
}

func (m *Mise) InstallVersion(ctx context.Context, version string, output io.Writer) error {
	binary, err := m.Locate()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, binary, "install", "python@"+version)
	cmd.Stdout, cmd.Stderr = output, output
	return process.RunBackground(cmd)
}

func (m *Mise) ExecWithVersion(ctx context.Context, version string, args []string, output io.Writer) error {
	binary, err := m.Locate()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, binary, append([]string{"exec", "python@" + version, "--", "python"}, args...)...)
	cmd.Stdout, cmd.Stderr = output, output
	return process.RunBackground(cmd)
}

func (m *Mise) InstallMise(ctx context.Context, output io.Writer) error {
	if _, err := m.Locate(); err == nil {
		return nil
	} else if !errors.Is(err, environment.ErrMiseMissing) {
		return err
	}
	if runtime.GOOS == "windows" {
		cmd := exec.CommandContext(ctx, "winget", "install", "--id", "jdx.mise", "--exact", "--source", "winget", "--accept-package-agreements", "--accept-source-agreements")
		cmd.Stdout, cmd.Stderr = output, output
		if err := process.RunBackground(cmd); err != nil {
			return fmt.Errorf("install mise with winget: %w", err)
		}
		path := filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "WinGet", "Links", "mise.exe")
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("mise installed; restart your terminal and retry: %w", err)
		}
		m.Binary = path
		return nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://mise.run", nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: time.Minute}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download mise installer: %s", response.Status)
	}
	script, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "sh")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = strings.NewReader(string(script)), output, output
	if err := process.RunBackground(cmd); err != nil {
		return fmt.Errorf("install mise: %w", err)
	}
	_, err = m.Locate()
	return err
}
