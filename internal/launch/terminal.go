// Terminals: "open a terminal here" (cm_ExecuteDOS, F-196) and running a command line in a terminal window that
// stays open (Shift+Enter, F-199/F-294). Every OS does this differently. macOS opens Terminal (or iTerm, or any
// app the user picks) with `open -a App folder`, and runs commands through a small self-deleting .command script so
// no AppleScript automation permission is needed. Windows uses Windows Terminal when it is installed, otherwise
// cmd.exe in a new console, and runs commands with `cmd /k` (keep open) or `cmd /c`. Linux has no single terminal,
// so a table lists the well-known ones with the options each uses for the start folder and for the command to run;
// $TERMINAL wins, then the desktop's own terminal, then x-terminal-emulator, then the rest of the table.
package launch

import (
	"os"
	"strings"
)

// linuxTerminal describes how one Linux terminal program takes a start folder and a command.
type linuxTerminal struct {
	name    string
	dirArgs func(dir string) []string // options that set the start folder (nil: only the working folder is set)
	exec    []string                  // options in front of the command's argument list
	asText  bool                      // the command is passed as one shell-quoted string after exec
}

func eqArg(flag string) func(string) []string {
	return func(d string) []string { return []string{flag + "=" + d} }
}

func sepArg(flag string) func(string) []string {
	return func(d string) []string { return []string{flag, d} }
}

// linuxTerminals is the probe order after $TERMINAL and the desktop's preference.
var linuxTerminals = []linuxTerminal{
	{name: "x-terminal-emulator", exec: []string{"-e"}},
	{name: "ptyxis", dirArgs: func(d string) []string { return []string{"--new-window", "--working-directory=" + d} }, exec: []string{"--"}},
	{name: "kgx", dirArgs: eqArg("--working-directory"), exec: []string{"--"}},
	{name: "gnome-terminal", dirArgs: eqArg("--working-directory"), exec: []string{"--"}},
	{name: "konsole", dirArgs: sepArg("--workdir"), exec: []string{"-e"}},
	{name: "xfce4-terminal", dirArgs: eqArg("--working-directory"), exec: []string{"-x"}},
	{name: "mate-terminal", dirArgs: eqArg("--working-directory"), exec: []string{"-x"}},
	{name: "qterminal", dirArgs: sepArg("--workdir"), exec: []string{"-e"}, asText: true},
	{name: "lxterminal", dirArgs: eqArg("--working-directory"), exec: []string{"-e"}, asText: true},
	{name: "tilix", dirArgs: eqArg("--working-directory"), exec: []string{"-e"}, asText: true},
	{name: "terminator", dirArgs: eqArg("--working-directory"), exec: []string{"-x"}},
	{name: "kitty", dirArgs: sepArg("--directory")},
	{name: "alacritty", dirArgs: sepArg("--working-directory"), exec: []string{"-e"}},
	{name: "wezterm", dirArgs: func(d string) []string { return []string{"start", "--cwd", d} }, exec: []string{"--"}},
	{name: "foot", dirArgs: eqArg("--working-directory")},
	{name: "urxvt", dirArgs: sepArg("-cd"), exec: []string{"-e"}},
	{name: "st", dirArgs: sepArg("-d"), exec: []string{"-e"}},
	{name: "xterm", exec: []string{"-e"}},
}

func lookupLinuxTerminal(name string) (linuxTerminal, bool) {
	base := baseName("linux", name)
	for _, t := range linuxTerminals {
		if t.name == base {
			t.name = name
			return t, true
		}
	}
	return linuxTerminal{}, false
}

// desktopTerminals maps XDG_CURRENT_DESKTOP values to their own terminals, tried before the generic order.
var desktopTerminals = map[string][]string{
	"GNOME":      {"ptyxis", "kgx", "gnome-terminal"},
	"UNITY":      {"gnome-terminal"},
	"KDE":        {"konsole"},
	"XFCE":       {"xfce4-terminal"},
	"MATE":       {"mate-terminal"},
	"LXQT":       {"qterminal"},
	"LXDE":       {"lxterminal"},
	"X-CINNAMON": {"gnome-terminal"},
	"CINNAMON":   {"gnome-terminal"},
}

// pickLinuxTerminal finds the terminal to use on Linux.
func (l *Launcher) pickLinuxTerminal() (linuxTerminal, error) {
	var order []string
	if l.Terminal != "" {
		order = append(order, l.Terminal)
	}
	if t := l.getenv("TERMINAL"); t != "" {
		order = append(order, t)
	}
	for _, d := range strings.Split(l.getenv("XDG_CURRENT_DESKTOP"), ":") {
		order = append(order, desktopTerminals[strings.ToUpper(d)]...)
	}
	for _, t := range linuxTerminals {
		order = append(order, t.name)
	}
	for _, name := range order {
		if !l.has(name) {
			continue
		}
		if t, ok := lookupLinuxTerminal(name); ok {
			return t, nil
		}
		// An unknown terminal from $TERMINAL: most accept -e followed by the command.
		return linuxTerminal{name: name, exec: []string{"-e"}}, nil
	}
	return linuxTerminal{}, ErrNoTerminal
}

// customTerminal reports whether Launcher.Terminal is a command line template (it contains %D).
func (l *Launcher) customTerminal() bool { return strings.Contains(l.Terminal, "%D") }

