//go:build windows

// Dialog windows on Windows (feature_dialog_window.go): a dialog window is owned by the window it belongs to, so it
// stays above it, is minimised with it and has no taskbar button of its own.
package main

import (
	"syscall"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var procSetWindowLongPtr = syscall.NewLazyDLL("user32.dll").NewProc("SetWindowLongPtrW")

const gwlpHwndParent = ^uintptr(7) // GWLP_HWNDPARENT (-8)

// placeWindow sizes a dialog window so its inside is w x h and puts its top left corner at x, y.
func placeWindow(win *application.WebviewWindow, x, y, w, h, frameW, frameH int) {
	win.SetSize(w+frameW, h+frameH)
	win.SetPosition(x, y)
}

// attachOwned makes the window below the dialog window its owner.
// frameExtra is not asked of the window here: the frame size learnt from the first page is used.
func frameExtra(win *application.WebviewWindow) (int, int, bool) { return 0, 0, false }

func attachOwned(win, parent *application.WebviewWindow) {
	if parent == nil {
		return
	}
	child, owner := uintptr(win.NativeWindow()), uintptr(parent.NativeWindow())
	if child == 0 || owner == 0 {
		return
	}
	application.InvokeSync(func() { procSetWindowLongPtr.Call(child, gwlpHwndParent, owner) })
}

// showOwned has nothing to do here (the owner was set before); false: the window still needs the focus.
func showOwned(win, parent *application.WebviewWindow) bool { return false }

func detachOwned(win, parent *application.WebviewWindow) {}

// prepareShow has nothing to do here.
func prepareShow(win *application.WebviewWindow) {}

// wakeWindow has nothing to do here (the watchdog only tells the window's page to paint again; showing the window
// again could bring back one that has just closed).
func wakeWindow(win *application.WebviewWindow) {}
