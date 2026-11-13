// The windows of the app. The file manager is the window named "main", made once at start; the About box is a window of
// its own named "about", made when it is asked for and closed with Esc. Both load the same page (frontend/dist), which
// asks App.Mode which one it is in (app.go): Wails v3 hands every call the window it came from, so the page needs no
// URL parameter or second program for it.
//
// On macOS an app launched with a file starts hidden, and Wails does not unhide it on its own, so the main window and
// the app are shown again once the page is up (reveal). The background colour is the dark theme's --bg (style.css), so
// the window does not flash white while the page loads.
package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Window names (also what App.Mode tells the page).
const (
	mainWindowName  = "main"
	aboutWindowName = "about"
)

// windowBackground matches --bg of the dark theme in style.css.
var windowBackground = application.NewRGB(27, 38, 54)

// newMainWindow makes the file manager's window and wires its events: fitting it to the screen and putting it back
// where it was left once it exists, showing it when the page is up, files dropped from the system, and saving the
// window when it closes.
func (a *App) newMainWindow(width, height int) {
	o := a.opts
	w := a.wapp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             mainWindowName,
		Title:            "mormiCommander",
		Width:            width,
		Height:           height,
		MinWidth:         minWinW,
		MinHeight:        minWinH,
		URL:              "/",
		BackgroundColour: windowBackground,
		// Files dropped from Finder / Explorer reach the page with their paths (the "filedrop" event, used by
		// frontend/src/features/system-dnd.js and config/buttonbar.js); drags between the panels stay ordinary HTML
		// drags.
		EnableFileDrop: true,
		Mac:            application.MacWindow{Appearance: macAppearance(o.Theme)},
		Windows:        application.WindowsWindow{Theme: windowsTheme(o.Theme)},
		Linux:          application.LinuxWindow{Icon: appIcon},
	})
	a.mu.Lock()
	a.win = w
	a.mu.Unlock()

	a.wapp.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		a.startup()
	})
	w.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) { a.reveal() })
	// The page tracks the held modifier keys for the function key bar; a key let go while another window (a dialog
	// window, another app) had the keyboard never reaches it, so it is told when this window stops being the active one.
	for _, ev := range []events.WindowEventType{events.Common.WindowLostFocus, events.Common.WindowHide, events.Common.WindowMinimise} {
		w.OnWindowEvent(ev, func(*application.WindowEvent) { a.emit("window-inactive") })
	}
	w.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		x, y := 0, 0
		if d := e.Context().DropTargetDetails(); d != nil {
			x, y = d.X, d.Y
		}
		a.emit("filedrop", x, y, e.Context().DroppedFiles())
	})
	// The hook runs before the window goes: its place is saved while it can still be read, and the whole app ends with
	// it (an About window left open must not keep the process alive without its file manager).
	w.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) {
		a.mu.Lock()
		quitting := a.quitting
		a.quitting = true
		a.mu.Unlock()
		if !quitting { // (Quit saved the window already and is ending the app)
			a.SaveWindow()
			go a.wapp.Quit()
		}
	})
}

// mainWindow is the file manager's window, nil before it exists (and in tests).
func (a *App) mainWindow() *application.WebviewWindow {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.win
}

// openAbout shows the About window, or brings it to the front when it is already open.
func (a *App) openAbout() {
	a.mu.Lock()
	if w := a.about; w != nil {
		a.mu.Unlock()
		w.Show()
		w.Focus()
		return
	}
	a.mu.Unlock()
	w := a.wapp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             aboutWindowName,
		Title:            "About mormiCommander",
		Width:            440,
		Height:           560,
		DisableResize:    true,
		URL:              "/",
		BackgroundColour: windowBackground,
		InitialPosition:  application.WindowCentered,
		// The About box has no menu of its own on Windows and Linux (macOS keeps the app's menu bar).
		Windows:             application.WindowsWindow{Theme: windowsTheme(a.opts.Theme), DisableMenu: true},
		Mac:                 application.MacWindow{Appearance: macAppearance(a.opts.Theme)},
		Linux:               application.LinuxWindow{Icon: appIcon},
		MinimiseButtonState: application.ButtonHidden,
		MaximiseButtonState: application.ButtonHidden,
	})
	w.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		a.mu.Lock()
		if a.about == w {
			a.about = nil
		}
		a.mu.Unlock()
	})
	a.mu.Lock()
	a.about = w
	a.mu.Unlock()
}

// windowOf is the window a call from the page came from (Wails v3 puts it in the call's context), or nil.
func windowOf(ctx interface{ Value(any) any }) application.Window {
	if ctx == nil {
		return nil
	}
	w, _ := ctx.Value(application.WindowKey).(application.Window)
	return w
}

// fitSize limits a window of w x h to 90% of the screen, so the title bar stays reachable when the system's scaling is
// high. A screen of unknown size leaves the window alone.
func fitSize(w, h, screenW, screenH int) (int, int) {
	if screenW <= 0 || screenH <= 0 {
		return w, h
	}
	return min(w, screenW*9/10), min(h, screenH*9/10)
}

// fitWindow shrinks the window to the screen it is on, and centres it if it had to.
func fitWindow(win *application.WebviewWindow, w, h int) {
	s, err := win.GetScreen()
	if err != nil || s == nil {
		return
	}
	fw, fh := fitSize(w, h, s.Size.Width, s.Size.Height)
	if fw != w || fh != h {
		win.SetSize(fw, fh)
		win.Center()
	}
}
