package fileops

// Tests of logging: the queue reports what it does to Queue.Log, and FileLogger writes it as TC-style lines.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileLogger(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "a"), "a")
	logPath := filepath.Join(t.TempDir(), "ops.log")
	log, closeLog, err := FileLogger(logPath)
	if err != nil {
		t.Fatal(err)
	}
	q := NewQueue(nil)
	q.Log = log
	start(t, q, Request{Kind: Copy, Sources: []string{filepath.Join(src, "a"), filepath.Join(src, "missing")}, Target: dst})
	if err := closeLog(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(logPath)
	s := string(b)
	for _, want := range []string{"] copy: " + filepath.Join(src, "a") + " -> " + filepath.Join(dst, "a"), "] error:", "] done: Copied 1 of 1 files"} {
		if !strings.Contains(s, want) {
			t.Errorf("log lacks %q:\n%s", want, s)
		}
	}
}

func TestEntryString(t *testing.T) {
	e := Entry{Time: time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local), Job: 7, Action: "move", Source: "/a", Target: "/b", Err: "x"}
	if got := e.String(); got != "2026-01-02 03:04:05 [7] move: /a -> /b (x)" {
		t.Errorf("got %q", got)
	}
}
