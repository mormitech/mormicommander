//go:build windows

// Dialog windows on Windows (feature_dialog_window.go): a dialog window is owned by the window it belongs to, so it
// stays above it, is minimised with it and has no taskbar button of its own (Wails' WS_EX_APPWINDOW is taken off, see
// dialogExStyle).
package main

import (
	"syscall"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procSetWindowLongPtr = user32.NewProc("SetWindowLongPtrW")
	procGetWindowLongPtr = user32.NewProc("GetWindowLongPtrW")
	procSetWindowPos     = user32.NewProc("SetWindowPos")
)

const (
	gwlpHwndParent = ^uintptr(7)  // GWLP_HWNDPARENT (-8)
	gwlExStyle     = ^uintptr(19) // GWL_EXSTYLE (-20)
	// SetWindowPos flags: change nothing but let the new style take effect (SWP_NOSIZE | SWP_NOMOVE | SWP_NOZORDER |
	// SWP_NOACTIVATE | SWP_FRAMECHANGED)
	swpStyleOnly = 0x0001 | 0x0002 | 0x0004 | 0x0010 | 0x0020
)

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
	application.InvokeSync(func() {
		procSetWindowLongPtr.Call(child, gwlpHwndParent, owner)
		// no taskbar button of its own (dialogExStyle); the window is still hidden here, so the taskbar never shows one
		ex, _, _ := procGetWindowLongPtr.Call(child, gwlExStyle)
		if nex := dialogExStyle(ex); nex != ex {
			procSetWindowLongPtr.Call(child, gwlExStyle, nex)
			procSetWindowPos.Call(child, 0, 0, 0, 0, 0, swpStyleOnly)
		}
	})
}

// showOwned has nothing to do here (the owner was set before); false: the window still needs the focus.
func showOwned(win, parent *application.WebviewWindow) bool { return false }

func detachOwned(win, parent *application.WebviewWindow) {}

// prepareShow has nothing to do here.
func prepareShow(win *application.WebviewWindow) {}

// wakeWindow has nothing to do here (the watchdog only tells the window's page to paint again; showing the window
// again could bring back one that has just closed).
func wakeWindow(win *application.WebviewWindow) {}
