// Package launch starts other programs on behalf of the file manager: it opens a file with the app the system uses for
// it, offers "Open with…", runs programs with parameters and a working folder, starts the configured external viewer
// and editor (F3/F4), opens a terminal in the current folder, reveals a file in Finder or Explorer, and applies the
// app's own internal file associations before the system ones. It is the engine behind Total Commander's Enter on a
// file, the command line, cm_ExecuteDOS, cm_Explorer and cm_InternalAssociate.
//
// Every action is first turned into a Command value, which describes the program, its arguments and its folder
// without running anything, and only then handed to the Launcher's Start function. That split exists for two
// reasons: tests can check exactly what would be run on macOS, Windows and Linux from any machine (by setting
// Launcher.OS and a fake Start), and the UI can show the user what is about to happen. File names are always passed
// as separate arguments (or, on Windows, carefully quoted into the raw command line Windows programs expect), never
// pasted into shell script text, so no file name can turn into a command.
package launch

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ErrUnsupported is returned when an action has no way to work on the current platform (for example the system
// "Open with" chooser on macOS, which only Finder can show).
var ErrUnsupported = errors.New("launch: not supported on this platform")

// ErrNoTerminal is returned on Linux when no known terminal program could be found.
var ErrNoTerminal = errors.New("launch: no terminal program found")

// ShowMode says how the window of a started program should appear. It matches the "Run minimized/maximized" choice
// of Total Commander's buttons and start menu items. Minimized and maximized are only honoured on Windows; hidden
// windows are honoured on Windows (HideWindow) and ignored elsewhere, where helper programs have no window anyway.
type ShowMode int

const (
	ShowNormal ShowMode = iota
	ShowMinimized
	ShowMaximized
	ShowHidden
)

// Command is one program start, fully described but not yet run.
//
// On macOS and Linux the program gets Args as its argument list. On Windows a program receives one raw command line
// and parses it itself (rundll32, explorer and cmd each do it differently), so Windows commands carry Params, the raw
// text after the program name, and the program name is quoted in front of it when the process starts.
type Command struct {
	Path       string   // program name or path; names are looked up in PATH when the process starts
	Args       []string // argument list (macOS, Linux)
	Params     string   // raw parameter text (Windows)
	Dir        string   // working folder; empty means the folder of this process
	Env        []string // extra environment entries "NAME=value", added to this process's environment
	Show       ShowMode // window state of the started program
	NewConsole bool     // Windows: give a console program its own console window
	Verb       string   // Windows: ShellExecute verb such as "runas" (run as administrator); empty for a normal start
	Cleanup    []string // temporary files (list files of %L and friends) to delete when the program has quit
}

// String gives a readable one-line form of the command, used in logs, tests and confirmation prompts.
func (c Command) String() string {
	var b strings.Builder
	if c.Params != "" {
		b.WriteString(QuoteWindowsArg(c.Path))
	} else {
		b.WriteString(ShellQuote(c.Path))
	}
	if c.Params != "" {
		b.WriteByte(' ')
		b.WriteString(c.Params)
	}
	for _, a := range c.Args {
		b.WriteByte(' ')
		b.WriteString(ShellQuote(a))
	}
	return b.String()
}

// Launcher turns actions into Commands and starts them. The zero value is not usable; call New. Every field may be
// replaced after New, which is how tests pretend to be another operating system and capture commands instead of
// running them.
type Launcher struct {
	// OS is the operating system whose conventions are used ("darwin", "windows", "linux", …). It defaults to
	// runtime.GOOS.
	OS string
	// Start runs a command without waiting for it to finish. The default starts a real process and deletes the
	// command's Cleanup files after it quits.
	Start func(Command) error
	// Output runs a command, waits for it and returns its combined output (used for captured command-line output).
	Output func(context.Context, Command) ([]byte, error)
	// Capture runs a command with no input, waits for it and returns at most limit bytes of its output; cancelling the
	// context stops it (captured command-line output with a Stop button, capture.go). Nil means the real one.
	Capture func(ctx context.Context, c Command, limit int) (CaptureResult, error)
	// LookPath finds a program in PATH, like exec.LookPath.
	LookPath func(string) (string, error)
	// LookupEnv reads an environment variable, like os.LookupEnv.
	LookupEnv func(string) (string, bool)
	// TempDir is where list files and helper scripts are written; empty means os.TempDir().
	TempDir string
	// Terminal is the preferred terminal: empty for the platform default, a known program name ("iTerm",
	// "kitty", "wt", "powershell", …) or a custom command line in which %D is replaced by the folder.
	Terminal string
}

// New returns a Launcher for the real system.
func New() *Launcher {
	return &Launcher{
		OS:        runtime.GOOS,
		Start:     startProcess,
		Output:    outputProcess,
		LookPath:  exec.LookPath,
		LookupEnv: os.LookupEnv,
	}
}

func (l *Launcher) getenv(name string) string {
	if l.LookupEnv == nil {
		return ""
	}
	v, _ := l.LookupEnv(name)
	return v
}

func (l *Launcher) tempDir() string {
	if l.TempDir != "" {
		return l.TempDir
	}
	return os.TempDir()
}

func (l *Launcher) windows() bool { return l.OS == "windows" }
func (l *Launcher) darwin() bool  { return l.OS == "darwin" }

func (l *Launcher) start(c Command) error {
	if l.Start == nil {
		return errors.New("launch: Launcher has no Start function (use launch.New)")
	}
	err := l.Start(c)
	if err != nil {
		removeAll(c.Cleanup)
	}
	return err
}

func (l *Launcher) has(name string) bool {
	if l.LookPath == nil {
		return false
	}
	_, err := l.LookPath(name)
	return err == nil
}

// buildExec makes the exec.Cmd for a command, with the platform's process attributes applied.
func buildExec(ctx context.Context, c Command) *exec.Cmd {
	var cmd *exec.Cmd
	if ctx != nil {
		cmd = exec.CommandContext(ctx, c.Path, c.Args...)
	} else {
		cmd = exec.Command(c.Path, c.Args...)
	}
	cmd.Dir = c.Dir
	if len(c.Env) > 0 {
		cmd.Env = append(os.Environ(), c.Env...)
	}
	return cmd
}

// startProcess is the default Start: it starts the program detached from this process's terminal session, and
// deletes the command's temporary files once the program has quit, the way Total Commander removes %L list files.
func startProcess(c Command) error {
	if handled, err := shellExecute(c); handled {
		if err == nil && len(c.Cleanup) > 0 {
			// ShellExecute gives no process to wait for, so the list files are removed after a generous delay.
			time.AfterFunc(10*time.Minute, func() { removeAll(c.Cleanup) })
		}
		return err
	}
	cmd := buildExec(nil, c)
	applyPlatform(cmd, c, true)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		_ = cmd.Wait()
		removeAll(c.Cleanup)
	}()
	return nil
}

// outputProcess is the default Output: run, wait, return stdout and stderr together.
func outputProcess(ctx context.Context, c Command) ([]byte, error) {
	cmd := buildExec(ctx, c)
	applyPlatform(cmd, c, false)
	out, err := cmd.CombinedOutput()
	removeAll(c.Cleanup)
	return out, err
}

func removeAll(paths []string) {
	for _, p := range paths {
		_ = os.Remove(p)
	}
}
