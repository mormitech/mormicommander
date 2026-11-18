//go:build !darwin && !windows

// Native file drags are made on macOS and Windows (feature_system_dragout_darwin.go, _windows.go). On Linux the page
// keeps its HTML drag, which carries file URLs (text/uri-list) and paths as text.
package main

import "github.com/wailsapp/wails/v3/pkg/application"

const nativeDragSupported = false

func nativeFileDrag(w *application.WebviewWindow, paths []string) bool { return false }

func heldModifiers() DragModifiers { return DragModifiers{} }
