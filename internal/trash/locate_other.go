//go:build !darwin && !linux && !windows

package trash

// Systems without a trash this package can use have no trash folders either.

func platformTrashDirs(string) []string { return nil }
