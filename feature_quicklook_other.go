//go:build !darwin

// Quick Look is a macOS panel (feature_quicklook_darwin.go). On Windows and Linux there is none, so these stand-ins
// show nothing and the page keeps the command greyed out (QuickLook.Supported answers false).
package main

import "github.com/wailsapp/wails/v3/pkg/application"

const quickLookSupported = false

func quickLookShow(w *application.WebviewWindow, paths []string) bool { return false }

func quickLookOpen() bool { return false }

func quickLookClose() {}
