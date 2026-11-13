package main

// Desktop integration, the starting-programs half: Enter on a file opens it with its app or runs it (F-051), smart
// Enter on shortcut files (F-738), Shift+Enter in a terminal (F-199, F-265), "Open with" (F-200), a terminal in the
// current folder (F-196, F-363, F-371), "Show in Finder / Explorer" (F-366), running the command line (F-293, F-294,
// F-302) and running a program with dropped files (section 36). The work is done by internal/launch and internal/links;
// this file only turns their answers into plain sentences for the page (frontend/src/features/system.js).

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"mormicommander/internal/launch"
	"mormicommander/internal/links"
)

// SystemLaunch is what the page calls to open files and start programs (window.go.main.SystemLaunch).
type SystemLaunch struct {
	app *App
	// newLauncher makes the launcher for one action; tests replace it with one that records commands.
	newLauncher func() *launch.Launcher
}

func init() {
	registerFeature("system", func(a *App) []any {
		return []any{newSystemLaunch(a), &SystemClip{app: a}, &SystemFiles{app: a}}
	})
}

func newSystemLaunch(a *App) *SystemLaunch {
	return &SystemLaunch{app: a, newLauncher: launch.New}
}

// systemDemoErr is the answer of every desktop action in the demo, whose files are not real.
var systemDemoErr = errors.New("This is the demo: its files are not real, so they can't be opened or run.")

// launcher is a launcher with the user's terminal choice (settings "terminal": a program name or a command line
// with %D for the folder).
func (s *SystemLaunch) launcher() *launch.Launcher {
	l := s.newLauncher()
	if s.app.settings != nil {
		if t, ok := s.app.settings.Get("terminal").(string); ok {
			l.Terminal = strings.TrimSpace(t)
		}
	}
	return l
}

// systemLocalDir checks that dir is a folder on this computer (not inside an archive or on an FTP server), so a
// program can be started there.
func systemLocalDir(dir string) error {
	if strings.Contains(dir, "://") {
		return errors.New("Programs can only be started in folders on this computer, not in archives or on servers.")
	}
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return fmt.Errorf("The folder %s is not there.", dir)
	}
	return nil
}

// systemStartDir is the folder a program starts in: dir when it is a local folder, else the home folder.
func systemStartDir(dir string) string {
	if systemLocalDir(dir) == nil {
		return dir
	}
	h, _ := os.UserHomeDir()
	return h
}

// systemIsProgram says whether a file is a program to run rather than a document to open: on Windows by its
// extension (PATHEXT), elsewhere by the execute bit together with a program or script header, so that the files on a
// FAT stick (where every file has the execute bit) still open in their apps.
func systemIsProgram(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	if fi.IsDir() {
		return runtime.GOOS == "darwin" && strings.HasSuffix(strings.TrimRight(path, "/"), ".app")
	}
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(path))
		pathext := os.Getenv("PATHEXT")
		if pathext == "" {
			pathext = ".COM;.EXE;.BAT;.CMD"
		}
		for _, e := range strings.Split(strings.ToLower(pathext), ";") {
			if e != "" && ext == strings.TrimSpace(e) {
				return true
			}
		}
		return false
	}
	if fi.Mode()&0o111 == 0 {
		return false
	}
	return launch.ExecKindOf(path) != launch.KindUnknown || systemHasShebang(path)
}

// systemHasShebang says whether a file starts with "#!", the mark of a script.
func systemHasShebang(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	b := make([]byte, 2)
	n, _ := f.Read(b)
	return n == 2 && b[0] == '#' && b[1] == '!'
}

// systemQuote quotes one file name for a command line of this system.
func systemQuote(s string) string {
	if runtime.GOOS == "windows" {
		return launch.QuoteWindowsArg(s)
	}
	return launch.ShellQuote(s)
}

