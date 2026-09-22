package process

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestCancellationStopsDescendants(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix process group test")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root := t.TempDir()
	cmd := exec.CommandContext(ctx, "sh", "-c", `(sleep 2; touch orphan) & touch ready; wait`)
	cmd.Dir, cmd.Stdout, cmd.Stderr = root, io.Discard, io.Discard
	done := make(chan error, 1)
	go func() { done <- RunBackground(cmd) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(root, "ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled process succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not stop process")
	}
	time.Sleep(2100 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(root, "orphan")); !os.IsNotExist(err) {
		t.Fatal("descendant survived cancellation")
	}
}
