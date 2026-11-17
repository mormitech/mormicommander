//go:build !darwin && !windows

// Linux has no one place that gives a file's icon (each desktop has its icon theme), so the file lists keep the page's
// own icons there (feature_icons.go).
package main

const sysIconsSupported = false

func sysIconForType(ext string, px int) []byte { return nil }

func sysIconForPath(path string, px int) []byte { return nil }
