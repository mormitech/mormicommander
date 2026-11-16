//go:build !darwin

package main

// appActive is for macOS only, where the system's mount windows take the focus and don't give it back; elsewhere
// nothing needs taking back, so it says false.
func appActive() bool { return false }
