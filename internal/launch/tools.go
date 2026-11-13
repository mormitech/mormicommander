// The external viewer and editor (F3 with an external viewer, F-590; F4, F-591; Shift+F4, F-595/F-153; the
// Viewer/Editor configuration page, F-420). Total Commander stores each as one line: a program, optionally followed
// by parameters in which %1 (or any other placeholder) stands for the file; without a placeholder the quoted file
// name is appended. An empty editor means the system's text editor: TextEdit in plain text mode through `open -t`
// on macOS, Notepad on Windows, and on Linux the desktop's default application for text/plain (falling back to
// xdg-open). Editing a file that lives inside an archive or on an FTP server needs to know when the user saved it, so
// WatchFile reports changes to a file until the caller stops watching.
package launch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Tool is a configured external viewer or editor.
type Tool struct {
	Program string // program path or name; empty means the system default (editor) or none (viewer)
	Params  string // parameter text with placeholders; empty means the file name is appended
}

// ParseTool splits a configuration line such as `"C:\Program Files\Notepad++\notepad++.exe" -n1 "%1"` or
// `code --wait %1` into program and parameters. A path with spaces must be quoted, as in Total Commander.
func ParseTool(line string) Tool {
	line = strings.TrimSpace(line)
	if line == "" {
		return Tool{}
	}
	prog, params := splitWindowsProgram(line)
	return Tool{Program: prog, Params: params}
}

// String gives the configuration line for a tool, the inverse of ParseTool.
func (t Tool) String() string {
	if t.Program == "" {
		return ""
	}
	p := t.Program
	if strings.ContainsAny(p, " \t") {
		p = `"` + p + `"`
	}
	if t.Params != "" {
		p += " " + t.Params
	}
	return p
}

// ToolCommand returns the command that opens file with tool. ctx gives the panels for placeholders other than %1;
// its File is set to file. An empty tool program means the system's default editor.
func (l *Launcher) ToolCommand(t Tool, file string, ctx ParamContext) (Command, error) {
	if t.Program == "" {
		return l.DefaultEditorCommand(file)
	}
	ctx.File = file
	exp, err := l.ExpandParams(t.Params, ctx)
	if err != nil {
		return Command{}, err
	}
	params := exp.Text
	if !exp.HasFile {
		var q string
		if l.windows() {
			q = QuoteWindowsArg(file)
		} else {
			q = EscapeArg(file)
		}
		if params != "" {
			params += " "
		}
		params += q
	}
	dir := ctx.Source.Dir
	if dir == "" {
		dir = filepath.Dir(file)
	}
	c, err := l.RunCommand(RunSpec{Program: t.Program, Params: params, Dir: dir, Cleanup: exp.ListFiles})
	if err != nil {
		removeAll(exp.ListFiles)
	}
	return c, err
}

// OpenInTool opens file with an external viewer or editor (F3/F4 when configured as external).
func (l *Launcher) OpenInTool(t Tool, file string, ctx ParamContext) error {
	c, err := l.ToolCommand(t, file, ctx)
	if err != nil {
		return err
	}
	return l.start(c)
}

// DefaultEditorCommand returns the command that opens file in the system's text editor.
func (l *Launcher) DefaultEditorCommand(file string) (Command, error) {
	switch {
	case l.darwin():
		return Command{Path: "open", Args: []string{"-t", safeArg(file)}}, nil
	case l.windows():
		return Command{Path: "notepad.exe", Params: QuoteWindowsArg(file)}, nil
	default:
		if app, ok := l.defaultTextApp(); ok {
			return l.OpenWithAppCommand(file, app)
		}
		return l.OpenDefaultCommand(file), nil
	}
}

// defaultTextApp finds the desktop's default application for text/plain on Linux.
func (l *Launcher) defaultTextApp() (App, bool) {
	if l.OS != runtime.GOOS {
		return App{}, false
	}
	apps, err := l.AppsForMIME("text/plain")
	if err != nil {
		return App{}, false
	}
	for _, a := range apps {
		if a.Default {
			return a, true
		}
	}
	return App{}, false
}

// CreateForEdit makes sure dir/name exists for Shift+F4 ("edit new file"): an existing file is left untouched,
// a missing one is created empty. It returns the full path and whether the file was created.
func CreateForEdit(dir, name string) (string, bool, error) {
	if name == "" || strings.ContainsAny(name, "\x00") {
		return "", false, errors.New("launch: no file name given")
	}
	p := name
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, name)
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err == nil {
		return p, true, f.Close()
	}
	if errors.Is(err, os.ErrExist) {
		fi, serr := os.Stat(p)
		if serr != nil {
			return "", false, serr
		}
		if fi.IsDir() {
			return "", false, errors.New("launch: " + name + " is a folder")
		}
		return p, false, nil
	}
	return "", false, err
}

// FileChange is one change WatchFile noticed.
type FileChange struct {
	Path    string
	Size    int64
	ModTime time.Time
	Removed bool // the file disappeared (some editors save by deleting and re-creating; it may come back)
}

// WatchFile checks path every interval and sends a FileChange whenever its size or modification time changes,
// until ctx is cancelled; then the channel is closed. It is how an edited archive member or FTP file is noticed as
// saved (F-592/F-593) without depending on the editor process, which on macOS is usually shared by many files.
func WatchFile(ctx context.Context, path string, interval time.Duration) <-chan FileChange {
	if interval <= 0 {
		interval = time.Second
	}
	ch := make(chan FileChange, 1)
	go func() {
		defer close(ch)
		var lastSize int64 = -1
		var lastMod time.Time
		missing := false
		if fi, err := os.Stat(path); err == nil {
			lastSize, lastMod = fi.Size(), fi.ModTime()
		} else {
			missing = true
		}
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			fi, err := os.Stat(path)
			var ev *FileChange
			switch {
			case err != nil && !missing:
				missing = true
				ev = &FileChange{Path: path, Removed: true}
			case err == nil && (missing || fi.Size() != lastSize || !fi.ModTime().Equal(lastMod)):
				missing = false
				lastSize, lastMod = fi.Size(), fi.ModTime()
				ev = &FileChange{Path: path, Size: lastSize, ModTime: lastMod}
			}
			if ev != nil {
				select {
				case ch <- *ev:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return ch
}
