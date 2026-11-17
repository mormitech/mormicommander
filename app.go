// The App object: what the page can ask about the app itself (version, platform, start folders, window state) and the
// few things it asks of the window (title, quit, minimise, About). Files and folders are in feature_fs.go, jobs in
// feature_jobs.go, settings in settings.go; each feature adds its own bound objects through features.go.
//
// Everything that touches Wails goes through this file, window.go, winstate.go, menu.go, shell.go and the Lister windows
// (feature_viewer_window.go), so the features call small helpers (a.emit, a.clipboardSetText, a.openFileDialog...) and
// never the Wails API itself.
package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"

	"mormicommander/internal/vfs"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// App is the window: what the page can ask about the app itself (version, platform, start folders, window state).
type App struct {
	ctx      context.Context // the application's context once Wails is set up; nil in tests (features check it)
	mu       sync.Mutex
	opts     startOpts
	mode     string    // "about" when the process shows only the About box (--about), else ""
	fs       vfs.FS    // the real file system, or the demo's fictional one
	mock     *vfs.Mock // the fictional file system in a demo run, else nil
	settings *Settings // see settings.go
	jobs     *Jobs     // see feature_jobs.go
	undo     *Undo     // the undo journal of file operations, see feature_undo.go
	ctl      *control  // control mode (see control.go); nil when off
	saved    *winState // where the window was left last time
	winW     int       // the size the window was asked for, before it is fitted to the screen
	winH     int
	menuBar  *application.Menu // the menu bar (see menu.go)
	quitting bool
	title    string // the window title the page set last (control mode's "info" tells it)

	wapp  *application.App           // the Wails application
	win   *application.WebviewWindow // the file manager's window ("main")
	about *application.WebviewWindow // the About window while it is open
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

// startup runs once the application is up and the main window exists: the window is fitted to the screen and put back
// where it was left, control mode starts, and every feature's start-up hook runs.
func (a *App) startup() {
	if w := a.mainWindow(); w != nil {
		fitWindow(w, a.winW, a.winH)
		if a.saved != nil {
			restoreWindow(a.wapp, w, *a.saved)
		}
	}
	a.reveal()
	if w := a.mainWindow(); w != nil && a.saved != nil {
		maximiseSaved(w, *a.saved)
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

// shutdown lets features clean up and stops control mode (the About window goes with the application).
func (a *App) shutdown() {
	featMu.Lock()
	hooks := append([]func(*App){}, shutdownHooks...)
	featMu.Unlock()
	for _, f := range hooks {
		f(a)
	}
	if a.ctl != nil {
		a.stopControl()
	}
}

// reveal shows the window and the app: macOS starts an app hidden when it is launched with a file, and Wails does not
// unhide it on its own.
func (a *App) reveal() {
	w := a.mainWindow()
	if w == nil {
		return
	}
	w.Show()
	application.InvokeSync(a.wapp.Show) // (AppKit wants the main thread, and Wails does not go there for this one)
}

// Activate brings the window the page lives in, and the app, to the front. The page calls it on a right-click while the
// window is not focused: macOS doesn't activate a window on a right-click, and an inactive web view gets no hover
// (mouse-move) events, so the context menu that opens would not react to the mouse until the window was clicked.
func (a *App) Activate(ctx context.Context) {
	w := windowOf(ctx)
	if w == nil {
		w = a.mainWindow()
	}
	if w == nil {
		return
	}
	application.InvokeSync(a.wapp.Show)
	w.Focus()
}

// Version is the app version from wails.json.
func (a *App) Version() string { return version() }

// Mode tells the page which window it is: "" for the file manager, "about" for the About window.
func (a *App) Mode(ctx context.Context) string {
	if w := windowOf(ctx); w != nil && w.Name() == aboutWindowName {
		return "about"
	}
	return ""
}

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
	if w := a.mainWindow(); w != nil {
		w.SetTitle(title)
	}
}

// Quit ends the app (cm_Exit). The window is saved first.
func (a *App) Quit() {
	if a.wapp == nil {
		return
	}
	a.SaveWindow()
	a.mu.Lock()
	a.quitting = true
	a.mu.Unlock()
	a.wapp.Quit()
}

// Minimize puts the window in the Dock / taskbar (cm_Minimize, Shift+Esc).
func (a *App) Minimize() {
	if a.toTray() { // (Options > Operation > Minimise to tray / menu bar)
		return
	}
	if w := a.mainWindow(); w != nil {
		w.Minimise()
	}
}

// ToggleMaximize maximises the window, or brings it back to its size (cm_Maximize / cm_Restore). Before maximising,
// the window's place and size are saved, so it can come back to them after a restart (F-018).
func (a *App) ToggleMaximize() {
	if w := a.mainWindow(); w != nil {
		if !w.IsMaximised() {
			a.SaveWindow()
		}
		w.ToggleMaximise()
	}
}

// OpenURL opens a web link in the system's browser.
func (a *App) OpenURL(url string) {
	if a.wapp != nil {
		a.wapp.Browser.OpenURL(url)
	}
}

// ShowAbout opens the About window.
func (a *App) ShowAbout() {
	if a.wapp != nil {
		a.openAbout()
	}
}

// CloseWindow closes the window the call came from (the About window's Esc).
func (a *App) CloseWindow(ctx context.Context) {
	if w := windowOf(ctx); w != nil && w.Name() != mainWindowName {
		w.Close()
	}
}
