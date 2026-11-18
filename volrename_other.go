//go:build !windows

package main

// setVolumeLabel is only on Windows; macOS renames with diskutil (feature_volume_rename.go).
var setVolumeLabel func(root, name string) error
