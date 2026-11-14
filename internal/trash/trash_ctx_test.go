package trash

// Tests for the cancellable and Finder-free routes (issue #36: a Trash job stuck at 0% while a Finder call hangs).
// The real trash is never touched: the command runner is faked, except in TestCommandStopsWithContext, which runs
// only "sleep" to check that an ended context kills a helper program.

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCommandStopsWithContext(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("no sleep program")
	}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	start := time.Now()
	_, err := command(ctx, "sleep", "30").CombinedOutput()
	if err == nil {
		t.Fatal("a killed helper must report an error")
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("the helper was not stopped with the context (took %v)", d)
	}
}

func TestMoveContextCancelled(t *testing.T) {
	if !supported {
		t.Skip("no trash on this platform")
	}
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	fake(t)
	lookPath = func(name string) (string, error) { return "/usr/bin/" + name, nil }
	ctx, cancel := context.WithCancel(context.Background())
	runCommand = func(cmd *exec.Cmd) ([]byte, error) { cancel(); return []byte("killed"), errors.New("signal: killed") }
	if err := MoveContext(ctx, file); !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled move = %v, want context.Canceled", err)
	}
}

func TestMoveDirect(t *testing.T) {
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	calls := fake(t)
	if err := MoveDirect(context.Background(), ""); err == nil || len(*calls) != 0 {
		t.Fatalf("MoveDirect must refuse an empty path before any call: %v %v", err, *calls)
	}
	if !supported {
		return
	}
	if err := MoveDirect(context.Background(), file); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 {
		t.Fatalf("want one call, got %v", *calls)
	}
	call := strings.Join((*calls)[0], "\n")
	if runtime.GOOS == "darwin" {
		if !DirectSupported() || !strings.Contains(call, "trashItemAtURL") || strings.Contains(call, `"Finder"`) || !strings.HasSuffix(call, file) {
			t.Errorf("on macOS MoveDirect must use NSFileManager, not Finder:\n%s", call)
		}
	} else if DirectSupported() {
		t.Error("only macOS has a separate direct route")
	}
}
