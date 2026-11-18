//go:build !darwin

package main

// setDockBadge does nothing outside macOS: Windows and Linux have no Dock badge the app can set this simply (F-749).
func setDockBadge(string) {}
