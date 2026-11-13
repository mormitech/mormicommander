// On Windows Total Commander writes checksum files with CRLF line endings and
// backslashes between folder names, which is what Windows users and tools
// expect, so that is the default here. The "Unix format" checkbox in the
// dialog switches to LF and forward slashes.

package fileutil

const defaultUnixFormat = false
