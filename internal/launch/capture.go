package launch

// Captured command-line output (F-293, option Misc > "Show command line output in a window"): instead of a terminal
// window, a console program or shell command typed on the command line runs without a window and what it prints is
// shown in the app. Since nobody can type into it, it gets no input (stdin is the null device, and on macOS and Linux
// it runs in a session of its own, so it has no terminal to ask a password on either); a program that waits anyway is
// stopped by the caller cancelling the context, which kills the whole process group; and the output is capped, so a
// program that never stops printing ("yes") is stopped when the cap is reached.

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"time"
)

// CaptureLimit is the most output a captured command keeps (1 MiB); the program is stopped when it prints more.
const CaptureLimit = 1 << 20

// CaptureResult is what a captured command line printed and how it ended.
type CaptureResult struct {
	Output    []byte // stdout and stderr together, at most the limit
	Truncated bool   // the program printed more than the limit and was stopped
	ExitCode  int    // the program's exit code (-1 when it was killed or could not start)
}

// cappedBuffer keeps the first limit bytes written to it and calls full once when more arrive.
type cappedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int
	truncated bool
	full      func()
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.truncated {
		return len(p), nil
	}
	room := b.limit - b.buf.Len()
	if len(p) > room {
		b.buf.Write(p[:max(room, 0)])
		b.truncated = true
		if b.full != nil {
			go b.full()
		}
		return len(p), nil
	}
	return b.buf.Write(p)
}

// captureProcess is the default Capture: run the command with no input, collect at most limit bytes of its output,
// and stop it (with its children) when ctx is cancelled or the limit is reached.
func captureProcess(ctx context.Context, c Command, limit int) (CaptureResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := buildExec(ctx, c)
	applyPlatform(cmd, c, false)
	ownGroup(cmd)
	out := &cappedBuffer{limit: limit, full: cancel}
	cmd.Stdin = nil // the null device
	cmd.Stdout, cmd.Stderr = out, out
	cmd.WaitDelay = 2 * time.Second // (a child that keeps the output pipe open does not keep us waiting)
	err := cmd.Run()
	removeAll(c.Cleanup)
	out.mu.Lock()
	r := CaptureResult{Output: append([]byte(nil), out.buf.Bytes()...), Truncated: out.truncated, ExitCode: -1}
	out.mu.Unlock()
	if cmd.ProcessState != nil {
		r.ExitCode = cmd.ProcessState.ExitCode()
	}
	var exitErr interface{ ExitCode() int }
	if err != nil && (errors.As(err, &exitErr) || r.Truncated || ctx.Err() != nil) {
		err = nil // a non-zero exit, a kill: the result tells; only a start failure is an error
	}
	if errors.Is(err, context.Canceled) {
		err = nil
	}
	return r, err
}

// CaptureLineLimited runs a command line through the shell in dir like CaptureLine, but with no input, a cap on the
// output (limit bytes, CaptureLimit when 0) and stopping when ctx is cancelled.
func (l *Launcher) CaptureLineLimited(ctx context.Context, line, dir string, limit int) (CaptureResult, error) {
	if limit <= 0 {
		limit = CaptureLimit
	}
	capture := l.Capture
	if capture == nil {
		capture = captureProcess
	}
	return capture(ctx, l.shellCommand(strings.TrimSpace(line), dir), limit)
}

// LineWantsTerminal says whether RunLine (in LineAuto mode) would open a terminal for a LineRun line: a shell command
// or a console program does; a GUI program, an application bundle or a document does not. Only such lines have
// output worth capturing; the others start as before.
func (l *Launcher) LineWantsTerminal(line, dir string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	var first string
	if l.windows() {
		first, _ = splitWindowsProgram(line)
	} else {
		words, err := SplitArgs(line)
		if err != nil || len(words) == 0 || hasShellSyntax(line) {
			return true
		}
		first = words[0]
	}
	path := ""
	if l.OS == runtime.GOOS {
		path = l.resolveFile(first, dir)
	}
	if path == "" {
		return true // the shell runs it
	}
	if l.darwin() && strings.HasSuffix(strings.TrimRight(path, "/"), ".app") {
		return false
	}
	if l.isDocument(path) {
		return false
	}
	return ExecKindOf(path) != KindGUI
}
