package main

// Options > Operation > Minimise to tray / menu bar (board issue #94): with it on, minimising the main window (Shift+Esc,
// the window's own button) hides it instead and puts mormiCommander's icon in the menu bar (macOS) or the
// notification area (Windows, Linux); a click on the icon, or its "Show mormiCommander", brings the window back.

import (
	"bytes"
	"image"
	"image/png"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	xdraw "golang.org/x/image/draw"
)

var tray struct {
	mu     sync.Mutex
	t      *application.SystemTray
	hiding bool
}

func init() {
	onStartup(func(a *App) {
		w := a.mainWindow()
		if w == nil {
			return
		}
		// the window's own minimise button: back from the Dock / taskbar, then hidden with the icon shown
		w.OnWindowEvent(events.Common.WindowMinimise, func(*application.WindowEvent) {
			if !trayWanted() {
				return
			}
			application.InvokeAsync(func() {
				w.UnMinimise()
				a.toTray()
			})
		})
	})
}

func trayWanted() bool {
	s, ok := currentSettings()
	return ok && s.Operation.MinimizeToTray
}

// toTray hides the main window and shows the tray icon; false when the option is off (the caller minimises as usual).
func (a *App) toTray() bool {
	if !trayWanted() || a.wapp == nil {
		return false
	}
	w := a.mainWindow()
	if w == nil {
		return false
	}
	tray.mu.Lock()
	if tray.t == nil {
		t := a.wapp.SystemTray.New()
		if icon := trayIcon(); icon != nil {
			t.SetIcon(icon)
		}
		t.SetTooltip("mormiCommander")
		m := application.NewMenu()
		m.Add("Show mormiCommander").OnClick(func(*application.Context) { a.fromTray() })
		m.AddSeparator()
		m.Add("Quit").OnClick(func(*application.Context) { a.Quit() })
		t.SetMenu(m)
		t.OnClick(func() { a.fromTray() })
		tray.t = t
	}
	t := tray.t
	tray.mu.Unlock()
	t.Show()
	w.Hide()
	return true
}

// fromTray shows the main window again and takes the icon away.
func (a *App) fromTray() {
	if w := a.mainWindow(); w != nil {
		w.Show().Focus()
	}
	tray.mu.Lock()
	t := tray.t
	tray.mu.Unlock()
	if t != nil {
		t.Hide()
	}
}

// trayIcon is the app icon at the size of a menu bar icon (36 pixels, sharp at twice the 18-point size).
func trayIcon() []byte {
	src, err := png.Decode(bytes.NewReader(appIcon))
	if err != nil {
		return appIcon
	}
	const px = 36
	dst := image.NewNRGBA(image.Rect(0, 0, px, px))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
	var buf bytes.Buffer
	if png.Encode(&buf, dst) != nil {
		return appIcon
	}
	return buf.Bytes()
}
