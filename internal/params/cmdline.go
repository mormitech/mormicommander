// The command-line box under the panels. In Total Commander most of what
// you type there is simply run with the active panel's folder as working
// directory, but a few things are handled by TC itself because a child
// process could never change the panel: "cd" in all its spellings (cd ..,
// cd.., cd \, cd /d D:\x, cd *.txt to set a file filter, a bare drive "D:"),
// internal command names (cm_Copy, em_mycommand), and "cmd", which opens a
// terminal in the current folder. ParseLine classifies a line into an
// Action; the UI performs it. Lines with shell syntax (pipes, redirects,
// "&&", globs on POSIX) are run through the shell, everything else is split
// into arguments and started directly so names with spaces never get
// re-interpreted. We also accept the shell habits Mac and Linux users have:
// "cd" alone and "cd ~" go home, "cd -" goes back to the previous folder.
package params

import (
	"strings"
)

// ActionKind says what a command line asks for.
type ActionKind int

const (
	// ActNone means nothing to do (empty line, "cd" on Windows).
	ActNone ActionKind = iota
	// ActChangeDir changes the active panel folder and/or file filter.
	ActChangeDir
	// ActInternal runs a cm_ or em_ command.
	ActInternal
	// ActOpenTerminal opens a terminal window in the current folder.
	ActOpenTerminal
	// ActRun starts a program.
	ActRun
)

// Action is a parsed command line.
type Action struct {
	Kind ActionKind

	// ActChangeDir: Path is the new folder ("" = stay), Back asks for the
	// previous folder ("cd -"), Filter is a wildcard filter to apply
	// ("cd *.txt"), ClearFilter removes the filter ("cd *" / "cd *.*").
	Path        string
	Back        bool
	Filter      string
	ClearFilter bool

	// ActInternal / ActRun: Command is the command name or program, Args
	// its arguments (not set when Shell is true).
	Command string
	Args    []string

	// ActRun: Line is the expanded line; Shell means run Line through the
	// system shell (sh -c / cmd /c). KeepOpen keeps the terminal open
	// (Shift+Enter, F-294). Elevate asks for admin rights ("*" prefix,
	// Windows only). Dir is the working folder.
	Line     string
	Shell    bool
	KeepOpen bool
	Elevate  bool
	Dir      string
}

// LineContext is the state ParseLine needs.
type LineContext struct {
	// Cwd is the active panel folder.
	Cwd string
	// Home is the user's home folder (for "cd", "cd ~").
	Home string
	// Style selects path and quoting rules; zero means DefaultStyle.
	Style Style
	// Env controls variable expansion; zero means DefaultEnvOptions.
	Env *EnvOptions
}

func (c LineContext) env() EnvOptions {
	if c.Env != nil {
		e := *c.Env
		return e
	}
	return DefaultEnvOptions(c.Style)
}

// ParseLine classifies a command-line box entry. keepOpen reflects
// Shift+Enter.
func ParseLine(line string, ctx LineContext, keepOpen bool) (Action, error) {
	st := ctx.Style.orDefault()
	ctx.Style = st
	t := strings.TrimSpace(line)
	if t == "" {
		return Action{Kind: ActNone}, nil
	}
	if !hasShellMeta(t, st, false) {
		if a, ok := parseCD(t, ctx); ok {
			return a, nil
		}
	}
	// Bare drive "D:" switches drive (Windows).
	if st == StyleWindows && len(t) == 2 && t[1] == ':' && isLetter(t[0]) {
		return Action{Kind: ActChangeDir, Path: strings.ToUpper(t[:1]) + `:\`}, nil
	}
	if strings.EqualFold(t, "cmd") || strings.EqualFold(t, "cmd.exe") {
		return Action{Kind: ActOpenTerminal, Dir: ctx.Cwd}, nil
	}
	a := Action{Kind: ActRun, KeepOpen: keepOpen, Dir: ctx.Cwd}
	if st == StyleWindows && strings.HasPrefix(t, "*") {
		a.Elevate = true
		t = strings.TrimSpace(t[1:])
	}
	env := ctx.env()
	if hasShellMeta(t, st, true) {
		// The shell expands $VARs itself; we only add %VAR% support.
		env.Dollar = false
		a.Shell = true
		a.Line = ExpandEnv(t, env)
		a.Command = firstWord(a.Line)
		return a, nil
	}
	a.Line = ExpandEnv(t, env)
	args, err := SplitArgs(a.Line, st)
	if err != nil {
		return Action{}, err
	}
	if len(args) == 0 {
		return Action{Kind: ActNone}, nil
	}
	a.Command = args[0]
	if len(args) > 1 {
		a.Args = args[1:]
	}
	if IsInternalCommand(a.Command) {
		return Action{Kind: ActInternal, Command: a.Command, Args: a.Args, Line: a.Line}, nil
	}
	return a, nil
}

func firstWord(s string) string {
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i]
	}
	return s
}

// hasShellMeta reports whether the line uses shell syntax outside quotes;
// globs adds POSIX wildcard and grouping characters to the check.
func hasShellMeta(s string, st Style, globs bool) bool {
	meta := "|<>&"
	if st == StylePOSIX {
		meta = "|<>&;`"
		if globs {
			meta += "*?[(){}"
		}
	}
	var q byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if q != 0 {
			if c == q {
				q = 0
			} else if st == StylePOSIX && q == '"' && (c == '`' || c == '$' && i+1 < len(s) && s[i+1] == '(') {
				return true
			}
			continue
		}
		if c == '"' || st == StylePOSIX && c == '\'' {
			q = c
			continue
		}
		if st == StylePOSIX && c == '\\' {
			i++
			continue
		}
		if st == StyleWindows && c == '^' {
			return true
		}
		if st == StylePOSIX && c == '$' && i+1 < len(s) && s[i+1] == '(' {
			return true
		}
		if strings.IndexByte(meta, c) >= 0 {
			return true
		}
	}
	return false
}

