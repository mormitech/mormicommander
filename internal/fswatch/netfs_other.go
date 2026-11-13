//go:build !darwin && !linux && !windows

// Network drive detection on other systems. There is no portable way to ask for the filesystem type, so every
// folder counts as local; the native backend (kqueue on the BSDs) or polling still applies, and users can switch
// a share to polling or off with the exclusion list.
package fswatch

func isNetworkPath(string) bool { return false }
