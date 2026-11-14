// Native Quick Look on macOS (board issue #19, F-863; the Go half is feature_quicklook_darwin.go). The system's Quick
// Look panel (QLPreviewPanel, the one Finder opens with Space) shows the file under the cursor, and while it is open the
// page sends it each new file the cursor reaches, so the panel follows the cursor the way it does in Finder.
//
// AppKit gives the panel to whichever object in the responder chain says it wants it. The app's own NSApplication is
// always at the end of that chain, so the three "controller" methods are added to its class at run time (only the
// ones it does not have yet); they accept the panel only while this file has something to show. The main window is
// given the keyboard back once the panel is up, so the arrow keys keep moving the cursor in the panels; keys pressed
// while the user has clicked into the panel are passed back to the main window too.
//
// The file is compiled without ARC, so objects are retained and released by hand. Every function here runs on the main
// thread (the Go side calls them through Wails' InvokeSync).

#import <Cocoa/Cocoa.h>
#import <Quartz/Quartz.h>
#import <objc/runtime.h>

@interface MCQuickLookSource : NSObject <QLPreviewPanelDataSource, QLPreviewPanelDelegate>
@property (retain) NSArray *items;    // NSURLs of the files shown
@property (assign) NSWindow *window;  // the main window, which gets the keyboard back
@end

// The data source of the panel; nil until Quick Look was first asked for.
static MCQuickLookSource *mcQL = nil;

@implementation MCQuickLookSource

- (void)dealloc {
	[_items release];
	[super dealloc];
}

- (NSInteger)numberOfPreviewItemsInPreviewPanel:(QLPreviewPanel *)panel {
	return (NSInteger)[self.items count];
}

- (id<QLPreviewItem>)previewPanel:(QLPreviewPanel *)panel previewItemAtIndex:(NSInteger)index {
	if (index < 0 || index >= (NSInteger)[self.items count]) return nil;
	return [self.items objectAtIndex:(NSUInteger)index];
}

// Keys typed while the panel has the keyboard go to the main window, so the cursor keys, Space and ⌘Y act there.
- (BOOL)previewPanel:(QLPreviewPanel *)panel handleEvent:(NSEvent *)event {
	NSWindow *w = self.window;
	if (!w || ([event type] != NSEventTypeKeyDown && [event type] != NSEventTypeKeyUp)) return NO;
	[w makeKeyWindow];
	[w sendEvent:event];
	return YES;
}

@end

// The controller methods added to the NSApplication class (see the top of the file).
static BOOL mcAcceptsPreviewPanelControl(id self, SEL _cmd, QLPreviewPanel *panel) {
	return mcQL != nil && [mcQL.items count] > 0;
}

static void mcBeginPreviewPanelControl(id self, SEL _cmd, QLPreviewPanel *panel) {
	panel.dataSource = mcQL;
	panel.delegate = mcQL;
}

static void mcEndPreviewPanelControl(id self, SEL _cmd, QLPreviewPanel *panel) {
	if (panel.dataSource == mcQL) panel.dataSource = nil;
	if (panel.delegate == mcQL) panel.delegate = nil;
}

// mcQuickLookSetup adds the controller methods once.
static void mcQuickLookSetup(void) {
	static BOOL done = NO;
	if (done) return;
	done = YES;
	Class c = object_getClass(NSApp);
	if (!c) return;
	if (![NSApp respondsToSelector:@selector(acceptsPreviewPanelControl:)]) {
		class_addMethod(c, @selector(acceptsPreviewPanelControl:), (IMP)mcAcceptsPreviewPanelControl, "B@:@");
	}
	if (![NSApp respondsToSelector:@selector(beginPreviewPanelControl:)]) {
		class_addMethod(c, @selector(beginPreviewPanelControl:), (IMP)mcBeginPreviewPanelControl, "v@:@");
	}
	if (![NSApp respondsToSelector:@selector(endPreviewPanelControl:)]) {
		class_addMethod(c, @selector(endPreviewPanelControl:), (IMP)mcEndPreviewPanelControl, "v@:@");
	}
}

// mcQuickLookShow shows the n files in the Quick Look panel, the first one in front; w is the main window (an NSWindow,
// may be NULL). An open panel switches to the new files; a closed one opens. It answers 1 when the panel is up.
int mcQuickLookShow(void *w, char **paths, int n) {
	@autoreleasepool {
		if (n <= 0) return 0;
		NSMutableArray *urls = [NSMutableArray arrayWithCapacity:(NSUInteger)n];
		for (int i = 0; i < n; i++) {
			NSString *s = paths[i] ? [NSString stringWithUTF8String:paths[i]] : nil;
			if (s) [urls addObject:[NSURL fileURLWithPath:s]];
		}
		if ([urls count] == 0) return 0;

		mcQuickLookSetup();
		if (!mcQL) mcQL = [[MCQuickLookSource alloc] init];
		mcQL.items = urls;
		NSWindow *win = (NSWindow *)w;
		if (win) mcQL.window = win;

		QLPreviewPanel *panel = [QLPreviewPanel sharedPreviewPanel];
		if (!panel) return 0;
		if ([panel isVisible]) {
			if (panel.dataSource != mcQL) [panel updateController];
			[panel reloadData];
			[panel setCurrentPreviewItemIndex:0];
			return 1;
		}
		NSWindow *back = mcQL.window ? mcQL.window : [NSApp keyWindow];
		[panel makeKeyAndOrderFront:nil];
		if (panel.dataSource != mcQL) [panel updateController];
		[panel reloadData];
		[panel setCurrentPreviewItemIndex:0];
		// the keyboard stays with the panels, so the cursor can move while the panel shows the files
		if (back) [back makeKeyWindow];
		return [panel isVisible] ? 1 : 0;
	}
}

// mcQuickLookOpen answers 1 while the Quick Look panel is on the screen.
int mcQuickLookOpen(void) {
	if (![QLPreviewPanel sharedPreviewPanelExists]) return 0;
	return [[QLPreviewPanel sharedPreviewPanel] isVisible] ? 1 : 0;
}

// mcQuickLookClose takes the Quick Look panel off the screen (nothing when it is not open).
void mcQuickLookClose(void) {
	if (![QLPreviewPanel sharedPreviewPanelExists]) return;
	QLPreviewPanel *panel = [QLPreviewPanel sharedPreviewPanel];
	if ([panel isVisible]) [panel orderOut:nil];
}