// OpenTerminalCommand returns the command that opens a terminal window in dir.
func (l *Launcher) OpenTerminalCommand(dir string) (Command, error) {
	if l.customTerminal() {
		if l.windows() {
			prog, params := splitWindowsProgram(l.Terminal)
			return Command{Path: prog, Params: strings.ReplaceAll(params, "%D", QuoteWindowsArg(dir)), Dir: dir, NewConsole: true}, nil
		}
		words, err := SplitArgs(l.Terminal)
		if err != nil || len(words) == 0 {
			return Command{}, ErrNoTerminal
		}
		for i := range words {
			words[i] = strings.ReplaceAll(words[i], "%D", dir)
		}
		return Command{Path: words[0], Args: words[1:], Dir: dir}, nil
	}
	switch {
	case l.darwin():
		app := l.Terminal
		if app == "" {
			app = "Terminal"
		}
		return Command{Path: "open", Args: []string{"-a", app, safeArg(dir)}, Dir: dir}, nil
	case l.windows():
		switch strings.ToLower(l.Terminal) {
		case "", "wt", "wt.exe":
			if l.Terminal != "" || l.has("wt.exe") {
				return Command{Path: "wt.exe", Params: "-d " + QuoteWindowsArg(dir), Dir: dir}, nil
			}
			return Command{Path: "cmd.exe", Params: "/k", Dir: dir, NewConsole: true}, nil
		case "powershell", "powershell.exe":
			return Command{Path: "powershell.exe", Params: "-NoExit", Dir: dir, NewConsole: true}, nil
		case "pwsh", "pwsh.exe":
			return Command{Path: "pwsh.exe", Params: "-NoExit", Dir: dir, NewConsole: true}, nil
		case "cmd", "cmd.exe":
			return Command{Path: "cmd.exe", Params: "/k", Dir: dir, NewConsole: true}, nil
		default:
			return Command{Path: l.Terminal, Dir: dir, NewConsole: true}, nil
		}
	default:
		t, err := l.pickLinuxTerminal()
		if err != nil {
			return Command{}, err
		}
		c := Command{Path: t.name, Dir: dir}
		if t.dirArgs != nil {
			c.Args = t.dirArgs(dir)
		}
		return c, nil
	}
}

// OpenTerminal opens a terminal window in dir (cm_ExecuteDOS).
func (l *Launcher) OpenTerminal(dir string) error {
	c, err := l.OpenTerminalCommand(dir)
	if err != nil {
		return err
	}
	return l.start(c)
}

// RunInTerminalCommand returns the command that runs a command line (shell syntax on macOS and Linux, cmd syntax
// on Windows) in a new terminal window in dir. With keepOpen the window stays open with an interactive shell after
// the command ends, as cmd /k does; without it the window closes (or, on macOS, shows that the process completed).
func (l *Launcher) RunInTerminalCommand(dir, line string, keepOpen bool) (Command, error) {
	if l.windows() {
		sw := "/c "
		if keepOpen {
			sw = "/k "
		}
		return Command{Path: "cmd.exe", Params: sw + line, Dir: dir, NewConsole: true}, nil
	}
	script := "cd -- " + ShellQuote(dir) + " || exit 1\n" + line + "\n"
	if keepOpen {
		script += `exec "${SHELL:-/bin/sh}" -i` + "\n"
	}
	if l.darwin() {
		f, err := os.CreateTemp(l.tempDir(), "mormi*.command")
		if err != nil {
			return Command{}, err
		}
		// The script deletes itself as its first act, so nothing is left behind however the window ends.
		body := "#!/bin/sh\nrm -f \"$0\"\n" + script
		_, werr := f.WriteString(body)
		cerr := f.Close()
		if werr == nil {
			werr = cerr
		}
		if werr == nil {
			werr = os.Chmod(f.Name(), 0o700)
		}
		if werr != nil {
			os.Remove(f.Name())
			return Command{}, werr
		}
		app := l.Terminal
		if app == "" || l.customTerminal() {
			app = "Terminal"
		}
		return Command{Path: "open", Args: []string{"-a", app, f.Name()}, Dir: dir}, nil
	}
	return l.terminalArgvCommand(dir, []string{"/bin/sh", "-c", script})
}

// RunInTerminal runs a command line in a new terminal window in dir.
func (l *Launcher) RunInTerminal(dir, line string, keepOpen bool) error {
	c, err := l.RunInTerminalCommand(dir, line, keepOpen)
	if err != nil {
		return err
	}
	return l.start(c)
}

// terminalArgvCommand runs an argument list inside a terminal window (Linux and the BSDs; desktop entries with
// Terminal=true use it too).
func (l *Launcher) terminalArgvCommand(dir string, argv []string) (Command, error) {
	if l.darwin() || l.windows() {
		line := make([]string, len(argv))
		for i, a := range argv {
			if l.windows() {
				line[i] = QuoteWindowsArg(a)
			} else {
				line[i] = ShellQuote(a)
			}
		}
		return l.RunInTerminalCommand(dir, strings.Join(line, " "), false)
	}
	if l.customTerminal() {
		c, err := l.OpenTerminalCommand(dir)
		if err != nil {
			return Command{}, err
		}
		c.Args = append(append(c.Args, "-e"), argv...)
		return c, nil
	}
	t, err := l.pickLinuxTerminal()
	if err != nil {
		return Command{}, err
	}
	c := Command{Path: t.name, Dir: dir}
	if t.dirArgs != nil && dir != "" {
		c.Args = append(c.Args, t.dirArgs(dir)...)
	}
	c.Args = append(c.Args, t.exec...)
	if t.asText {
		q := make([]string, len(argv))
		for i, a := range argv {
			q[i] = ShellQuote(a)
		}
		c.Args = append(c.Args, strings.Join(q, " "))
	} else {
		c.Args = append(c.Args, argv...)
	}
	return c, nil
}
