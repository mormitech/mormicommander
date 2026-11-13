//go:build !windows

// On macOS and Linux checksum files use LF line endings and forward slashes,
// matching md5sum, shasum and b3sum, so "Unix format" is the default there.
// The Windows build uses TC's CRLF and backslash layout instead.

package fileutil

const defaultUnixFormat = true
