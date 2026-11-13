package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"

	"mormicommander/internal/vfs"

	"github.com/wailsapp/wails/v2/pkg/menu"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the window: what the page can ask about the app itself (version, platform, start folders, window state).
// Files and folders are in feature_fs.go, jobs in feature_jobs.go, settings in settings.go; each feature adds its own
// bound objects through features.go.
type App struct {
	ctx      context.Context
	mu       sync.Mutex
	opts     startOpts
	mode     string    // "" for the file manager, "about" for the About window
	fs       vfs.FS    // the real file system, or the demo's fictional one
	mock     *vfs.Mock // the fictional file system in a demo run, else nil
	settings *Settings // see settings.go
	jobs     *Jobs     // see feature_jobs.go
	ctl      *control  // control mode (see control.go); nil when off
	saved    *winState // where the window was left last time
	winW     int       // the size the window was asked for, before it is fitted to the screen
	winH     int
	menuBar  *menu.Menu           // the menu bar (see menu.go)
	wins     map[string]*exec.Cmd // windows of their own (About), by key
	quitting bool
	title    string // the window title the page set last (control mode's "info" tells it)
}

// NewApp makes the app for the given command line.
func NewApp(o startOpts) *App {
	a := &App{opts: o, fs: vfs.Local{}}
	if o.About {
		a.mode = "about"
	}
	if o.Demo {
		a.mock = vfs.NewMock()
		a.fs = a.mock
	}
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	fitWindow(ctx, a.winW, a.winH)
	if a.saved != nil && a.mode == "" {
		restoreWindow(ctx, *a.saved)
	}
	a.reveal()
	if a.mode != "" {
		return
	}
	if a.opts.Control != "" {
		if addr, err := a.startControl(a.opts.Control); err != nil {
			fmt.Fprintln(os.Stderr, "mormicommander: control mode:", err)
		} else {
			fmt.Fprintln(os.Stderr, "mormicommander: control mode on", addr)
		}
	}
	featMu.Lock()
	hooks := append([]func(*App){}, startupHooks...)
	featMu.Unlock()
	for _, f := range hooks {
		f(a)
	}
}

// shutdown closes the windows this one opened (About) and lets features clean up.
func (a *App) shutdown(ctx context.Context) {
	featMu.Lock()
	hooks := append([]func(*App){}, shutdownHooks...)
	featMu.Unlock()
	for _, f := range hooks {
		f(a)
	}
	if a.ctl != nil {
		a.stopControl()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, c := range a.wins {
		if c.Process != nil {
			c.Process.Kill()
		}
	}
}

// reveal shows the window and the app: macOS starts an app hidden when it is launched with a file, and Wails does not
// unhide it on its own.
func (a *App) reveal() {
	if a.ctx == nil {
		return
	}
	wruntime.WindowShow(a.ctx)
	wruntime.Show(a.ctx)
}

// emit sends an event to the page.
func (a *App) emit(event string, data ...any) {
	if a.ctx != nil {
		wruntime.EventsEmit(a.ctx, event, data...)
	}
}

// Version is the app version from wails.json.
func (a *App) Version() string { return version() }

// Mode tells the page which window it is: "" for the file manager, "about" for the About window.
func (a *App) Mode() string { return a.mode }

// Platform is the operating system ("darwin", "windows" or "linux"), so the page can use its words and keys.
func (a *App) Platform() string { return runtime.GOOS }

// Demo says whether the app shows the fictional files of --demo.
func (a *App) Demo() bool { return a.mock != nil }

// StartInfo is what the command line asked for: the panel folders and the active panel ("" = what the last session had).
func (a *App) StartInfo() map[string]any {
	return map[string]any{"left": a.opts.Left, "right": a.opts.Right, "active": a.opts.Active, "demo": a.mock != nil,
		"control": a.opts.Control != "", "theme": a.opts.Theme}
}

// SetTitle sets the window title ("mormiCommander – /path/of/the/active/panel").
func (a *App) SetTitle(title string) {
	a.mu.Lock()
	a.title = title
	a.mu.Unlock()
	if a.ctx != nil {
		wruntime.WindowSetTitle(a.ctx, title)
	}
}

// Quit ends the app (cm_Exit). The window is saved first.
func (a *App) Quit() {
	if a.ctx == nil {
		return
	}
	a.mu.Lock()
	a.quitting = true
	a.mu.Unlock()
	a.SaveWindow()
	wruntime.Quit(a.ctx)
}

// Minimize puts the window in the Dock / taskbar (cm_Minimize, Shift+Esc).
func (a *App) Minimize() {
	if a.ctx != nil {
		wruntime.WindowMinimise(a.ctx)
	}
}

// ToggleMaximize maximises the window, or brings it back to its size (cm_Maximize / cm_Restore).
func (a *App) ToggleMaximize() {
	if a.ctx != nil {
		wruntime.WindowToggleMaximise(a.ctx)
	}
}

// OpenURL opens a web link in the system's browser.
func (a *App) OpenURL(url string) {
	if a.ctx != nil {
		wruntime.BrowserOpenURL(a.ctx, url)
	}
}

// ShowAbout opens the About window.
func (a *App) ShowAbout() { a.openWindow("about", "--about") }

// CloseWindow closes this window (the About window's Esc).
func (a *App) CloseWindow() {
	if a.ctx != nil {
		wruntime.Quit(a.ctx)
	}
}

// openWindow opens a window of its own: a second copy of this program, started with the given arguments, because
// Wails v2 has one window per process. A window that is already open (same key) is not opened twice.
func (a *App) openWindow(key string, args ...string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if c := a.wins[key]; c != nil && c.ProcessState == nil {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if a.opts.Theme != "" {
		args = append([]string{"--theme", a.opts.Theme}, args...)
	}
	cmd := exec.Command(exe, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	if a.wins == nil {
		a.wins = map[string]*exec.Cmd{}
	}
	a.wins[key] = cmd
	go func() {
		cmd.Wait()
		a.mu.Lock()
		delete(a.wins, key)
		a.mu.Unlock()
	}()
	return nil
}
