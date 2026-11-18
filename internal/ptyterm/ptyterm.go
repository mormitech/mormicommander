// Package ptyterm runs the user's shell on a pseudo terminal for the embedded terminal pane (issue #31, F-856).
//
// The pane is opened by the user (it is off by default) and starts the user's own login shell in the active panel's
// folder. This package never types a command into the shell on its own: the only text it writes, besides what the user
// types, is a "cd" to a folder the user opened in a panel (Session.Cd), and only while the shell sits at its prompt
// (no program runs in the foreground), and names the user asked to insert (Session.Insert), which never end a line.
// Both refuse control characters, so a file or folder name can never press Enter or run anything.
//
// macOS and Linux use github.com/creack/pty; Windows (ConPTY) is not built yet, Start says so.
package ptyterm

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ErrUnsupported is returned by Start where no pseudo terminal is available (Windows, for now).
var ErrUnsupported = errors.New("the embedded terminal is not available on this system yet")

// ErrControlChars is returned when a folder or name to send holds a control character (a new line could run a command).
var ErrControlChars = errors.New("the name holds a control character (such as a new line) and is not sent to the terminal")

// ErrBusy is returned by Cd while a program runs in the terminal's foreground: the text would go to that program.
var ErrBusy = errors.New("a program is running in the terminal")

// Options say what to start.
type Options struct {
	Shell      string   // the shell program; "" picks the user's ($SHELL) or a system one
	Dir        string   // the folder to start in
	Cols, Rows int      // the size in characters
	Env        []string // the environment; nil takes the app's own
}

// shells that take -l for a login shell (the one Terminal.app starts too, which reads the user's profile and PATH: an
// app started from the Dock has only the system's minimal PATH).
var loginShells = map[string]bool{"zsh": true, "bash": true, "sh": true, "dash": true, "ksh": true, "mksh": true, "fish": true, "tcsh": true, "csh": true}

// PickShell returns the shell to run and its arguments: want when it is an existing absolute program, else $SHELL,
// else the first of zsh, bash and sh that exists.
func PickShell(want string) (string, []string) {
	ok := func(p string) bool {
		if p == "" || !filepath.IsAbs(p) {
			return false
		}
		st, err := os.Stat(p)
		return err == nil && !st.IsDir() && st.Mode()&0o111 != 0
	}
	shell := ""
	for _, c := range []string{want, os.Getenv("SHELL"), "/bin/zsh", "/bin/bash", "/bin/sh"} {
		if ok(c) {
			shell = c
			break
		}
	}
	if shell == "" {
		shell = "/bin/sh"
	}
	if loginShells[filepath.Base(shell)] {
		return shell, []string{"-l"}
	}
	return shell, nil
}

// Env returns the environment for the shell: base with TERM set for xterm.js, and a UTF-8 locale when none is set (an
// app started from the Dock gets none, and the shell would then show non-ASCII names as escapes).
func Env(base []string) []string {
	out := make([]string, 0, len(base)+4)
	hasLocale := false
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		switch k {
		case "TERM", "COLORTERM", "TERM_PROGRAM":
			continue
		case "LANG", "LC_ALL", "LC_CTYPE":
			hasLocale = true
		}
		out = append(out, kv)
	}
	out = append(out, "TERM=xterm-256color", "COLORTERM=truecolor", "TERM_PROGRAM=mormiCommander")
	if !hasLocale && runtime.GOOS == "darwin" {
		out = append(out, "LANG=en_US.UTF-8")
	}
	return out
}

// hasControl tells whether s holds a control character (C0, DEL or C1).
func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return true
		}
	}
	return false
}

// Quote quotes s as one word for the shell (by its program's name): single quotes, with fish's own rule for
// backslashes and quotes inside them. It refuses control characters.
func Quote(shell, s string) (string, error) {
	if hasControl(s) {
		return "", ErrControlChars
	}
	if filepath.Base(shell) == "fish" {
		return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'", nil
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'", nil
}

// CdLine is the text that changes the shell's folder to dir: a leading space (most shells can keep it out of the
// history), "cd", the quoted folder and Enter. dir must be absolute.
func CdLine(shell, dir string) (string, error) {
	if !filepath.IsAbs(dir) {
		return "", errors.New("the folder is not an absolute path")
	}
	q, err := Quote(shell, dir)
	if err != nil {
		return "", err
	}
	return " cd " + q + "\r", nil
}

// InsertText is the text that puts names at the prompt: each one quoted, separated and followed by a space. It never
// ends a line.
func InsertText(shell string, names []string) (string, error) {
	var b strings.Builder
	for _, n := range names {
		if n == "" {
			continue
		}
		q, err := Quote(shell, n)
		if err != nil {
			return "", err
		}
		b.WriteString(q)
		b.WriteByte(' ')
	}
	return b.String(), nil
}
