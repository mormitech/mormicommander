//go:build !darwin && !windows && !(linux && cgo && !android && !server && !purego)

// Dialog windows where the window system gives no way to keep one above another (feature_dialog_window.go): they are
// only sized and placed.
package main

import "github.com/wailsapp/wails/v3/pkg/application"

// placeWindow sizes a dialog window so its inside is w x h and puts its top left corner at x, y.
func placeWindow(win *application.WebviewWindow, x, y, w, h, frameW, frameH int) {
	win.SetSize(w+frameW, h+frameH)
	win.SetPosition(x, y)
}

// frameExtra is not asked of the window here: the frame size learnt from the first page is used.
func frameExtra(win *application.WebviewWindow) (int, int, bool) { return 0, 0, false }

func attachOwned(win, parent *application.WebviewWindow) {}

// showOwned has nothing to do here (the owner was set before); false: the window still needs the focus.
func showOwned(win, parent *application.WebviewWindow) bool { return false }

func detachOwned(win, parent *application.WebviewWindow) {}

// prepareShow has nothing to do here.
func prepareShow(win *application.WebviewWindow) {}

// wakeWindow has nothing to do here (the watchdog only tells the window's page to paint again; showing the window
// again could bring back one that has just closed).
func wakeWindow(win *application.WebviewWindow) {}
