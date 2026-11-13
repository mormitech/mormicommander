// Running programs: a program with parameters and a start folder (buttons, start menu items, associations), and a
// line typed into the command line (Enter, Shift+Enter; F-293/F-294/F-812). Total Commander's rules are followed:
// a document typed on the command line opens with its app, a GUI program starts on its own, a console program gets
// a terminal window (which closes when it ends, or stays open with Shift+Enter), and a shell command that is no
// program at all (dir, export, a pipe) goes to the shell. Output can also be captured instead, for the option of
// showing it in the app. Whether a program is a console program is read from the program file itself (see
// exekind.go), not guessed from its name.
package launch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// RunSpec is one program start as a button or start menu item describes it.
type RunSpec struct {
	Program  string   // program path or name (looked up in PATH); a document is opened with its app
	Params   string   // parameter text, already expanded (see ExpandParams); split by SplitArgs outside Windows
	Dir      string   // start folder; empty means the source panel's folder chosen by the caller
	Show     ShowMode // window state (minimized/maximized honoured on Windows)
	Admin    bool     // Windows: run as administrator (UAC prompt); ErrUnsupported elsewhere
	Cleanup  []string // list files from ExpandParams, deleted when the program has quit
	Terminal bool     // run inside a terminal window
	KeepOpen bool     // with Terminal: keep the window open afterwards
}

// RunCommand returns the command Run would start.
func (l *Launcher) RunCommand(spec RunSpec) (Command, error) {
	if spec.Program == "" {
		return Command{}, errors.New("launch: no program given")
	}
	if spec.Admin && !l.windows() {
		return Command{}, ErrUnsupported
	}
	program := l.ExpandEnv(spec.Program)
	if spec.Terminal {
		line := ""
		if l.windows() {
			line = QuoteWindowsArg(program)
			if spec.Params != "" {
				line += " " + spec.Params
			}
		} else {
			args, err := SplitArgs(spec.Params)
			if err != nil {
				return Command{}, err
			}
			q := []string{ShellQuote(program)}
			for _, a := range args {
				q = append(q, ShellQuote(a))
			}
			line = strings.Join(q, " ")
		}
		c, err := l.RunInTerminalCommand(spec.Dir, line, spec.KeepOpen)
		c.Cleanup = append(c.Cleanup, spec.Cleanup...)
		return c, err
	}
	// The file the program name refers to, when this Launcher runs on the system it describes.
	resolved := ""
	if l.OS == runtime.GOOS {
		resolved = l.resolveFile(program, spec.Dir)
	}
	// A macOS application bundle is started with open, which passes the parameters on after --args. A bare name
	// that is neither a file here nor a program in PATH is taken as an application name ("BBEdit", "TextEdit").
	appName := l.darwin() && resolved == "" && !strings.Contains(program, "/") && l.LookPath != nil && !l.has(program)
	if l.darwin() && (appName || strings.HasSuffix(strings.TrimRight(program, "/"), ".app")) {
		args, err := SplitArgs(spec.Params)
		if err != nil {
			return Command{}, err
		}
		app := program
		if resolved != "" {
			app = resolved
		}
		c := Command{Path: "open", Args: []string{"-a", app}, Dir: spec.Dir, Cleanup: spec.Cleanup}
		if len(args) > 0 {
			c.Args = append(append(c.Args, "--args"), args...)
		}
		return c, nil
	}
	// A document (not a program) is opened with its app, as ShellExecute would do it.
	if resolved != "" && l.isDocument(resolved) {
		c := l.OpenDefaultCommand(resolved)
		c.Dir = spec.Dir
		c.Cleanup = spec.Cleanup
		return c, nil
	}
	if resolved != "" {
		program = resolved
	}
	c, err := l.CommandFor(program, spec.Params, spec.Dir)
	if err != nil {
		return Command{}, err
	}
	c.Show = spec.Show
	c.Cleanup = spec.Cleanup
	if spec.Admin {
		c.Verb = "runas"
	}
	if l.windows() && resolved != "" && ExecKindOf(resolved) == KindConsole {
		c.NewConsole = true
	}
	return c, nil
}

// Run starts a program as described by spec.
func (l *Launcher) Run(spec RunSpec) error {
	c, err := l.RunCommand(spec)
	if err != nil {
		removeAll(spec.Cleanup)
		return err
	}
	return l.start(c)
}

