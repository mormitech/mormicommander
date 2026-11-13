//go:build !darwin

// Window numbers for screenshots are a macOS thing (see windowid_darwin.go); elsewhere the QA script takes the picture
// itself.
package main

import "github.com/wailsapp/wails/v3/pkg/application"

// windowID is only known on macOS (see windowid_darwin.go).
func (a *App) windowID() int { return 0 }

func windowNumber(w *application.WebviewWindow) int { return 0 }
