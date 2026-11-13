// This file holds the Windows side of ASCII-mode transfers. FTP sends text
// with CRLF line endings, which already is the Windows convention, so an
// ASCII download is stored exactly as it arrives; only uploads convert bare
// LF characters (from files written by cross-platform tools) to CRLF.
package ftp

const asciiDownloadStripsCR = false
