//go:build !unix

// Windows (and other non-Unix systems) can't open a folder for syncing, and
// MoveFileEx already makes the rename durable, so there is nothing to do.

package config

func syncDir(string) {}