// parseCD recognises the cd command and its spellings.
func parseCD(t string, ctx LineContext) (Action, bool) {
	st := ctx.Style
	if len(t) < 2 || !strings.EqualFold(t[:2], "cd") {
		return Action{}, false
	}
	rest := t[2:]
	switch {
	case rest == "":
	case rest[0] == ' ' || rest[0] == '\t':
		rest = strings.TrimSpace(rest)
	case rest[0] == '.' || rest[0] == '\\' || rest[0] == '/':
		// cmd.exe style "cd.." and "cd\".
	default:
		return Action{}, false
	}
	if st == StyleWindows && len(rest) >= 2 && strings.EqualFold(rest[:2], "/d") &&
		(len(rest) == 2 || rest[2] == ' ') {
		rest = strings.TrimSpace(rest[2:])
	}
	arg := unquoteArg(ExpandEnv(rest, ctx.env()), st)
	a := Action{Kind: ActChangeDir}
	switch {
	case arg == "":
		if st == StyleWindows || ctx.Home == "" {
			return Action{Kind: ActNone}, true
		}
		a.Path = ctx.Home
		return a, true
	case arg == "-":
		a.Back = true
		return a, true
	case arg == "~" || strings.HasPrefix(arg, "~/") || strings.HasPrefix(arg, `~\`):
		arg = ctx.Home + arg[1:]
	case arg == `\` || arg == "/":
		a.Path = RootOf(ctx.Cwd, st)
		return a, true
	}
	arg = expandDots(arg)
	// A wildcard in the last element sets a filter.
	dir, last := splitLast(arg)
	if strings.ContainsAny(last, "*?") {
		if last == "*" || last == "*.*" {
			a.ClearFilter = true
		} else {
			a.Filter = last
		}
		arg = dir
		if arg == "" {
			return a, true
		}
	}
	if st == StyleWindows && len(arg) == 2 && arg[1] == ':' && isLetter(arg[0]) {
		arg += `\`
	}
	if IsVirtual(arg) {
		a.Path = CleanPath(arg, st)
	} else {
		a.Path = JoinPath(ctx.Cwd, arg, st)
	}
	return a, true
}

// unquoteArg removes the quoting of a single cd argument. Unquoted spaces
// are kept as part of the path, as cmd.exe and TC do ("cd Program Files").
func unquoteArg(s string, st Style) string {
	if s == "" || !strings.ContainsAny(s, `"'\`) {
		return s
	}
	if st == StyleWindows {
		return strings.ReplaceAll(s, `"`, "")
	}
	if args, err := SplitArgs(s, st); err == nil && len(args) == 1 {
		return args[0]
	}
	return s
}

// expandDots turns "..." into "../.." and "...." into "../../.." (4DOS
// style) for each path element made only of three or more dots.
func expandDots(p string) string {
	if !strings.Contains(p, "...") {
		return p
	}
	sep := "/"
	if strings.Contains(p, `\`) {
		sep = `\`
	}
	parts := strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' })
	changed := false
	for i, el := range parts {
		if len(el) >= 3 && strings.Trim(el, ".") == "" {
			parts[i] = strings.TrimSuffix(strings.Repeat(".."+sep, len(el)-1), sep)
			changed = true
		}
	}
	if !changed {
		return p
	}
	out := strings.Join(parts, sep)
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) {
		out = sep + out
	}
	return out
}

// splitLast splits off the last path element.
func splitLast(p string) (dir, last string) {
	i := strings.LastIndexAny(p, `/\`)
	if i < 0 {
		return "", p
	}
	if i == 0 {
		return p[:1], p[1:]
	}
	if i == 2 && p[1] == ':' {
		return p[:3], p[3:]
	}
	return p[:i], p[i+1:]
}
