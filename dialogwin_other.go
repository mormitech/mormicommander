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

func attachOwned(win, parent *application.WebviewWindow) {}

// showOwned has nothing to do here (the owner was set before); false: the window still needs the focus.
func showOwned(win, parent *application.WebviewWindow) bool { return false }

func detachOwned(win, parent *application.WebviewWindow) {}
