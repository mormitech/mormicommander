//go:build !windows

package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// termEvents collects what the Terminal feature sends to the page.
type termEvents struct {
	mu   sync.Mutex
	out  strings.Builder
	exit []int
}

func (e *termEvents) emit(event string, data ...any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch event {
	case "terminal-data":
		b, _ := base64.StdEncoding.DecodeString(data[1].(string))
		e.out.Write(b)
	case "terminal-exit":
		e.exit = append(e.exit, data[1].(int))
	}
}

func (e *termEvents) waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		e.mu.Lock()
		ok := cond()
		e.mu.Unlock()
		if ok {
			return
		}
	}
	t.Fatalf("never saw %s; output %q", what, e.out.String())
}

func TestTerminalFeature(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	sub := filepath.Join(dir, "sub dir")
	os.Mkdir(sub, 0o755)
	ev := &termEvents{}
	term := newTerminal(&App{})
	term.emit, term.shell, term.env = ev.emit, "/bin/sh", []string{"PATH=/usr/bin:/bin", "HOME=" + dir, "ENV=", "PS1=$ "}
	t.Cleanup(term.closeAll)

	id, err := term.Start(dir, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	// the shell starts in the folder asked for and runs only what is typed
	if err := term.Write(id, "echo \"in=$(pwd)\"\r"); err != nil {
		t.Fatal(err)
	}
	ev.waitFor(t, func() bool { return strings.Contains(ev.out.String(), "in="+dir) }, "the start folder")
	for end := time.Now().Add(5 * time.Second); term.Busy(id) && time.Now().Before(end); {
		time.Sleep(20 * time.Millisecond)
	}
	// a folder that does not exist is not sent
	if ok, _ := term.Cd(id, filepath.Join(dir, "missing")); ok {
		t.Error("cd to a missing folder is not sent")
	}
	if ok, err := term.Cd(id, sub); !ok || err != nil {
		t.Fatalf("Cd = %v, %v", ok, err)
	}
	term.Write(id, "echo \"now=$(pwd)\"\r")
	ev.waitFor(t, func() bool { return strings.Contains(ev.out.String(), "now="+sub) }, "the cd")
	// names go in quoted, without Enter; a name with a new line is refused
	if err := term.Insert(id, []string{"a b.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := term.Insert(id, []string{"x\ntouch pwned"}); err == nil {
		t.Error("a name with a new line must be refused")
	}
	term.Write(id, "\x15exit 3\r")
	ev.waitFor(t, func() bool { return len(ev.exit) == 1 }, "the exit event")
	if ev.exit[0] != 3 {
		t.Errorf("exit code = %d, want 3", ev.exit[0])
	}
	if _, err := os.Stat(filepath.Join(sub, "pwned")); err == nil {
		t.Error("nothing may run from an inserted name")
	}
	if err := term.Write(id, "x"); err == nil {
		t.Error("writing to an ended terminal fails")
	}
}

func TestTerminalStartDir(t *testing.T) {
	d := t.TempDir()
	if startDir(d) != d {
		t.Error("an existing folder is used")
	}
	home, _ := os.UserHomeDir()
	for _, bad := range []string{"", "relative", "/no/such/folder", "ftp://host/x"} {
		if startDir(bad) != home {
			t.Errorf("startDir(%q) should fall back to the home folder", bad)
		}
	}
}
