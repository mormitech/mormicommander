//go:build linux && cgo && !android && !server && !purego

// Dialog windows on Linux (feature_dialog_window.go): a dialog window is transient for the window it belongs to, so
// the window manager keeps it above that window and places it with it. Wails builds with GTK 4 unless the gtk3 tag is
// given; gtk_window_set_transient_for is the same in both.
package main

/*
#cgo !gtk3 pkg-config: gtk4
#cgo gtk3 pkg-config: gtk+-3.0
#include <gtk/gtk.h>

static void dlgTransient(void *child, void *parent) {
	if (!child) return;
	gtk_window_set_transient_for(GTK_WINDOW(child), parent ? GTK_WINDOW(parent) : NULL);
}
*/
import "C"

import (
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// placeWindow sizes a dialog window so its inside is w x h and puts its top left corner at x, y.
func placeWindow(win *application.WebviewWindow, x, y, w, h, frameW, frameH int) {
	win.SetSize(w+frameW, h+frameH)
	win.SetPosition(x, y)
}

// attachOwned makes the dialog window transient for the window below it.
func attachOwned(win, parent *application.WebviewWindow) {
	if parent == nil {
		return
	}
	c, p := win.NativeWindow(), parent.NativeWindow()
	application.InvokeSync(func() { C.dlgTransient(unsafe.Pointer(c), unsafe.Pointer(p)) })
}

// showOwned has nothing to do here (the owner was set before); false: the window still needs the focus.
func showOwned(win, parent *application.WebviewWindow) bool { return false }

func detachOwned(win, parent *application.WebviewWindow) {}