// systemErr turns a launch error into a sentence.
func systemErr(what, path string, err error) error {
	name := filepath.Base(path)
	switch {
	case errors.Is(err, launch.ErrNoTerminal):
		return errors.New("No terminal program was found. Choose one in the settings (\"terminal\").")
	case errors.Is(err, launch.ErrUnsupported):
		return fmt.Errorf("This system can't %s.", what)
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("%s is not there any more.", name)
	case errors.Is(err, fs.ErrPermission):
		return fmt.Errorf("You don't have permission to %s %s.", what, name)
	}
	return fmt.Errorf("Can't %s %s: %v", what, name, err)
}

// SystemOpenResult tells the page what Enter on a file did: "opened" (with its app), "started" (a program), "dir"
// (show Dir with the cursor on Focus: a shortcut to a folder or file).
type SystemOpenResult struct {
	Kind  string `json:"kind"`
	Dir   string `json:"dir"`
	Focus string `json:"focus"`
}

// Open is Enter on a file: a shortcut file (.lnk, .url, .webloc, .desktop) is followed, a program is started (a
// console program or script in a terminal), and any other file opens with the app the system uses for it.
func (s *SystemLaunch) Open(path, dir string) (SystemOpenResult, error) {
	if s.app.mock != nil {
		return SystemOpenResult{}, systemDemoErr
	}
	if launch.IsShortcutName(filepath.Base(path)) {
		if r, err, ok := s.follow(path); ok {
			return r, err
		}
	}
	l := s.launcher()
	dir = systemStartDir(dir)
	if systemIsProgram(path) {
		if runtime.GOOS != "windows" && launch.ExecKindOf(path) == launch.KindConsole && !strings.HasSuffix(path, ".app") {
			if err := l.RunInTerminal(dir, systemQuote(path), false); err != nil {
				return SystemOpenResult{}, systemErr("run", path, err)
			}
			return SystemOpenResult{Kind: "started"}, nil
		}
		if err := l.Run(launch.RunSpec{Program: path, Dir: dir}); err != nil {
			return SystemOpenResult{}, systemErr("run", path, err)
		}
		return SystemOpenResult{Kind: "started"}, nil
	}
	if err := l.OpenDefault(path); err != nil {
		return SystemOpenResult{}, systemErr("open", path, err)
	}
	return SystemOpenResult{Kind: "opened"}, nil
}

// follow does what a shortcut file points to. ok is false when the file is no shortcut after all (then it is opened
// like any other file).
func (s *SystemLaunch) follow(path string) (SystemOpenResult, error, bool) {
	a, err := links.FollowLink(path)
	if err != nil {
		if errors.Is(err, links.ErrNotLink) || errors.Is(err, links.ErrBadShortcut) {
			return SystemOpenResult{}, nil, false
		}
		if errors.Is(err, links.ErrBroken) {
			return SystemOpenResult{}, fmt.Errorf("%s points to something that is not there.", filepath.Base(path)), true
		}
		return SystemOpenResult{}, fmt.Errorf("Can't follow %s: %v", filepath.Base(path), err), true
	}
	l := s.launcher()
	switch a.Kind {
	case links.ActionOpenDir:
		return SystemOpenResult{Kind: "dir", Dir: a.Dir}, nil, true
	case links.ActionFocusFile:
		return SystemOpenResult{Kind: "dir", Dir: a.Dir, Focus: a.Focus}, nil, true
	case links.ActionOpenURL:
		if err := l.OpenDefault(a.URL); err != nil {
			return SystemOpenResult{}, systemErr("open", path, err), true
		}
		return SystemOpenResult{Kind: "opened"}, nil, true
	case links.ActionRun:
		params := make([]string, 0, len(a.Args)-1)
		for _, x := range a.Args[1:] {
			params = append(params, systemQuote(x))
		}
		if err := l.Run(launch.RunSpec{Program: a.Args[0], Params: strings.Join(params, " "), Dir: systemStartDir(a.Dir)}); err != nil {
			return SystemOpenResult{}, systemErr("run", path, err), true
		}
		return SystemOpenResult{Kind: "started"}, nil, true
	}
	return SystemOpenResult{}, nil, false
}

