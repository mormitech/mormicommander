//go:build darwin

// The macOS half of the system file icons (feature_icons.go): NSWorkspace's icons, the ones Finder shows, drawn into a
// PNG by feature_icons_darwin.m.
package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>

void *mcIconPNG(const char *path, const char *ext, int px, int *outLen);
*/
import "C"

import "unsafe"

const sysIconsSupported = true

func sysIconPNG(path, ext string, isType bool, px int) []byte {
	var cp, ce *C.char
	if isType {
		ce = C.CString(ext)
		defer C.free(unsafe.Pointer(ce))
	} else {
		cp = C.CString(path)
		defer C.free(unsafe.Pointer(cp))
	}
	var n C.int
	p := C.mcIconPNG(cp, ce, C.int(px), &n)
	if p == nil || n <= 0 {
		return nil
	}
	defer C.free(p)
	return C.GoBytes(p, n)
}

func sysIconForType(ext string, px int) []byte { return sysIconPNG("", ext, true, px) }

func sysIconForPath(path string, px int) []byte { return sysIconPNG(path, "", false, px) }
