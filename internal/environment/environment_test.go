package environment

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

type fakeProvisioner struct {
	resolved, installed, executed []string
	binary                        string
	err                           error
}

func (f *fakeProvisioner) ResolveVersion(_ context.Context, version string) (string, error) {
	f.resolved = append(f.resolved, version)
	return "3.11.14", f.err
}
func (f *fakeProvisioner) InstallVersion(_ context.Context, version string, _ io.Writer) error {
	f.installed = append(f.installed, version)
	return f.err
}
func (f *fakeProvisioner) ExecWithVersion(ctx context.Context, version string, args []string, output io.Writer) error {
	f.executed = append(f.executed, version)
	if f.err != nil {
		return f.err
	}
	cmd := exec.CommandContext(ctx, f.binary, args...)
	cmd.Stdout, cmd.Stderr = output, output
	return cmd.Run()
}
func (f *fakeProvisioner) InstallMise(context.Context, io.Writer) error { return f.err }

func TestNamesStayInsideRoot(t *testing.T) {
	service := &Service{Root: t.TempDir()}
	for _, name := range []string{"", ".", "..", "../outside", "a/b", `a\b`, "/tmp", "C:temp", "a\nb", "a\x00b", " space", "trail.", "NUL", "con.txt", "LPT1", ".easyvenv-create"} {
		if _, err := service.EnvironmentPath(name); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
	for _, name := range []string{"project", "my project", "a'b", "a$(touch nope)", "python-3.11", "项目"} {
		path, err := service.EnvironmentPath(name)
		if err != nil || filepath.Dir(path) != service.Root {
			t.Errorf("name %q: %s %v", name, path, err)
		}
	}
}

func TestDefaultPythonNeverUsesMise(t *testing.T) {
	backend := &fakeProvisioner{err: errors.New("must not call mise")}
	service := &Service{Root: t.TempDir(), Provisioner: backend}
	python, err := service.ResolvePython(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if python.Source != "path" || python.Executable == "" || !strings.HasPrefix(python.Version, "3.") {
		t.Fatalf("unexpected interpreter: %+v", python)
	}
	if len(backend.resolved) != 0 {
		t.Fatal("consulted mise")
	}
}

func TestExplicitVersionUsesMise(t *testing.T) {
	backend := &fakeProvisioner{}
	service := &Service{Root: t.TempDir(), Provisioner: backend}
	python, err := service.ResolvePython(context.Background(), "3.11")
	if err != nil {
		t.Fatal(err)
	}
	if python.Source != "mise" || python.Requested != "3.11" || python.Version != "3.11.14" || len(backend.resolved) != 1 {
		t.Fatalf("unexpected resolution: %+v %+v", python, backend)
	}
	for _, version := range []string{"--help", "python@3", "../../bad", "2.7", "3;touch bad"} {
		if _, err := service.ResolvePython(context.Background(), version); err == nil {
			t.Errorf("accepted %q", version)
		}
	}
}

func TestRealEnvironmentLifecycle(t *testing.T) {
	service := &Service{Root: filepath.Join(t.TempDir(), "root with spaces")}
	python, err := service.ResolvePython(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	env, err := service.Create(context.Background(), "my project", python, &output)
	if err != nil {
		t.Fatalf("%v\n%s", err, output.String())
	}
	if env.Version != python.Version || env.Source != "path" || env.CreatedAt.IsZero() {
		t.Fatalf("metadata: %+v", env)
	}
	output.Reset()
	err = service.Run(context.Background(), env.Name, []string{"python", "-c", `import os,sys; assert os.path.samefile(sys.prefix, os.environ['VIRTUAL_ENV']), (sys.executable, sys.prefix, os.environ['VIRTUAL_ENV']); assert sys.prefix != sys.base_prefix, (sys.prefix, sys.base_prefix); print(sys.prefix)`}, nil, &output, &output)
	if err != nil {
		t.Fatalf("run: %v: %s", err, &output)
	}
	prefixInfo, err := os.Stat(strings.TrimSpace(output.String()))
	if err != nil {
		t.Fatalf("inspect Python prefix %q: %v", output.String(), err)
	}
	envInfo, err := os.Stat(env.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(prefixInfo, envInfo) {
		t.Fatalf("Python prefix %q is not environment %q", strings.TrimSpace(output.String()), env.Path)
	}
	output.Reset()
	if err := service.Run(context.Background(), env.Name, []string{"pip", "--version"}, nil, &output, &output); err != nil {
		t.Fatalf("pip entrypoint: %v: %s", err, &output)
	}
	_, location, found := strings.Cut(output.String(), " from ")
	pipPath, _, versionFound := strings.Cut(location, " (python ")
	if !found || !versionFound {
		t.Fatalf("unexpected pip version output: %s", &output)
	}
	pipPath, err = filepath.EvalSymlinks(pipPath)
	if err != nil {
		t.Fatal(err)
	}
	envPath, err := filepath.EvalSymlinks(env.Path)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(envPath, pipPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		t.Fatalf("pip resolved outside environment: %s", &output)
	}
	if _, err := service.Create(context.Background(), env.Name, python, io.Discard); !errors.Is(err, os.ErrExist) {
		t.Fatalf("duplicate: %v", err)
	}
	if err := os.Remove(filepath.Join(env.Path, metadataFile)); err != nil {
		t.Fatal(err)
	}
	envs, err := service.List()
	if err != nil || len(envs) != 1 || envs[0].Version != python.Version {
		t.Fatalf("metadata-free discovery: %+v, %v", envs, err)
	}
	if err := os.WriteFile(filepath.Join(env.Path, metadataFile), []byte("broken JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Inspect(env.Name); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VIRTUAL_ENV", env.Path)
	if err := service.Delete(env.Name); err == nil {
		t.Fatal("deleted active environment")
	}
	t.Setenv("VIRTUAL_ENV", "")
	if err := service.Delete(env.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(env.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("environment remains: %v", err)
	}
}

func TestCreationFailureCleansOnlyNewEnvironment(t *testing.T) {
	backend := &fakeProvisioner{err: errors.New("creation failed")}
	service := &Service{Root: t.TempDir(), Provisioner: backend}
	_, err := service.Create(context.Background(), "failed", Interpreter{Source: "mise", Version: "3.11.14"}, io.Discard)
	if err == nil {
		t.Fatal("expected creation failure")
	}
	if _, err := os.Stat(filepath.Join(service.Root, "failed")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("partial directory remains")
	}
	path := filepath.Join(service.Root, "existing")
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "keep"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(context.Background(), "existing", Interpreter{Source: "mise"}, io.Discard); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(path, "keep")); err != nil {
		t.Fatal("existing directory changed", err)
	}
}

func TestDiscoveryRejectsIncompleteAndSymlinkedEnvironments(t *testing.T) {
	service := &Service{Root: t.TempDir()}
	for _, name := range []string{"empty", "unfinished", "valid"} {
		path := filepath.Join(service.Root, name)
		bin, executable := "bin", "python"
		if runtime.GOOS == "windows" {
			bin, executable = "Scripts", "python.exe"
		}
		if err := os.MkdirAll(filepath.Join(path, bin), 0755); err != nil {
			t.Fatal(err)
		}
		if name == "empty" {
			continue
		}
		if err := os.WriteFile(filepath.Join(path, "pyvenv.cfg"), []byte("home = /python\nversion = 3.11.14\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, bin, executable), nil, 0755); err != nil {
			t.Fatal(err)
		}
		if name == "unfinished" {
			if err := os.WriteFile(filepath.Join(path, marker), nil, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(filepath.Join(service.Root, "valid"), filepath.Join(service.Root, "link")); err != nil {
			t.Fatal(err)
		}
		if err := service.Delete("link"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("symlink deletion: %v", err)
		}
	}
	envs, err := service.List()
	if err != nil || len(envs) != 1 || envs[0].Name != "valid" {
		t.Fatalf("discovery: %+v %v", envs, err)
	}
}

func TestConcurrentCreationHasOneOwner(t *testing.T) {
	service := &Service{Root: t.TempDir()}
	python, err := service.ResolvePython(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := service.Create(context.Background(), "shared", python, io.Discard)
			results <- err
		}()
	}
	wait.Wait()
	close(results)
	successes, duplicates := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, os.ErrExist) {
			duplicates++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || duplicates != 1 {
		t.Fatalf("success=%d duplicates=%d", successes, duplicates)
	}
	if _, err := service.Inspect("shared"); err != nil {
		t.Fatal(err)
	}
}

func TestCancelledCreationCleansUp(t *testing.T) {
	service := &Service{Root: t.TempDir()}
	python, err := service.ResolvePython(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Create(ctx, "cancelled", python, io.Discard); err == nil {
		t.Fatal("expected cancellation")
	}
	if _, err := os.Stat(filepath.Join(service.Root, "cancelled")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancelled environment remains")
	}
}

func TestEnvironmentThroughStorageAlias(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix storage alias regression")
	}
	root := t.TempDir()
	storage := filepath.Join(root, "storage")
	if err := os.Mkdir(storage, 0755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(storage, alias); err != nil {
		t.Fatal(err)
	}
	service := &Service{Root: alias}
	python, err := service.ResolvePython(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	env, err := service.Create(context.Background(), "project", python, &output)
	if err != nil {
		t.Fatalf("create through alias: %v: %s", err, &output)
	}
	canonical, err := filepath.EvalSymlinks(env.Path)
	if err != nil {
		t.Fatal(err)
	}
	output.Reset()
	err = service.Run(context.Background(), env.Name, []string{filepath.Join(canonical, "bin", "python"), "-c", `import os,sys; expected = os.environ['VIRTUAL_ENV']; assert sys.prefix != expected, (sys.prefix, expected); assert os.path.samefile(sys.prefix, expected), (sys.executable, sys.prefix, expected); assert sys.prefix != sys.base_prefix, (sys.prefix, sys.base_prefix)`}, nil, &output, &output)
	if err != nil {
		t.Fatalf("run through canonical path: %v: %s", err, &output)
	}
	t.Setenv("VIRTUAL_ENV", canonical)
	env, err = service.Inspect(env.Name)
	if err != nil || !env.Active {
		t.Fatalf("active environment through alias: %+v: %v", env, err)
	}
	if err := service.Delete(env.Name); err == nil {
		t.Fatal("deleted active environment accessed through an alias")
	}
}