// RunKeepOpen is Shift+Enter on a file: a program or script runs in a terminal window that stays open afterwards; an
// application bundle starts; a folder opens in Finder / Explorer; a document opens with its app.
func (s *SystemLaunch) RunKeepOpen(path, dir string) (SystemOpenResult, error) {
	if s.app.mock != nil {
		return SystemOpenResult{}, systemDemoErr
	}
	l := s.launcher()
	dir = systemStartDir(dir)
	fi, err := os.Stat(path)
	if err != nil {
		return SystemOpenResult{}, systemErr("run", path, err)
	}
	switch {
	case fi.IsDir() && systemIsProgram(path):
		err = l.Run(launch.RunSpec{Program: path, Dir: dir})
	case fi.IsDir():
		err = l.OpenFolder(path)
		if err == nil {
			return SystemOpenResult{Kind: "opened"}, nil
		}
	case systemIsProgram(path):
		err = l.Run(launch.RunSpec{Program: path, Dir: dir, Terminal: true, KeepOpen: true})
	default:
		if err = l.OpenDefault(path); err == nil {
			return SystemOpenResult{Kind: "opened"}, nil
		}
	}
	if err != nil {
		return SystemOpenResult{}, systemErr("run", path, err)
	}
	return SystemOpenResult{Kind: "started"}, nil
}

// SystemLineResult tells the page what a command line did: "run" (a program or shell command was started), "cd"
// (show the folder Target), "internal" (run the cm_* command Command), "back" (cd -) or "empty".
type SystemLineResult struct {
	Kind    string `json:"kind"`
	Target  string `json:"target"`
	Command string `json:"command"`
}

// RunLine runs a command line in dir (Enter in the command line): a typed folder or drive is shown, a program or shell
// command starts (console programs in a terminal). keepOpen (Shift+Enter) always uses a terminal that stays open.
func (s *SystemLaunch) RunLine(line, dir string, keepOpen bool) (SystemLineResult, error) {
	l := s.launcher()
	if s.app.mock != nil {
		return SystemLineResult{}, errors.New("This is the demo: it can't run programs.")
	}
	start := systemStartDir(dir)
	p := l.ParseLine(line, start)
	switch p.Kind {
	case launch.LineEmpty:
		return SystemLineResult{Kind: "empty"}, nil
	case launch.LineCd:
		if p.Back {
			return SystemLineResult{Kind: "back"}, nil
		}
		return SystemLineResult{Kind: "cd", Target: p.Target}, nil
	case launch.LineInternal:
		return SystemLineResult{Kind: "internal", Command: p.Command}, nil
	}
	mode := launch.LineAuto
	if keepOpen {
		mode = launch.LineTerminal
	}
	if err := l.RunLine(line, start, keepOpen, mode); err != nil {
		if errors.Is(err, launch.ErrNoTerminal) {
			return SystemLineResult{}, systemErr("run", line, err)
		}
		return SystemLineResult{}, fmt.Errorf("Can't run %q: %v", line, err)
	}
	return SystemLineResult{Kind: "run"}, nil
}

// RunWith starts a program with files as its parameters (files dropped on a program in a panel).
func (s *SystemLaunch) RunWith(program string, files []string, dir string) error {
	if s.app.mock != nil {
		return systemDemoErr
	}
	q := make([]string, len(files))
	for i, f := range files {
		q[i] = systemQuote(f)
	}
	if err := s.launcher().Run(launch.RunSpec{Program: program, Params: strings.Join(q, " "), Dir: systemStartDir(dir)}); err != nil {
		return systemErr("run", program, err)
	}
	return nil
}

// AppsFor lists the applications that can open a file, the system's default first (for "Open with").
func (s *SystemLaunch) AppsFor(path string) ([]launch.App, error) {
	if s.app.mock != nil {
		return nil, systemDemoErr
	}
	apps, err := s.launcher().AppsFor(path)
	if err != nil && len(apps) == 0 {
		return nil, systemErr("list the apps for", path, err)
	}
	return apps, nil
}

