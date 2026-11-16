//go:build darwin

// Whether mormiCommander is the active app (macOS). The network feature asks before it mounts a share: macOS's own
// windows (its sign-in window, its "There was a problem connecting" alert) take the focus, and when they close macOS
// doesn't hand it back, so the app takes it back itself, but only when it had it to begin with.
package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static int appIsActive(void) {
	return [[NSRunningApplication currentApplication] isActive] ? 1 : 0;
}
*/
import "C"

// appActive tells whether this app is the frontmost one.
func appActive() bool { return C.appIsActive() != 0 }
