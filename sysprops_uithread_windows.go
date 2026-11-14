//go:build windows

// The Explorer property sheet (Properties dialog, "System properties" button) must be created on the UI thread,
// which owns the message loop and the COM apartment; Wails calls bound methods on goroutines instead. This hands
// internal/attrs the Wails main-thread dispatcher so the call is moved there. Other platforms start a helper process
// and need nothing.
package main

import (
	"mormicommander/internal/attrs"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func init() {
	attrs.UIThread = application.InvokeSync
}
