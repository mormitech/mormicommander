// Native file drags on macOS (board issue #108, feature_system_dragout_darwin.go). mcStartFileDrag begins an AppKit
// dragging session of file URLs from the main window's web view, the way a Finder window drags files, so Finder and
// other apps receive real files. The page has cancelled its own HTML drag before asking, while the mouse button is
// still down; the session is started with a made-up "mouse dragged" event at the pointer, as AppKit wants an event.
// Other programs are only offered a copy, so a drop on Finder never moves a file out of its panel; inside this app
// every operation is offered and the page picks one from the modifier keys. When the session ends the web view is
// given the mouse-up it never saw (the session took the mouse), and Go is told what the drop did (mcDragEnded).
//
// The file is compiled without ARC, so objects are retained and released by hand.

#import <Cocoa/Cocoa.h>

extern void mcDragEnded(int op);

@interface MCFileDragSource : NSObject <NSDraggingSource>
@property (assign) NSView *view;
@end

// The source of the drag under way (nil when none): only one native drag runs at a time.
static MCFileDragSource *mcSource = nil;

// mcFindWebView finds the web view inside a window's views (nil when there is none).
static NSView *mcFindWebView(NSView *v) {
	Class wk = NSClassFromString(@"WKWebView");
	if (!v || !wk) return nil;
	if ([v isKindOfClass:wk]) return v;
	for (NSView *s in [v subviews]) {
		NSView *f = mcFindWebView(s);
		if (f) return f;
	}
	return nil;
}

// mcMouseEvent makes a left mouse event of the given type at a point in the window.
static NSEvent *mcMouseEvent(NSEventType type, NSWindow *win, NSPoint p, float pressure) {
	return [NSEvent mouseEventWithType:type
				  location:p
			     modifierFlags:[NSEvent modifierFlags]
				 timestamp:[[NSProcessInfo processInfo] systemUptime]
			      windowNumber:[win windowNumber]
				   context:nil
			       eventNumber:0
				clickCount:1
				  pressure:pressure];
}

@implementation MCFileDragSource

- (NSDragOperation)draggingSession:(NSDraggingSession *)session
	sourceOperationMaskForDraggingContext:(NSDraggingContext)context {
	if (context == NSDraggingContextWithinApplication) {
		return NSDragOperationCopy | NSDragOperationLink | NSDragOperationMove | NSDragOperationGeneric;
	}
	return NSDragOperationCopy;
}

- (void)draggingSession:(NSDraggingSession *)session
	   endedAtPoint:(NSPoint)screenPoint
	      operation:(NSDragOperation)operation {
	NSView *v = self.view;
	NSWindow *win = [v window];
	if (v && win) {
		NSPoint p = [win convertPointFromScreen:screenPoint];
		[v mouseUp:mcMouseEvent(NSEventTypeLeftMouseUp, win, p, 0)];
	}
	mcDragEnded((int)operation);
	if (mcSource == self) {
		[mcSource autorelease];
		mcSource = nil;
	}
}

@end

// mcStartFileDrag begins a drag of n files from the window w (an NSWindow). It answers 1 when the drag started, 0 when
// it could not (no window, no files, a drag already under way, or the mouse button no longer down).
int mcStartFileDrag(void *w, char **paths, int n) {
	@autoreleasepool {
		if (!w || n <= 0 || mcSource) return 0;
		if (([NSEvent pressedMouseButtons] & 1) == 0) return 0;
		NSWindow *win = (NSWindow *)w;
		NSView *view = mcFindWebView([win contentView]);
		if (!view) view = [win contentView];
		if (!view) return 0;
		NSPoint wp = [win convertPointFromScreen:[NSEvent mouseLocation]];
		NSPoint vp = [view convertPoint:wp fromView:nil];

		NSMutableArray *items = [NSMutableArray arrayWithCapacity:n];
		for (int i = 0; i < n; i++) {
			NSString *s = paths[i] ? [NSString stringWithUTF8String:paths[i]] : nil;
			if (!s) continue;
			NSURL *u = [NSURL fileURLWithPath:s];
			NSDraggingItem *it = [[[NSDraggingItem alloc] initWithPasteboardWriter:u] autorelease];
			NSImage *icon = [[NSWorkspace sharedWorkspace] iconForFile:s];
			[icon setSize:NSMakeSize(32, 32)];
			CGFloat off = (CGFloat)(i < 4 ? i : 4) * 4;
			[it setDraggingFrame:NSMakeRect(vp.x - 16 + off, vp.y - 16 - off, 32, 32) contents:icon];
			[items addObject:it];
		}
		if ([items count] == 0) return 0;

		mcSource = [[MCFileDragSource alloc] init];
		mcSource.view = view;
		NSDraggingSession *session = [view beginDraggingSessionWithItems:items
									   event:mcMouseEvent(NSEventTypeLeftMouseDragged, win, wp, 1)
									  source:mcSource];
		if (!session) {
			[mcSource release];
			mcSource = nil;
			return 0;
		}
		session.animatesToStartingPositionsOnCancelOrFail = YES;
		session.draggingFormation = NSDraggingFormationPile;
		return 1;
	}
}

// mcModifierFlags are the modifier keys held now (NSEventModifierFlags).
unsigned long mcModifierFlags(void) {
	return (unsigned long)[NSEvent modifierFlags];
}

// mcLeftButtonDown answers 1 while the left mouse button is held (Quartz event state, readable from any thread).
int mcLeftButtonDown(void) {
	return CGEventSourceButtonState(kCGEventSourceStateCombinedSessionState, kCGMouseButtonLeft) ? 1 : 0;
}
