//go:build !darwin && !linux

// On Windows and the other systems extended attributes are absent or behind calls mormiCommander does not use, so
// every file reads as having none and there is nothing to remove.
package xattr

import "errors"

var errUnsupported = errors.New("this system has no extended attributes")

const supported = false

func listNames(string) ([]string, error) { return nil, nil }

func valueSize(string, string) int { return -1 }

func removeName(string, string) error { return nil }

func isAbsent(error) bool { return true }

func getValue(string, string) ([]byte, bool, error) { return nil, false, nil }

func setValue(string, string, []byte) error { return errUnsupported }
