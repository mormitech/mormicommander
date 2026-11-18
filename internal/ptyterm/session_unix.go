//go:build !windows

package ptyterm

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

// Session is one shell running on a pseudo terminal.
type Session struct {
	Shell string // the shell program (for quoting)

	cmd       *exec.Cmd
	pty       *os.File
	mu        sync.Mutex // guards writes and closed
	closed    bool
	done      chan struct{}
	exitCode  int
	closeOnce sync.Once
}

// Start starts the shell in o.Dir on a new pseudo terminal of o.Cols x o.Rows.
func Start(o Options) (*Session, error) {
	shell, args := PickShell(o.Shell)
	cmd := exec.Command(shell, args...)
	cmd.Dir = o.Dir
	env := o.Env
	if env == nil {
		env = os.Environ()
	}
	cmd.Env = Env(env)
	f, err := pty.StartWithSize(cmd, winsize(o.Cols, o.Rows))
	if err != nil {
		return nil, err
	}
	s := &Session{Shell: shell, cmd: cmd, pty: f, done: make(chan struct{}), exitCode: -1}
	go func() {
		_ = cmd.Wait()
		if cmd.ProcessState != nil {
			s.exitCode = cmd.ProcessState.ExitCode()
		}
		close(s.done)
	}()
	return s, nil
}

func winsize(cols, rows int) *pty.Winsize {
	clamp := func(v, def int) uint16 {
		if v <= 0 {
			return uint16(def)
		}
		return uint16(min(v, 1000))
	}
	return &pty.Winsize{Cols: clamp(cols, 80), Rows: clamp(rows, 24)}
}

// Read reads what the shell and its programs print (io.EOF, or an error, once the shell has ended).
func (s *Session) Read(p []byte) (int, error) {
	n, err := s.pty.Read(p)
	if err != nil && errors.Is(err, syscall.EIO) { // (Linux reports the closed other end as EIO)
		return n, errors.Join(err, os.ErrClosed)
	}
	return n, err
}

// Write sends what the user typed.
func (s *Session) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, os.ErrClosed
	}
	return s.pty.Write(p)
}

// Resize tells the shell the new size of the pane.
func (s *Session) Resize(cols, rows int) error {
	return pty.Setsize(s.pty, winsize(cols, rows))
}

// Busy tells whether a program other than the shell runs in the terminal's foreground (a cd typed now would go to
// it). An interactive shell at its prompt is its own foreground process group.
func (s *Session) Busy() bool {
	pg, err := unix.IoctlGetInt(int(s.pty.Fd()), unix.TIOCGPGRP)
	if err != nil || s.cmd.Process == nil {
		return true
	}
	return pg != s.cmd.Process.Pid
}

// Cd changes the shell's folder to dir, unless a program runs in the foreground (ErrBusy).
func (s *Session) Cd(dir string) error {
	line, err := CdLine(s.Shell, dir)
	if err != nil {
		return err
	}
	if s.Busy() {
		return ErrBusy
	}
	_, err = s.Write([]byte(line))
	return err
}

// Insert puts the quoted names at the prompt without pressing Enter.
func (s *Session) Insert(names []string) error {
	text, err := InsertText(s.Shell, names)
	if err != nil || text == "" {
		return err
	}
	_, err = s.Write([]byte(text))
	return err
}

// Done is closed when the shell has ended; ExitCode is then its exit code (-1 when it was killed).
func (s *Session) Done() <-chan struct{} { return s.done }

// ExitCode is the shell's exit code once Done is closed.
func (s *Session) ExitCode() int { return s.exitCode }

// Close ends the shell and everything started in the terminal: a hang-up to its session (as closing a terminal window
// does), then a kill after a short wait.
func (s *Session) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		if p := s.cmd.Process; p != nil {
			_ = syscall.Kill(-p.Pid, syscall.SIGHUP)
			select {
			case <-s.done:
			case <-time.After(1500 * time.Millisecond):
				_ = syscall.Kill(-p.Pid, syscall.SIGKILL)
				select {
				case <-s.done:
				case <-time.After(time.Second):
				}
			}
		}
		_ = s.pty.Close()
	})
	return nil
}