// OpenWith opens files with an application from the AppsFor list.
func (s *SystemLaunch) OpenWith(paths []string, app launch.App) error {
	if s.app.mock != nil {
		return systemDemoErr
	}
	l := s.launcher()
	for _, p := range paths {
		if err := l.OpenWithApp(p, app); err != nil {
			return systemErr("open", p, err)
		}
	}
	return nil
}

// OpenWithProgram opens files with a program the user typed: an app name, a path, or (Linux) a .desktop id.
func (s *SystemLaunch) OpenWithProgram(paths []string, program string) error {
	if s.app.mock != nil {
		return systemDemoErr
	}
	program = strings.TrimSpace(program)
	if program == "" {
		return errors.New("Type the name or the path of a program.")
	}
	l := s.launcher()
	for _, p := range paths {
		if err := l.OpenWith(p, program); err != nil {
			return systemErr("open", p, err)
		}
	}
	return nil
}

// HasChooser says whether the system has an "Open with" dialog of its own (Windows only).
func (s *SystemLaunch) HasChooser() bool { return runtime.GOOS == "windows" }

// OpenWithChooser shows the system's own "Open with" dialog for a file (Windows).
func (s *SystemLaunch) OpenWithChooser(path string) error {
	if s.app.mock != nil {
		return systemDemoErr
	}
	if err := s.launcher().OpenWithChooser(path); err != nil {
		return systemErr("show the \"Open with\" dialog for", path, err)
	}
	return nil
}

// Terminal opens the system's terminal in a folder (cm_ExecuteDOS).
func (s *SystemLaunch) Terminal(dir string) error {
	if s.app.mock != nil {
		return systemDemoErr
	}
	if err := systemLocalDir(dir); err != nil {
		return err
	}
	if err := s.launcher().OpenTerminal(dir); err != nil {
		return systemErr("open a terminal in", dir, err)
	}
	return nil
}

// Reveal shows a file or folder selected in Finder / Explorer / the file manager (cm_Explorer).
func (s *SystemLaunch) Reveal(path string) error {
	if s.app.mock != nil {
		return systemDemoErr
	}
	if strings.Contains(path, "://") {
		return errors.New("Only files on this computer can be shown in the file manager.")
	}
	if _, err := os.Lstat(path); err != nil {
		return systemErr("show", path, err)
	}
	if err := s.launcher().Reveal(path); err != nil {
		return systemErr("show", path, err)
	}
	return nil
}

// OpenFolder opens a folder in Finder / Explorer / the file manager.
func (s *SystemLaunch) OpenFolder(dir string) error {
	if s.app.mock != nil {
		return systemDemoErr
	}
	if err := systemLocalDir(dir); err != nil {
		return err
	}
	if err := s.launcher().OpenFolder(dir); err != nil {
		return systemErr("open", dir, err)
	}
	return nil
}

// OpenURL opens a web address or a file URL with the system's handler.
func (s *SystemLaunch) OpenURL(url string) error {
	if s.app.mock != nil {
		return systemDemoErr
	}
	return s.launcher().OpenDefault(url)
}

// Desktop is the user's desktop folder (cm_OpenDesktop); the home folder when there is none.
func (s *SystemLaunch) Desktop() string {
	if s.app.mock != nil {
		return s.app.mock.Clean("~/Desktop")
	}
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "linux" {
		if d := systemXDGDesktop(home); d != "" {
			return d
		}
	}
	d := filepath.Join(home, "Desktop")
	if fi, err := os.Stat(d); err == nil && fi.IsDir() {
		return d
	}
	return home
}

// systemXDGDesktop reads XDG_DESKTOP_DIR from ~/.config/user-dirs.dirs ("" when it is not set).
func systemXDGDesktop(home string) string {
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		cfg = filepath.Join(home, ".config")
	}
	b, err := os.ReadFile(filepath.Join(cfg, "user-dirs.dirs"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "XDG_DESKTOP_DIR="); ok {
			v = strings.Trim(v, `"`)
			v = strings.Replace(v, "$HOME", home, 1)
			if fi, err := os.Stat(v); err == nil && fi.IsDir() {
				return v
			}
		}
	}
	return ""
}
