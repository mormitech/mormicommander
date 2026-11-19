// Package rclonetest builds the fake rclone program (fakerclone) for tests, so nothing ever talks to a real remote.
package rclonetest

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

var (
	once  sync.Once
	built string
	err   error
)

// Fake is a built fake rclone with its remotes folder.
type Fake struct {
	Bin  string // the program
	Root string // every folder in it is a remote
	Log  string // one line per call
}

// Env is the environment the fake needs.
func (f Fake) Env() []string {
	return []string{"FAKE_RCLONE_ROOT=" + f.Root, "FAKE_RCLONE_LOG=" + f.Log}
}

// Calls returns the calls made so far, one line each (arguments separated by tabs).
func (f Fake) Calls() []string {
	b, _ := os.ReadFile(f.Log)
	var out []string
	for _, l := range splitLines(string(b)) {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

// New builds the fake (once per test binary) and makes an empty remotes folder with the given remotes.
func New(t *testing.T, remotes ...string) Fake {
	t.Helper()
	once.Do(func() {
		dir, e := os.MkdirTemp("", "fakerclone")
		if e != nil {
			err = e
			return
		}
		name := "rclone"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		built = filepath.Join(dir, name)
		_, self, _, _ := runtime.Caller(0)
		cmd := exec.Command("go", "build", "-o", built, ".")
		cmd.Dir = filepath.Join(filepath.Dir(self), "fakerclone")
		if out, e := cmd.CombinedOutput(); e != nil {
			err = &buildError{string(out), e}
		}
	})
	if err != nil {
		t.Skipf("can't build the fake rclone: %v", err)
	}
	root := t.TempDir()
	for _, r := range remotes {
		if e := os.MkdirAll(filepath.Join(root, r), 0o755); e != nil {
			t.Fatal(e)
		}
	}
	return Fake{Bin: built, Root: root, Log: filepath.Join(t.TempDir(), "calls.log")}
}

type buildError struct {
	out string
	err error
}

func (b *buildError) Error() string { return b.err.Error() + ": " + b.out }
