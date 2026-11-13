//go:build unix && !darwin && !linux

package fileops

// The BSDs and other Unix-like systems have extended attributes behind different calls (extattr_*), which mormiCommander
// doesn't target; on them a copy leaves extended attributes out.

func copyXattrs(src, dst string) error { return nil }
