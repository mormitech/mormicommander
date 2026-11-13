//go:build darwin

// The number macOS gives the file manager's window, which `screencapture -l` takes (control mode's "screenshot"). Wails
// v3 hands out the window's NSWindow, so the number is asked of the window itself: with the About box in the same
// process, "the first window of this process" (what the v2 shell used) could be the wrong one.
package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static int windowNumberOf(void *w) {
	if (!w) return 0;
	NSWindow *win = (__bridge NSWindow *)w;
	if (![win isVisible]) return 0;
	return (int)[win windowNumber];
}
*/
import "C"

import "github.com/wailsapp/wails/v3/pkg/application"

// windowID is the number of the main window on the screen, for `screencapture -l ID file.png` (0 when it is not on
// the screen).
func (a *App) windowID() int { return windowNumber(a.mainWindow()) }

// windowNumber is the number of a window on the screen (0 when it is not on the screen).
func windowNumber(w *application.WebviewWindow) int {
	if w == nil {
		return 0
	}
	return application.InvokeSyncWithResult(func() int { return int(C.windowNumberOf(w.NativeWindow())) })
}
