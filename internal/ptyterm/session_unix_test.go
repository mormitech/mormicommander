//go:build !windows

package ptyterm

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// reader collects a session's output in the background.
type reader struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func collect(s *Session) *reader {
	r := &reader{}
	go func() {
		b := make([]byte, 4096)
		for {
			n, err := s.Read(b)
			r.mu.Lock()
			r.buf.Write(b[:n])
			r.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	return r
}

func (r *reader) waitFor(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		ok := strings.Contains(r.buf.String(), want)
		r.mu.Unlock()
		if ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t.Fatalf("output never showed %q; got %q", want, r.buf.String())
}

func startSh(t *testing.T, dir string) (*Session, *reader) {
	t.Helper()
	// a plain sh with no profile, so the test does not depend on the user's shell set-up
	env := []string{"PATH=/usr/bin:/bin", "HOME=" + dir, "ENV=", "PS1=$ "}
	s, err := Start(Options{Shell: "/bin/sh", Dir: dir, Cols: 100, Rows: 30, Env: env})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, collect(s)
}

func TestSessionRunsTypedLinesAndCd(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	sub := filepath.Join(dir, "it's a folder")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	s, out := startSh(t, dir)
	if _, err := s.Write([]byte("echo sum=$((40+2))\r")); err != nil {
		t.Fatal(err)
	}
	out.waitFor(t, "sum=42")
	// at the prompt the shell is not busy, so a cd goes in
	deadline := time.Now().Add(5 * time.Second)
	for s.Busy() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if err := s.Cd(sub); err != nil {
		t.Fatal(err)
	}
	s.Write([]byte("echo \"at=$(pwd)\"\r"))
	out.waitFor(t, "at="+sub)
	if err := s.Resize(120, 40); err != nil {
		t.Fatal(err)
	}
	s.Write([]byte("stty size\r"))
	out.waitFor(t, "40 120")
}

func TestSessionBusyRefusesCd(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	s, out := startSh(t, dir)
	// job control on, so the running program gets the foreground (as in an interactive login shell)
	s.Write([]byte("set -m; sleep 30\r"))
	deadline := time.Now().Add(5 * time.Second)
	for !s.Busy() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !s.Busy() {
		t.Skip("this sh does not give the foreground to its jobs")
	}
	if err := s.Cd(dir); err != ErrBusy {
		t.Fatalf("Cd while a program runs = %v, want ErrBusy", err)
	}
	_ = out
}

func TestSessionCloseEndsShell(t *testing.T) {
	dir := t.TempDir()
	s, _ := startSh(t, dir)
	s.Write([]byte("sleep 60\r"))
	time.Sleep(100 * time.Millisecond)
	done := make(chan struct{})
	go func() { s.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(6 * time.Second):
		t.Fatal("Close did not return")
	}
	select {
	case <-s.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("the shell still runs after Close")
	}
	if _, err := s.Write([]byte("x")); err == nil {
		t.Error("writing after Close fails")
	}
}
