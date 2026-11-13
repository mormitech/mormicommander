//go:build !windows

package fileops

// macOS and Linux accept every name without "/" and NUL, which checkName already refuses, so there is nothing more
// to check here (Windows has its own rules in name_windows.go).

func checkNamePlatform(name string) error { return nil }
