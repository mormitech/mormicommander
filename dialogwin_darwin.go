//go:build darwin

// Dialog windows on macOS (feature_dialog_window.go): a dialog window is a child window of the window it belongs to,
// so it stays above it and moves with it, and it is sized and placed in one step without the animation Wails' SetSize
// uses (a dialog has to appear at once, at its final size).
package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// dlgPlace gives a window an inside of cw x ch points with its top left corner at x, y (measured from the top left of
// the primary screen, as Wails' Position is).
static void dlgPlace(void *w, int x, int y, int cw, int ch) {
	if (!w) return;
	NSWindow *win = (__bridge NSWindow *)w;
	NSScreen *primary = [[NSScreen screens] firstObject];
	if (!primary) primary = [NSScreen mainScreen];
	CGFloat ph = [primary frame].size.height;
	NSRect f = [win frameRectForContentRect:NSMakeRect(0, 0, cw, ch)];
	f.origin.x = x;
	f.origin.y = ph - y - f.size.height;
	[win setFrame:f display:YES animate:NO];
}

// dlgFrameExtra tells how much bigger a window's frame is than its inside (title bar, borders), from the window's own
// style; 0 when there is no window.
static int dlgFrameExtra(void *w, int *ew, int *eh) {
	if (!w) return 0;
	NSWindow *win = (__bridge NSWindow *)w;
	NSRect f = [win frame];
	NSRect c = [win contentRectForFrameRect:f];
	*ew = (int)(f.size.width - c.size.width);
	*eh = (int)(f.size.height - c.size.height);
	return 1;
}

static void dlgAttach(void *parent, void *child) {
	if (!parent || !child) return;
	NSWindow *p = (__bridge NSWindow *)parent, *c = (__bridge NSWindow *)child;
	if ([c parentWindow] == p) return;
	if ([c parentWindow]) [[c parentWindow] removeChildWindow:c];
	[p addChildWindow:c ordered:NSWindowAbove];
}

static void dlgDetach(void *child) {
	if (!child) return;
	NSWindow *c = (__bridge NSWindow *)child;
	if ([c parentWindow]) [[c parentWindow] removeChildWindow:c];
}
*/
import "C"

import "github.com/wailsapp/wails/v3/pkg/application"

// placeWindow sizes a dialog window's inside to w x h and puts its top left corner at x, y.
func placeWindow(win *application.WebviewWindow, x, y, w, h, frameW, frameH int) {
	application.InvokeSync(func() { C.dlgPlace(win.NativeWindow(), C.int(x), C.int(y), C.int(w), C.int(h)) })
}

// frameExtra is how much bigger the window's frame is than its inside, asked of the window itself (a page's innerHeight
// in a hidden spare window does not tell it reliably on macOS, see issue #135).
func frameExtra(win *application.WebviewWindow) (int, int, bool) {
	if win == nil {
		return 0, 0, false
	}
	var ew, eh C.int
	var ok C.int
	application.InvokeSync(func() { ok = C.dlgFrameExtra(win.NativeWindow(), &ew, &eh) })
	return int(ew), int(eh), ok != 0
}

// attachOwned is done when the window is shown on macOS (adding a child window puts it on the screen).
func attachOwned(win, parent *application.WebviewWindow) {}

// showOwned makes the shown dialog window a child of the window below it; true: it is the key window (Show made it).
func showOwned(win, parent *application.WebviewWindow) bool {
	if parent != nil {
		application.InvokeSync(func() { C.dlgAttach(parent.NativeWindow(), win.NativeWindow()) })
	}
	return true
}

// detachOwned lets go of a closing dialog window.
func detachOwned(win, parent *application.WebviewWindow) {
	application.InvokeSync(func() { C.dlgDetach(win.NativeWindow()) })
}
