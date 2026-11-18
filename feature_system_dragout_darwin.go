//go:build darwin

// The Go half of native file drags on macOS (board issue #108): it hands the files to the Objective-C code in
// feature_system_dragout_darwin.m on the main thread, and passes the end of the drag back to the page (dragEnded in
// feature_system_dragout.go).
package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>

int mcStartFileDrag(void *w, char **paths, int n);
unsigned long mcModifierFlags(void);
int mcLeftButtonDown(void);
*/
import "C"

import (
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const nativeDragSupported = true

// nativeFileDrag starts a native drag of the files from the window; false when it could not start.
func nativeFileDrag(w *application.WebviewWindow, paths []string) bool {
	if w == nil || len(paths) == 0 {
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
		return C.mcStartFileDrag(w.NativeWindow(), (**C.char)(cpaths), C.int(len(paths))) != 0
	})
}

// mouseButtonDown: the left mouse button is held now (safe from any goroutine).
func mouseButtonDown() bool { return C.mcLeftButtonDown() != 0 }

// heldModifiers reads the modifier keys held now.
func heldModifiers() DragModifiers { return modifiersFromMacFlags(uint64(C.mcModifierFlags())) }

//export mcDragEnded
func mcDragEnded(op C.int) {
	// The page is told from another goroutine, so the main thread that ended the drag is not held up.
	go dragEnded(int(op))
}
