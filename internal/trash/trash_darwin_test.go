package trash

// macOS-only tests for the Finder call (issue #36): it carries an AppleScript timeout, and the two known failures
// (automation permission refused, no answer in time) come back as errors callers can recognise. The real trash is
// never touched: the command runner is faked.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDarwinFinderTimeoutAndErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	calls := fake(t)
	if err := Move(file); err != nil {
		t.Fatal(err)
	}
	if call := strings.Join((*calls)[0], "\n"); !strings.Contains(call, "with timeout of 61 seconds") {
		t.Errorf("the Finder call must carry a timeout:\n%s", call)
	}
	if finderTimeout(100000) != 600 {
		t.Error("the Finder timeout must be capped")
	}
	for out, want := range map[string]error{
		"execution error: Not authorized to send Apple events to Finder. (-1743)": ErrFinderNotAllowed,
		"execution error: Finder got an error: AppleEvent timed out. (-1712)":     ErrFinderTimeout,
	} {
		runCommand = func(*exec.Cmd) ([]byte, error) { return []byte(out), errors.New("exit status 1") }
		if err := Move(file); !errors.Is(err, want) || !strings.Contains(err.Error(), file) {
			t.Errorf("%q gave %v, want %v", out, err, want)
		}
	}
	runCommand = func(*exec.Cmd) ([]byte, error) { return []byte("some other failure"), errors.New("exit status 1") }
	if err := Move(file); err == nil || !strings.Contains(err.Error(), "some other failure") {
		t.Errorf("other output must be kept: %v", err)
	}
}
