//go:build !darwin

// Native file drags are only made on macOS so far (feature_system_dragout_darwin.go). On Windows and Linux the page
// keeps its HTML drag, which carries file URLs and paths as text.
package main

import "github.com/wailsapp/wails/v3/pkg/application"

const nativeDragSupported = false

func nativeFileDrag(w *application.WebviewWindow, paths []string) bool { return false }

func heldModifiers() DragModifiers { return DragModifiers{} }
