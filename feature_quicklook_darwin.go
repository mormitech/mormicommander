//go:build darwin

// The Go half of native Quick Look on macOS (board issue #19, F-863): it hands the files to the Objective-C code in
// feature_quicklook_darwin.m on the main thread. feature_quicklook.go decides what may be shown.
package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework Quartz
#include <stdlib.h>

int mcQuickLookShow(void *w, char **paths, int n);
int mcQuickLookOpen(void);
void mcQuickLookClose(void);
*/
import "C"

import (
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const quickLookSupported = true

// quickLookShow shows the files in the Quick Look panel (opening it when it is closed); false when it could not.
func quickLookShow(w *application.WebviewWindow, paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	return application.InvokeSyncWithResult(func() bool {
		cpaths := C.malloc(C.size_t(len(paths)) * C.size_t(unsafe.Sizeof(uintptr(0))))
		defer C.free(cpaths)
		arr := unsafe.Slice((**C.char)(cpaths), len(paths))
		for i, p := range paths {
			arr[i] = C.CString(p)
		}
		defer func() {
			for _, s := range arr {
				C.free(unsafe.Pointer(s))
			}
		}()
		var win unsafe.Pointer
		if w != nil {
			win = w.NativeWindow()
		}
		return C.mcQuickLookShow(win, (**C.char)(cpaths), C.int(len(paths))) != 0
	})
}

// quickLookOpen says whether the Quick Look panel is on the screen.
func quickLookOpen() bool {
	return application.InvokeSyncWithResult(func() bool { return C.mcQuickLookOpen() != 0 })
}

// quickLookClose takes the Quick Look panel off the screen.
func quickLookClose() { application.InvokeSync(func() { C.mcQuickLookClose() }) }
