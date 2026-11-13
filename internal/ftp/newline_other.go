// This file holds the macOS/Linux side of ASCII-mode transfers. FTP sends
// text with CRLF line endings, but Unix-like systems end lines with a bare
// LF, so an ASCII download turns every CRLF into LF before writing the
// local file (and uploads turn LF back into CRLF on the wire).

//go:build !windows

package ftp

const asciiDownloadStripsCR = true
