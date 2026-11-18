// Tests for captured command-line output (F-293): the cap, the stop, no input, and which lines are captured.
package launch

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCappedBuffer(t *testing.T) {
	called := make(chan struct{}, 2)
	b := &cappedBuffer{limit: 5, full: func() { called <- struct{}{} }}
	b.Write([]byte("abc"))
	if n, _ := b.Write([]byte("defgh")); n != 5 {
		t.Errorf("Write must report all bytes taken, got %d", n)
	}
	b.Write([]byte("more"))
	if got := b.buf.String(); got != "abcde" || !b.truncated {
		t.Errorf("buffer = %q truncated=%v", got, b.truncated)
	}
	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("full was not called")
	}
	select {
	case <-called:
		t.Error("full must be called once")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestCaptureLineLimitedUsesShellAndLimit(t *testing.T) {
	l, _ := fakeLauncher(t, "linux", nil)
	var got Command
	var gotLimit int
	l.Capture = func(_ context.Context, c Command, limit int) (CaptureResult, error) {
		got, gotLimit = c, limit
		return CaptureResult{Output: []byte("x")}, nil
	}
	if _, err := l.CaptureLineLimited(context.Background(), "  ls -l ", "/w", 0); err != nil {
		t.Fatal(err)
	}
	sameCmd(t, got, Command{Path: "/bin/sh", Args: []string{"-c", "ls -l"}, Dir: "/w"})
	if gotLimit != CaptureLimit {
		t.Errorf("limit = %d, want the default %d", gotLimit, CaptureLimit)
	}
}

func TestLineWantsTerminal(t *testing.T) {
	l, _ := fakeLauncher(t, "linux", nil) // another OS than this one when not on Linux: no file lookups
	for _, line := range []string{"ls | wc", "echo $HOME", "nosuchprogram-xyz"} {
		if runtime.GOOS != "windows" && !l.LineWantsTerminal(line, t.TempDir()) {
			t.Errorf("%q should want a terminal (the shell runs it)", line)
		}
	}
	if l.LineWantsTerminal("   ", "") {
		t.Error("an empty line wants nothing")
	}
	if runtime.GOOS == "windows" {
		return
	}
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "notes.txt"), "hello", 0o644)
	writeFile(t, filepath.Join(dir, "tool.sh"), "#!/bin/sh\necho hi\n", 0o755)
	real := New()
	if real.LineWantsTerminal("notes.txt", dir) {
		t.Error("a document opens with its app: nothing to capture")
	}
	if !real.LineWantsTerminal("./tool.sh", dir) {
		t.Error("a script is a console program")
	}
}

func TestCaptureProcessReal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	l := New()
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	r, err := l.CaptureLineLimited(ctx, "echo out; echo err >&2; exit 3", dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if s := string(r.Output); !strings.Contains(s, "out") || !strings.Contains(s, "err") || r.ExitCode != 3 || r.Truncated {
		t.Errorf("result = %q exit %d truncated %v", s, r.ExitCode, r.Truncated)
	}

	// no input: a program reading stdin gets end-of-file at once instead of waiting
	start := time.Now()
	r, err = l.CaptureLineLimited(ctx, "cat; echo done", dir, 0)
	if err != nil || !strings.Contains(string(r.Output), "done") || time.Since(start) > 10*time.Second {
		t.Errorf("reading stdin: %q %v", r.Output, err)
	}

	// endless output: stopped at the cap
	r, err = l.CaptureLineLimited(ctx, "yes", dir, 1000)
	if err != nil || !r.Truncated || len(r.Output) != 1000 {
		t.Errorf("yes: truncated=%v len=%d err=%v", r.Truncated, len(r.Output), err)
	}

	// a program that waits forever: stopped when the context is cancelled (the Stop button), children included
	stopCtx, stop := context.WithCancel(ctx)
	time.AfterFunc(300*time.Millisecond, stop)
	start = time.Now()
	r, err = l.CaptureLineLimited(stopCtx, "echo started; sleep 60 & wait", dir, 0)
	if err != nil || !strings.Contains(string(r.Output), "started") || time.Since(start) > 10*time.Second {
		t.Errorf("stop: %q %v after %v", r.Output, err, time.Since(start))
	}
}
