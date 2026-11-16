//go:build darwin

// Dialog windows on macOS (feature_dialog_window.go): a dialog window is a child window of the window it belongs to,
// so it stays above it and moves with it, and it is sized and placed in one step without the animation Wails' SetSize
// uses (a dialog has to appear at once, at its final size). A dialog window's web view draws whenever its window is on
// the screen (WebKit's own guess from the window's occlusion state is not used: for a window that was hidden until a
// moment ago it now and then stayed "not visible", and the page then never drew the dialog), and the watchdog can wake
// a window whose page has not drawn (wakeWindow).
package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>

// While the user drags a dialog window by its title bar it is not a child window: macOS moves a child window against its
// parent on every step of a drag, and on recent macOS that made the drag choppy (some ten frames a second). The window
// lets go of its parent when the drag starts (NSWindowWillMoveNotification) and is attached again, above it, once the
// mouse button is up. Windows moved by code (dlgPlace) are no child windows' drags and are not touched: the
// notification comes only for user moves.
static char dlgParentKey;

static void dlgWatchDrags(void) {
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		[[NSNotificationCenter defaultCenter] addObserverForName:NSWindowWillMoveNotification object:nil queue:nil
			usingBlock:^(NSNotification *n) {
				NSWindow *c = n.object;
				NSWindow *p = [c parentWindow];
				if (!p) return;
				objc_setAssociatedObject(c, &dlgParentKey, p, OBJC_ASSOCIATION_ASSIGN);
				[p removeChildWindow:c];
				[NSTimer scheduledTimerWithTimeInterval:0.05 repeats:YES block:^(NSTimer *t) {
					if ([NSEvent pressedMouseButtons] & 1) return;
					[t invalidate];
					NSWindow *back = objc_getAssociatedObject(c, &dlgParentKey);
					objc_setAssociatedObject(c, &dlgParentKey, nil, OBJC_ASSOCIATION_ASSIGN);
					if (back && [c isVisible] && [back isVisible] && ![c parentWindow]) [back addChildWindow:c ordered:NSWindowAbove];
				}];
			}];
	});
}

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
	dlgWatchDrags();
	NSWindow *p = (__bridge NSWindow *)parent, *c = (__bridge NSWindow *)child;
	objc_setAssociatedObject(c, &dlgParentKey, nil, OBJC_ASSOCIATION_ASSIGN); // (a drag in progress gives it back to no one else)
	if ([c parentWindow] == p) return;
	if ([c parentWindow]) [[c parentWindow] removeChildWindow:c];
	[p addChildWindow:c ordered:NSWindowAbove];
}

// dlgWebView finds the web view inside a window (nil when there is none).
static NSView *dlgWebView(NSView *v) {
	Class wk = NSClassFromString(@"WKWebView");
	if (!v || !wk) return nil;
	if ([v isKindOfClass:wk]) return v;
	for (NSView *s in [v subviews]) {
		NSView *f = dlgWebView(s);
		if (f) return f;
	}
	return nil;
}

// dlgNoOcclusion makes the window's web view draw whenever the window is on the screen, whatever its occlusion state
// says (WKWebView's _windowOcclusionDetectionEnabled; nothing happens on a WebKit without it).
static void dlgNoOcclusion(void *w) {
	if (!w) return;
	NSView *wv = dlgWebView([(__bridge NSWindow *)w contentView]);
	SEL sel = NSSelectorFromString(@"_setWindowOcclusionDetectionEnabled:");
	if (!wv || ![wv respondsToSelector:sel]) return;
	void (*set)(id, SEL, BOOL) = (void (*)(id, SEL, BOOL))[wv methodForSelector:sel];
	set(wv, sel, NO);
}

// dlgWake wakes a shown window whose page has not drawn: it is put on the screen again if it is not there, its web
// view is hidden and shown (WebKit then works out again whether the page is visible) and the window is resized by one
// point and back (the page lays out and paints again).
static void dlgWake(void *w) {
	if (!w) return;
	NSWindow *win = (__bridge NSWindow *)w;
	dlgNoOcclusion(w);
	if (![win isVisible]) [win orderFront:nil];
	NSView *wv = dlgWebView([win contentView]);
	if (wv) {
		[wv setHidden:YES];
		[wv setHidden:NO];
		[wv setNeedsLayout:YES];
		[wv setNeedsDisplay:YES];
	}
	NSRect f = [win frame];
	NSRect g = f;
	g.size.width += 1;
	[win setFrame:g display:YES animate:NO];
	[win setFrame:f display:YES animate:NO];
}

static void dlgDetach(void *child) {
	if (!child) return;
	NSWindow *c = (__bridge NSWindow *)child;
	objc_setAssociatedObject(c, &dlgParentKey, nil, OBJC_ASSOCIATION_ASSIGN); // (closing: a drag's end must not attach it again)
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

// prepareShow makes the window's web view draw whenever the window is on the screen (see the top of this file).
func prepareShow(win *application.WebviewWindow) {
	application.InvokeSync(func() { C.dlgNoOcclusion(win.NativeWindow()) })
}

// wakeWindow wakes a shown dialog window whose page has not said the dialog is drawn (the watchdog in
// feature_dialog_window.go).
func wakeWindow(win *application.WebviewWindow) {
	application.InvokeSync(func() { C.dlgWake(win.NativeWindow()) })
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
