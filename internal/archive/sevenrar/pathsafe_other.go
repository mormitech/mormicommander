//go:build !windows

// On macOS and Linux every byte except "/" and NUL is legal in a file name,
// so entry names are used as they are (after the traversal checks in
// pathsafe.go).

package sevenrar

const windowsPaths = false
