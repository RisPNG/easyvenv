package provision

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/RisPNG/easyvenv/internal/environment"
)

func TestMain(m *testing.M) {
	if os.Getenv("EASYVENV_MISE_FAKE") == "1" {
		file, err := os.OpenFile(os.Getenv("EASYVENV_MISE_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			panic(err)
		}
		json.NewEncoder(file).Encode(os.Args[1:])
		file.Close()
		if os.Getenv("EASYVENV_MISE_FAIL") == "1" {
			fmt.Fprintln(os.Stderr, "provision failed")
			os.Exit(9)
		}
		if os.Args[1] == "latest" {
			if os.Args[2] != "--installed" || os.Getenv("EASYVENV_MISE_INSTALLED") == "1" {
				fmt.Println("3.11.14")
			}
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestMiseCommandsAndVersionResolution(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(t.TempDir(), "commands.jsonl")
	t.Setenv("EASYVENV_MISE_FAKE", "1")
	t.Setenv("EASYVENV_MISE_LOG", log)
	backend := &Mise{Binary: binary}
	version, err := backend.ResolveVersion(context.Background(), "3.11")
	if err != nil || version != "3.11.14" {
		t.Fatalf("%s %v", version, err)
	}
	if err := backend.InstallVersion(context.Background(), version, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := backend.ExecWithVersion(context.Background(), version, []string{"-m", "venv", "/path with spaces/project"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var calls [][]string
	for decoder.More() {
		var call []string
		if err := decoder.Decode(&call); err != nil {
			t.Fatal(err)
		}
		calls = append(calls, call)
	}
	want := [][]string{{"latest", "--installed", "python@3.11"}, {"latest", "python@3.11"}, {"install", "python@3.11.14"}, {"exec", "python@3.11.14", "--", "python", "-m", "venv", "/path with spaces/project"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("got %#v want %#v", calls, want)
	}
	t.Setenv("EASYVENV_MISE_FAIL", "1")
	if err := backend.InstallVersion(context.Background(), version, io.Discard); err == nil {
		t.Fatal("ignored install failure")
	}
}

func TestExactVersionDoesNotRequireNetwork(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	backend := &Mise{Binary: binary}
	version, err := backend.ResolveVersion(context.Background(), "3.11.14")
	if err != nil || version != "3.11.14" {
		t.Fatalf("%s %v", version, err)
	}
}

func TestMissingMiseIsExplicit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	backend := &Mise{}
	if _, err := backend.ResolveVersion(context.Background(), "3.11.14"); !errors.Is(err, environment.ErrMiseMissing) {
		t.Fatalf("%v", err)
	}
}
