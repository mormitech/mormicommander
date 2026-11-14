// Turning a button-bar button or a start-menu entry into a launch request.
// Both have the same four fields in Total Commander: the command (a program
// or a cm_/em_ internal command), the parameters (a placeholder template),
// the start path, and the window state stored as "iconic" (1 = minimised,
// -1 = maximised, 0 = normal). A command written with a leading "*" asks to
// run elevated, which only Windows can honour. PrepareButton does all the
// expansion and splitting so the UI only has to show the "?" prompt when
// asked, start the process, and call Cleanup when it exits.
package params

import (
	"path/filepath"
	"strings"
)

// ShowMode is the window state a program is started in.
type ShowMode int

const (
	// ShowNormal starts the program in a normal window.
	ShowNormal ShowMode = iota
	// ShowMinimized starts it minimised (TC "Run minimized", iconic=1).
	ShowMinimized
	// ShowMaximized starts it maximised (iconic=-1).
	ShowMaximized
)

// ShowModeFromIconic converts TC's iconic value (bar files, usercmd.ini,
// [user] menu) to a ShowMode.
func ShowModeFromIconic(n int) ShowMode {
	switch {
	case n > 0:
		return ShowMinimized
	case n < 0:
		return ShowMaximized
	}
	return ShowNormal
}

// Iconic converts a ShowMode back to TC's iconic value.
func (m ShowMode) Iconic() int {
	switch m {
	case ShowMinimized:
		return 1
	case ShowMaximized:
		return -1
	}
	return 0
}

// Button is a button-bar button or a start-menu entry.
type Button struct {
	Command   string // program, cm_/em_ command; leading "*" = elevated
	Params    string // placeholder template; leading "?" = prompt
	StartPath string // working folder template; "" = source folder
	Iconic    int    // 1 minimised, -1 maximised, 0 normal
}

// Launch is a fully expanded button, ready to run.
type Launch struct {
	// Internal is true when Program is a cm_ or em_ command name.
	Internal bool
	// Program is the expanded command (without the "*").
	Program string
	// Line is the expanded parameter text; Args is Line split.
	Line string
	Args []string
	// Dir is the working folder.
	Dir string
	// Show is the window state.
	Show ShowMode
	// Elevate asks for administrator rights (Windows only).
	Elevate bool
	// Prompt asks the UI to let the user edit Line first (SetLine).
	Prompt bool
	// Canceled is true when the user canceled a %[...] question; nothing
	// should run.
	Canceled bool
	// ListFiles must be deleted after the program exits (Cleanup).
	ListFiles []string

	style Style
}

// IsInternalCommand reports whether name is a cm_ or em_ command name.
func IsInternalCommand(name string) bool {
	l := strings.ToLower(name)
	return len(l) > 3 && (strings.HasPrefix(l, "cm_") || strings.HasPrefix(l, "em_"))
}

// PrepareButton expands a button for the given panel state.
func PrepareButton(b Button, ctx Context) (Launch, error) {
	st := ctx.style()
	l := Launch{Show: ShowModeFromIconic(b.Iconic), style: st}
	env := ctx.Env
	env.Percent = true
	cmd := strings.TrimSpace(b.Command)
	if strings.HasPrefix(cmd, "*") {
		l.Elevate = true
		cmd = strings.TrimSpace(cmd[1:])
	}
	cmd = ExpandEnv(cmd, env)
	if len(cmd) >= 2 && cmd[0] == '"' && cmd[len(cmd)-1] == '"' {
		cmd = cmd[1 : len(cmd)-1]
	}
	l.Program = cmd
	l.Internal = IsInternalCommand(cmd)

	if st == StyleWindows {
		ext := strings.ToLower(filepath.Ext(cmd))
		ctx.Batch = ext == ".bat" || ext == ".cmd"
	}
	r, err := Expand(b.Params, ctx)
	l.ListFiles = r.ListFiles
	l.Prompt = r.Prompt
	l.Canceled = r.Canceled
	if err != nil || r.Canceled {
		return l, err
	}
	dir, err := ExpandPath(b.StartPath, ctx)
	if err != nil {
		l.Cleanup()
		return l, err
	}
	if strings.TrimSpace(dir) == "" {
		dir = ctx.Source.Path
	}
	l.Dir = dir
	if err := l.SetLine(r.Params); err != nil {
		l.Cleanup()
		return l, err
	}
	return l, nil
}

// SetLine replaces the parameter text (after the "?" prompt) and splits it
// again.
func (l *Launch) SetLine(line string) error {
	args, err := SplitArgs(line, l.style)
	if err != nil {
		return err
	}
	l.Line, l.Args = line, args
	return nil
}

// Cleanup deletes the list files.
func (l *Launch) Cleanup() error {
	r := Result{ListFiles: l.ListFiles}
	err := r.Cleanup()
	l.ListFiles = nil
	return err
}
