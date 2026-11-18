//go:build darwin

// The Dock badge of macOS (F-749): a short label such as "42%" on the app's Dock icon while jobs run.
package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include <stdlib.h>
#include <string.h>

// mcSetDockBadge takes a copy of label (or NULL) and sets it on the main thread, where AppKit wants it.
static void mcSetDockBadge(const char *label) {
	char *copy = (label && label[0]) ? strdup(label) : NULL;
	dispatch_async(dispatch_get_main_queue(), ^{
		@autoreleasepool {
			[[NSApp dockTile] setBadgeLabel:(copy ? [NSString stringWithUTF8String:copy] : nil)];
		}
		free(copy);
	});
}
*/
import "C"
import "unsafe"

// setDockBadge shows label on the Dock icon; "" takes the badge off.
func setDockBadge(label string) {
	if label == "" {
		C.mcSetDockBadge(nil)
		return
	}
	c := C.CString(label)
	defer C.free(unsafe.Pointer(c))
	C.mcSetDockBadge(c)
}