// resolveFile finds the file a program name refers to: relative to dir first (Total Commander runs files in the
// current folder), then in PATH. It returns "" when there is no such file.
func (l *Launcher) resolveFile(name, dir string) string {
	if name == "" {
		return ""
	}
	if filepath.IsAbs(name) || strings.ContainsAny(name, `/\`) {
		p := name
		if !filepath.IsAbs(p) && dir != "" {
			p = filepath.Join(dir, p)
		}
		if fileExists(p) {
			return p
		}
		return ""
	}
	if dir != "" {
		if p := filepath.Join(dir, name); fileExists(p) {
			return p
		}
		if l.windows() {
			for _, ext := range windowsExecExts(l.getenv("PATHEXT")) {
				if p := filepath.Join(dir, name+ext); fileExists(p) {
					return p
				}
			}
		}
	}
	if l.LookPath != nil {
		if p, err := l.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func windowsExecExts(pathext string) []string {
	if pathext == "" {
		pathext = ".COM;.EXE;.BAT;.CMD"
	}
	var out []string
	for _, e := range strings.Split(pathext, ";") {
		if e = strings.TrimSpace(e); e != "" {
			out = append(out, strings.ToLower(e))
		}
	}
	return out
}

// isDocument says whether p is a file to open with its app rather than a program to run.
func (l *Launcher) isDocument(p string) bool {
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		return false
	}
	if l.windows() {
		ext := strings.ToLower(filepath.Ext(p))
		for _, e := range windowsExecExts(l.getenv("PATHEXT")) {
			if ext == e {
				return false
			}
		}
		return true
	}
	return fi.Mode()&0o111 == 0
}

// LineMode chooses how RunLine runs a program that is not a document.
type LineMode int

const (
	LineAuto     LineMode = iota // GUI programs on their own, console programs and shell commands in a terminal
	LineTerminal                 // always in a terminal window
	LineDetached                 // never in a terminal: start the program (or the shell) without a window
)

// RunLineCommand returns the command RunLine would start. line must already be a LineRun line (see ParseLine).
func (l *Launcher) RunLineCommand(line, dir string, keepOpen bool, mode LineMode) (Command, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return Command{}, errors.New("launch: empty command line")
	}
	if keepOpen {
		mode = LineTerminal
	}
	var first, rest string
	if l.windows() {
		first, rest = splitWindowsProgram(line)
	} else {
		words, err := SplitArgs(line)
		if err != nil || len(words) == 0 || hasShellSyntax(line) {
			// Pipes, redirections, variables: only a shell can run this.
			return l.shellLine(line, dir, keepOpen, mode)
		}
		first = words[0]
		rest = strings.TrimSpace(line[len(firstWordRaw(line)):])
	}
	real := l.OS == runtime.GOOS
	path := ""
	if real {
		path = l.resolveFile(first, dir)
	}
	if path == "" {
		// Not a file: a shell built-in (dir, cd /d, export) or a mistake the shell will report.
		return l.shellLine(line, dir, keepOpen, mode)
	}
	if l.darwin() && strings.HasSuffix(strings.TrimRight(path, "/"), ".app") {
		return l.RunCommand(RunSpec{Program: path, Params: rest, Dir: dir})
	}
	if l.isDocument(path) {
		c := l.OpenDefaultCommand(path)
		c.Dir = dir
		return c, nil
	}
	kind := ExecKindOf(path)
	if mode == LineTerminal || (mode == LineAuto && kind != KindGUI && !l.windows()) {
		return l.RunInTerminalCommand(dir, line, keepOpen)
	}
	c, err := l.CommandFor(path, rest, dir)
	if err != nil {
		return Command{}, err
	}
	if l.windows() && kind == KindConsole {
		c.NewConsole = mode != LineDetached
		if mode == LineDetached {
			c.Show = ShowHidden
		}
	}
	return c, nil
}

// RunLine runs a command line typed in dir: Enter (keepOpen false) or Shift+Enter (keepOpen true).
func (l *Launcher) RunLine(line, dir string, keepOpen bool, mode LineMode) error {
	c, err := l.RunLineCommand(line, dir, keepOpen, mode)
	if err != nil {
		return err
	}
	return l.start(c)
}

// shellLine runs the line through the shell: in a terminal, or without a window in detached mode.
func (l *Launcher) shellLine(line, dir string, keepOpen bool, mode LineMode) (Command, error) {
	if mode == LineDetached {
		return l.shellCommand(line, dir), nil
	}
	return l.RunInTerminalCommand(dir, line, keepOpen)
}

// shellCommand is the command that lets the system shell run line in dir without a terminal window.
func (l *Launcher) shellCommand(line, dir string) Command {
	if l.windows() {
		return Command{Path: "cmd.exe", Params: "/c " + line, Dir: dir, Show: ShowHidden}
	}
	return Command{Path: "/bin/sh", Args: []string{"-c", line}, Dir: dir}
}

// CaptureLine runs a command line through the shell in dir, waits, and returns what it printed (stdout and stderr
// together). It is the "show output in the app" option of the command line.
func (l *Launcher) CaptureLine(ctx context.Context, line, dir string) ([]byte, error) {
	if l.Output == nil {
		return nil, errors.New("launch: Launcher has no Output function (use launch.New)")
	}
	return l.Output(ctx, l.shellCommand(line, dir))
}

// hasShellSyntax spots characters that only a shell understands, outside quotes.
func hasShellSyntax(line string) bool {
	quote := byte(0)
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			} else if c == '\\' && quote == '"' {
				i++
			}
		case c == '\\':
			i++
		case c == '"' || c == '\'':
			quote = c
		case strings.IndexByte("|&;<>$`*?(){}[]~#", c) >= 0:
			return true
		}
	}
	return false
}

// firstWordRaw returns the first word of line exactly as typed, quotes and escapes included.
func firstWordRaw(line string) string {
	quote := byte(0)
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\\':
			i++
		case c == '"' || c == '\'':
			quote = c
		case c == ' ' || c == '\t':
			return line[:i]
		}
	}
	return line
}
